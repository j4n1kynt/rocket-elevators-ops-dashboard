"""
AND-105 Task 7: generate_explanations.py
Generates 2-3 sentence risk explanations for all HIGH-risk elevators using Ollama
(qwen2.5:1.5b) and writes them to the predictions.risk_explanation column.

Pre-extraction strategy: Python extracts structured key facts before calling the LLM,
so the 1.5B model only needs to do NLG over concrete numbers — no reasoning over raw JSON.

Usage:
  py -3 intelligence/generate_explanations.py             # all HIGH, skip existing
  py -3 intelligence/generate_explanations.py --limit 20  # test on first 20
  py -3 intelligence/generate_explanations.py --force     # regenerate all (ignores existing)
  py -3 intelligence/generate_explanations.py --verify    # check results only, no generation
"""
# AND-105 Task 7: bulk risk explanation generation

import argparse, asyncio, json, subprocess, sys, time
from datetime import date, timedelta

import requests

# ── Config ────────────────────────────────────────────────────────────────────
OLLAMA_URL  = 'http://localhost:11434'
MODEL       = 'qwen2.5:1.5b'
CONCURRENCY = 8          # parallel Ollama requests
BATCH_SIZE  = 100        # elevators processed per DB round-trip
TIMEOUT     = 120        # seconds per Ollama call
DB_CTR      = 'rocket-elevators-ops-dashboard-db-1'
DB_USER     = 'api_user'
DB_NAME     = 'rocket_elevators'

# Pre-extraction prompt — tuned for qwen2.5:1.5b (1.5B parameters)
# Rigid template: model receives structured facts, not raw JSON, so it only does NLG
SYSTEM_PROMPT = '\n'.join([
    'You are an elevator safety analyst. Write exactly 2-3 sentences explaining why this elevator is HIGH risk.',
    '',
    'Rules:',
    '- Use ONLY the facts given below. Do not invent or add any information.',
    '- Start with the most recent inspection: cite its date and outcome.',
    '- If failed_inspections > 0, mention the count.',
    '- If incidents > 0, mention the count.',
    '- Do not mention risk scores, model names, or algorithms.',
    '- If all indicators show no problems, say so directly — do not manufacture concerns.',
])

# ── DB helpers ────────────────────────────────────────────────────────────────
def _psql(sql, via_stdin=False):
    """Run SQL against the Docker PostgreSQL container. Returns stdout text."""
    base = ['docker', 'exec']
    if via_stdin:
        base.append('-i')
    base += [DB_CTR, 'psql', '-U', DB_USER, '-d', DB_NAME, '-t', '-A']
    if via_stdin:
        result = subprocess.run(base, input=sql, text=True, capture_output=True)
    else:
        result = subprocess.run(base + ['-c', sql], text=True, capture_output=True)
    if result.returncode != 0:
        raise RuntimeError(f'psql exited {result.returncode}: {result.stderr.strip()[:200]}')
    return result.stdout.strip()


def query_rows(sql):
    """Run a SELECT and return list of dicts via json_agg."""
    wrapped = f'SELECT json_agg(row_to_json(t)) FROM ({sql}) t'
    raw = _psql(wrapped)
    if not raw or raw == 'null':
        return []
    return json.loads(raw)


def update_batch(updates):
    """Write a batch of {elevator_id: explanation} to predictions in one transaction."""
    if not updates:
        return
    # \set ON_ERROR_STOP on causes psql to exit non-zero on any SQL error,
    # which prevents a silent ROLLBACK when COMMIT is issued on an aborted transaction
    lines = ['\\set ON_ERROR_STOP on\nBEGIN;\n']
    for eid, explanation in updates.items():
        # Validate eid is an integer before interpolating into SQL structure
        eid_int = int(eid)
        # Strip null bytes (PostgreSQL rejects them mid-string, silently rolling back
        # the batch); escape single quotes for standard SQL quoting
        escaped = explanation.replace('\x00', '').replace("'", "''")
        lines.append(
            f"UPDATE predictions SET risk_explanation = '{escaped}' "
            f"WHERE elevator_id = {eid_int};\n"
        )
    lines.append('COMMIT;\n')
    _psql(''.join(lines), via_stdin=True)


# ── Pre-extraction ────────────────────────────────────────────────────────────
_PASSING_OUTCOMES = {'Passed', 'Complete', 'Pass', 'Completed'}
_PENDING_STATUSES = {'Pending', 'In Progress', 'Open', 'Follow up', 'Followup'}


def build_summary(elev):
    """Extract key facts from raw context so the 1.5B model only does NLG."""
    inspections = elev.get('inspections', [])
    incidents   = elev.get('incidents', [])
    alterations = elev.get('alterations', [])

    most_recent = inspections[0] if inspections else None
    failed_insp = sum(1 for i in inspections if i.get('outcome') not in _PASSING_OUTCOMES)
    pending_alts = sum(1 for a in alterations if a.get('status') in _PENDING_STATUSES)

    return {
        'most_recent_inspection': most_recent,
        'total_inspections':      len(inspections),
        'failed_inspections':     failed_insp,
        'incidents_past_2yr':     len(incidents),
        'pending_alterations':    pending_alts,
    }


# ── User message formatter ────────────────────────────────────────────────────
def user_msg(elev):
    s = build_summary(elev)
    mr = s['most_recent_inspection']
    mr_str = (
        f"{mr.get('date', '?')} | {mr.get('inspection_type', '?')} | outcome: {mr.get('outcome', '?')}"
        if mr else 'none on record'
    )
    return '\n'.join([
        f"Elevator {elev['elevator_id']} — {elev.get('elevator_type', 'Elevator')} at {elev['location']}",
        f"Risk Level: {elev['risk_level']}",
        '',
        f"Most recent inspection: {mr_str}",
        f"Non-passing inspections (of last {s['total_inspections']}): {s['failed_inspections']}",
        f"Incidents in past 2 years: {s['incidents_past_2yr']}",
        f"Pending alterations: {s['pending_alterations']}",
    ])


# ── Async Ollama call (thread-pool via asyncio.to_thread) ────────────────────
def _call_ollama_sync(system, user):
    """Blocking Ollama call — run in thread pool via asyncio.to_thread."""
    payload = {
        'model':   MODEL,
        'stream':  False,
        'options': {'temperature': 0.1, 'num_predict': 150},
        'messages': [
            {'role': 'system', 'content': system},
            {'role': 'user',   'content': user},
        ],
    }
    for attempt in (1, 2):
        try:
            resp = requests.post(
                f'{OLLAMA_URL}/api/chat',
                json=payload,
                timeout=TIMEOUT,
            )
            resp.raise_for_status()
            return resp.json()['message']['content'].strip()
        except Exception as exc:
            if attempt == 2:
                return f'[ERROR: {exc}]'
            time.sleep(5)


async def call_ollama(semaphore, system, user):
    """Async wrapper — acquires semaphore then runs Ollama in a thread."""
    async with semaphore:
        return await asyncio.to_thread(_call_ollama_sync, system, user)


async def call_ollama_timed(semaphore, system, user_content, idx, total, eid):
    """Like call_ollama but prints a progress line on start and returns (explanation, elapsed_s)."""
    async with semaphore:
        print(f'  Processing elevator {idx}/{total} (ID {eid})...', flush=True)
        t0 = time.time()
        result = await asyncio.to_thread(_call_ollama_sync, system, user_content)
        return result, time.time() - t0


# ── Batch context fetch ───────────────────────────────────────────────────────
def fetch_context_batch(elevator_ids):
    """Fetch inspections, incidents, alterations for a list of elevator IDs in 3 queries."""
    # Cast to int before interpolating into IN (...) — rejects any non-integer
    # value that could survive the JSON round-trip as a crafted string
    try:
        ids_str = ','.join(str(int(i)) for i in elevator_ids)
    except (ValueError, TypeError) as exc:
        raise ValueError(f'elevator_ids must be integers: {exc}') from exc
    two_years_ago = (date.today() - timedelta(days=730)).isoformat()

    # Top-5 inspections per elevator, most recent first
    insp_rows = query_rows(f"""
        SELECT elevator_id, inspection_type,
               latest_inspection_date::text AS date,
               outcome, rn
        FROM (
            SELECT elevator_id, inspection_type,
                   latest_inspection_date,
                   outcome,
                   ROW_NUMBER() OVER (
                       PARTITION BY elevator_id
                       ORDER BY latest_inspection_date DESC NULLS LAST
                   ) AS rn
            FROM inspections
            WHERE elevator_id IN ({ids_str})
        ) sub
        WHERE rn <= 5
    """)

    # Incidents in past 2 years
    inc_rows = query_rows(f"""
        SELECT elevator_id, category,
               date_of_occurrence::text AS date,
               injury_severity,
               LEFT(incident_summary, 100) AS summary
        FROM incidents
        WHERE elevator_id IN ({ids_str})
          AND date_of_occurrence >= '{two_years_ago}'
        ORDER BY elevator_id, date_of_occurrence DESC
    """)

    # Top-5 alterations per elevator
    alt_rows = query_rows(f"""
        SELECT elevator_id, alteration_type, status,
               LEFT(summary, 80) AS summary, rn
        FROM (
            SELECT elevator_id, alteration_type, status, summary,
                   ROW_NUMBER() OVER (
                       PARTITION BY elevator_id ORDER BY alteration_id DESC
                   ) AS rn
            FROM alterations
            WHERE elevator_id IN ({ids_str})
        ) sub
        WHERE rn <= 5
    """)

    # Group by elevator_id
    inspections_map = {}
    for r in (insp_rows or []):
        eid = r['elevator_id']
        inspections_map.setdefault(eid, []).append(r)

    incidents_map = {}
    for r in (inc_rows or []):
        eid = r['elevator_id']
        incidents_map.setdefault(eid, []).append(r)

    alterations_map = {}
    for r in (alt_rows or []):
        eid = r['elevator_id']
        alterations_map.setdefault(eid, []).append(r)

    return inspections_map, incidents_map, alterations_map


# ── Main async loop ───────────────────────────────────────────────────────────
async def generate_all(elevators, args):
    semaphore = asyncio.Semaphore(CONCURRENCY)
    total = len(elevators)
    done = 0
    errors = 0
    t_start = time.time()

    # Process in batches
    for batch_start in range(0, total, BATCH_SIZE):
        batch = elevators[batch_start:batch_start + BATCH_SIZE]
        batch_ids = [e['elevator_id'] for e in batch]

        # Fetch context for this batch
        insp_map, inc_map, alt_map = fetch_context_batch(batch_ids)

        # Augment elevator dicts
        augmented = []
        for e in batch:
            eid = e['elevator_id']
            augmented.append({
                **e,
                'inspections': insp_map.get(eid, []),
                'incidents':   inc_map.get(eid, []),
                'alterations': alt_map.get(eid, []),
            })

        # Build async tasks — each prints progress on start, returns (explanation, elapsed_s)
        tasks = [
            call_ollama_timed(
                semaphore, SYSTEM_PROMPT, user_msg(e),
                batch_start + i + 1, total, e['elevator_id'],
            )
            for i, e in enumerate(augmented)
        ]
        # return_exceptions=True prevents CancelledError (Ctrl+C) from discarding
        # completed work — exceptions are returned as values, not raised
        results = await asyncio.gather(*tasks, return_exceptions=True)

        # Collect updates and count errors
        cancelled = False
        updates = {}
        for e, result in zip(augmented, results):
            eid = e['elevator_id']
            if isinstance(result, asyncio.CancelledError):
                cancelled = True  # save completed work first, re-raise after
                continue
            elif isinstance(result, BaseException):
                errors += 1
                print(f'  FAIL {eid}: {type(result).__name__}: {result}', flush=True)
                continue
            explanation, call_elapsed = result
            if explanation.startswith('[ERROR:'):
                errors += 1
                print(f'  FAIL {eid}: {explanation}', flush=True)
            else:
                print(f'    -> {call_elapsed:.1f}s | {len(explanation)} chars', flush=True)
                updates[eid] = explanation

        # Write to DB
        try:
            update_batch(updates)
        except RuntimeError as exc:
            errors += len(updates)
            print(f'  FAIL batch write -- {len(updates)} explanations lost: {exc}', flush=True)
        if cancelled:
            raise asyncio.CancelledError()
        done += len(batch)

        # Progress report
        elapsed = time.time() - t_start
        rate = done / elapsed if elapsed > 0 else 0
        remaining = (total - done) / rate if rate > 0 else 0
        pct = done / total * 100
        print(
            f'  [{done}/{total}] {pct:.1f}%  '
            f'{elapsed:.0f}s elapsed  '
            f'~{remaining/3600:.1f}h remaining  '
            f'{errors} errors',
            flush=True,
        )

    return done, errors


# ── Verification ──────────────────────────────────────────────────────────────
def verify():
    total = query_rows(
        "SELECT COUNT(*) AS n FROM predictions WHERE risk_level = 'HIGH'"
    )
    explained = query_rows(
        "SELECT COUNT(*) AS n FROM predictions WHERE risk_level = 'HIGH' AND risk_explanation IS NOT NULL"
    )
    null_count = query_rows(
        "SELECT COUNT(*) AS n FROM predictions WHERE risk_level = 'HIGH' AND risk_explanation IS NULL"
    )
    t = int(total[0]['n']) if total else 0
    e = int(explained[0]['n']) if explained else 0
    n = int(null_count[0]['n']) if null_count else 0

    avg_row = query_rows(
        "SELECT AVG(LENGTH(risk_explanation))::int AS avg_len "
        "FROM predictions WHERE risk_level = 'HIGH' AND risk_explanation IS NOT NULL"
    )
    avg_len = int(avg_row[0]['avg_len']) if avg_row and avg_row[0]['avg_len'] else 0

    print(f'HIGH elevators total:       {t:>6}')
    print((f'  With explanation:         {e:>6}  ({e/t*100:.1f}%)') if t else '')
    print(f'  Without explanation:      {n:>6}')
    print(f'  Avg explanation length:   {avg_len:>6} chars')

    # Spot-check 3 random rows — show source inspections alongside explanation
    samples = query_rows("""
        SELECT p.elevator_id, p.risk_level, p.risk_score::float8,
               p.risk_explanation, e.location
        FROM predictions p
        JOIN elevators e ON e.elevator_id = p.elevator_id
        WHERE p.risk_level = 'HIGH' AND p.risk_explanation IS NOT NULL
        ORDER BY RANDOM() LIMIT 3
    """)
    if samples:
        print()
        print('Spot-check (3 random) — explanation vs source inspection data:')
        for s in samples:
            eid = int(s['elevator_id'])
            inspections = query_rows(f"""
                SELECT inspection_type, latest_inspection_date::text AS date, outcome
                FROM inspections WHERE elevator_id = {eid}
                ORDER BY latest_inspection_date DESC NULLS LAST LIMIT 3
            """)
            print(f"  Elevator {eid} ({s['risk_level']}, {float(s['risk_score']):.4f}) — {s['location']}")
            print(f"  Source — top 3 inspections:")
            if inspections:
                for insp in inspections:
                    print(f"    {insp.get('date','?')} | {insp.get('inspection_type','?')} | {insp.get('outcome','?')}")
            else:
                print(f"    No inspections on record")
            print(f"  Explanation:")
            print(f"    {s['risk_explanation']}")
            print()

    return n == 0  # True = fully populated


# ── Entry point ───────────────────────────────────────────────────────────────
def main():
    parser = argparse.ArgumentParser(description='Generate Ollama risk explanations for HIGH elevators')
    parser.add_argument('--limit',  type=int, default=None, help='Process only the first N elevators (for testing)')
    parser.add_argument('--force',  action='store_true',    help='Regenerate even if risk_explanation already set')
    parser.add_argument('--verify', action='store_true',    help='Run verification only, no generation')
    args = parser.parse_args()

    if args.verify:
        print('=== Verification ===')
        ok = verify()
        sys.exit(0 if ok else 1)

    # Windows asyncio fix
    if sys.platform == 'win32':
        asyncio.set_event_loop_policy(asyncio.WindowsSelectorEventLoopPolicy())

    # Fetch target elevators
    filter_clause = '' if args.force else 'AND p.risk_explanation IS NULL'
    limit_clause  = f'LIMIT {args.limit}' if args.limit else ''
    elevators = query_rows(f"""
        SELECT e.elevator_id, e.location,
               COALESCE(e.elevator_type, 'Elevator') AS elevator_type,
               p.risk_score::float8 AS risk_score, p.risk_level
        FROM elevators e
        JOIN predictions p ON p.elevator_id = e.elevator_id
        WHERE p.risk_level = 'HIGH' {filter_clause}
        ORDER BY p.risk_score DESC
        {limit_clause}
    """)

    if not elevators:
        print('No HIGH elevators to process (all already explained). Use --force to regenerate.')
        sys.exit(0)

    mode = 'FORCE' if args.force else 'RESUME'
    print(f'Model:       {MODEL}')
    print(f'Elevators:   {len(elevators)} (mode: {mode}{f", limit={args.limit}" if args.limit else ""})')
    print(f'Concurrency: {CONCURRENCY} parallel Ollama requests')
    print(f'Batch size:  {BATCH_SIZE} elevators per DB round-trip')
    est_hours = len(elevators) * 8.0 / CONCURRENCY / 3600
    print(f'Estimated:   ~{est_hours:.1f}h at 8s/call x {CONCURRENCY} concurrent')
    print()

    t_start = time.time()
    done, errors = asyncio.run(generate_all(elevators, args))
    elapsed = time.time() - t_start

    print()
    print(f'=== Complete ===')
    print(f'Processed:  {done} elevators in {elapsed/3600:.2f}h')
    print(f'Errors:     {errors}')
    print()
    print('=== Verification ===')
    verify()


if __name__ == '__main__':
    main()
