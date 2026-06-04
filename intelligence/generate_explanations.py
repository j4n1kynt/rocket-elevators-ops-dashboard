"""
AND-105 Task 7: generate_explanations.py
Generates 2-3 sentence risk explanations for all HIGH-risk elevators using Ollama
(mistral:7b) and writes them to the predictions.risk_explanation column.

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
MODEL       = 'mistral:7b'
CONCURRENCY = 4          # parallel Ollama requests
BATCH_SIZE  = 100        # elevators processed per DB round-trip
TIMEOUT     = 300        # seconds per Ollama call
DB_CTR      = 'rocket-elevators-ops-dashboard-db-1'
DB_USER     = 'api_user'
DB_NAME     = 'rocket_elevators'

# V4 hardened prompt — identical to notebook SYSTEM_V4
SYSTEM_PROMPT = '\n'.join([
    'You are an Ontario elevator safety analyst familiar with TSSA compliance requirements.',
    '',
    'Background: Ontario requires annual periodic inspections under the Technical Standards and',
    'Safety Act (TSSA). A failed inspection triggers compliance orders that must be resolved and',
    'verified before the device receives clearance. Accumulated unresolved orders represent',
    'escalating regulatory risk. Inspection types: Periodic (annual), Followup (post-failure), Other.',
    '',
    'Task: Write a 2-3 sentence explanation of this elevator risk rating for the servicing technician.',
    'Lead with the single most operationally significant risk factor.',
    'Cite specific dates and counts from the data. Do not mention risk scores or algorithms.',
    'Use only the information provided in this message. Do not add knowledge not present in the data.',
    'If a category has no records, state that explicitly rather than omitting or inferring.',
    'If all indicators are benign, state that directly — do not manufacture concerns to fill the required sentences.',
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
        # Escape single quotes for standard SQL quoting
        escaped = explanation.replace("'", "''")
        lines.append(
            f"UPDATE predictions SET risk_explanation = '{escaped}' "
            f"WHERE elevator_id = {eid};\n"
        )
    lines.append('COMMIT;\n')
    _psql(''.join(lines), via_stdin=True)


# ── User message formatter ────────────────────────────────────────────────────
def user_msg(elev):
    eid, etype, loc, level = (
        elev['elevator_id'], elev['elevator_type'],
        elev['location'], elev['risk_level'],
    )
    lines = [
        f'Elevator {eid} — {etype} at {loc}',
        f'Risk Level: {level}',
        '',
        'Recent inspections (most recent first):',
    ]
    if elev['inspections']:
        for i in elev['inspections']:
            d = i.get('date') or 'unknown'
            t = i.get('inspection_type') or 'unknown'
            o = i.get('outcome', '')
            lines.append(f'  {d} | {t} | {o}')
    else:
        lines.append('  No inspections on record')

    lines += ['', 'Incidents in past 2 years:']
    if elev['incidents']:
        for i in elev['incidents']:
            lines.append(
                f"  {i.get('date') or 'unknown'} | "
                f"{i.get('category') or 'unknown'} | "
                f"severity: {i.get('injury_severity') or 'none'}"
            )
    else:
        lines.append('  None')

    lines += ['', 'Recent alterations:']
    if elev['alterations']:
        for a in elev['alterations']:
            lines.append(
                f"  {a.get('alteration_type') or 'unknown'} | "
                f"status: {a.get('status') or 'unknown'}"
            )
    else:
        lines.append('  None')

    return '\n'.join(lines)


# ── Async Ollama call (thread-pool via asyncio.to_thread) ────────────────────
def _call_ollama_sync(system, user):
    """Blocking Ollama call — run in thread pool via asyncio.to_thread."""
    payload = {
        'model':   MODEL,
        'stream':  False,
        'options': {'temperature': 0.1, 'num_predict': 200},
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


# ── Batch context fetch ───────────────────────────────────────────────────────
def fetch_context_batch(elevator_ids):
    """Fetch inspections, incidents, alterations for a list of elevator IDs in 3 queries."""
    ids_str = ','.join(str(i) for i in elevator_ids)
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

        # Build async tasks — CONCURRENCY simultaneous Ollama calls via thread pool
        tasks = [
            call_ollama(semaphore, SYSTEM_PROMPT, user_msg(e))
            for e in augmented
        ]
        # return_exceptions=True prevents CancelledError (Ctrl+C) from discarding
        # completed work — exceptions are returned as values, not raised
        results = await asyncio.gather(*tasks, return_exceptions=True)

        # Collect updates and count errors
        cancelled = False
        updates = {}
        for e, explanation in zip(augmented, results):
            eid = e['elevator_id']
            if isinstance(explanation, asyncio.CancelledError):
                cancelled = True  # save completed work first, re-raise after
            elif isinstance(explanation, BaseException):
                errors += 1
                print(f'  ✗ {eid}: {type(explanation).__name__}: {explanation}', flush=True)
            elif explanation.startswith('[ERROR:'):
                errors += 1
                print(f'  ✗ {eid}: {explanation}', flush=True)
            else:
                updates[eid] = explanation

        # Write to DB
        try:
            update_batch(updates)
        except RuntimeError as exc:
            errors += len(updates)
            print(f'  ✗ batch write failed — {len(updates)} explanations lost: {exc}', flush=True)
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

    print(f'HIGH elevators total:    {t:>6}')
    print(f'  With explanation:      {e:>6}  ({e/t*100:.1f}%)' if t else '')
    print(f'  Without explanation:   {n:>6}')

    # Spot-check 3 random explained rows
    samples = query_rows("""
        SELECT p.elevator_id, p.risk_level, p.risk_score::float8,
               LEFT(p.risk_explanation, 200) AS explanation_preview
        FROM predictions p
        WHERE p.risk_level = 'HIGH' AND p.risk_explanation IS NOT NULL
        ORDER BY RANDOM() LIMIT 3
    """)
    if samples:
        print()
        print('Spot-check (3 random):')
        for s in samples:
            print(f"  Elevator {s['elevator_id']} ({s['risk_level']}, {float(s['risk_score']):.4f}):")
            print(f"    {s['explanation_preview']}")
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
    est_hours = len(elevators) * 12.5 / CONCURRENCY / 3600
    print(f'Estimated:   ~{est_hours:.1f}h at 12.5s/call × {CONCURRENCY} concurrent')
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
