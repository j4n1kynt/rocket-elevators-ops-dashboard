# FastMCP Server — Security Audit
**AND-107 | Re-audited: 2026-06-19 | Scope:** `platform/mcp/`

---

## Methodology

AND-107 requires treating every MCP tool input the same way a web API treats user
input — validate and sanitise everything, because the arguments arrive from a
language model, which is effectively untrusted input. This audit was performed by
reading every SQL query, every user-supplied parameter path, and every external
resource access across all tool files, plus the server lifespan, the connection
pool, and the RAG layer.

This is a **re-audit**. The implementation has changed materially since the first
pass (2026-06-16); the substantive deltas are listed below so the two can be
reconciled. All seven categories still pass.

### What changed since the last audit

| Area | Then | Now |
|---|---|---|
| Incident narrative search | PostgreSQL full-text search (`plainto_tsquery`, `ts_headline`) | ChromaDB semantic search (`incident_narratives` collection) — **no SQL at all** (FOUNDATION-1b) |
| Input validation | `tools/_validators.py` (standalone functions) | `tools/models.py` (per-tool Pydantic models, `strict=True`) |
| Bool-as-int defence | explicit `isinstance(v, bool)` check before the int check | handled by Pydantic **strict mode** — `True`/`1` are rejected at the type layer |
| Connection pool | psycopg2 `ThreadedConnectionPool` + double-checked `threading.Lock` | asyncpg pool created once in the lifespan; release handled by `async with` |
| SQL placeholders | psycopg2 named `%(name)s` | asyncpg positional `$1, $2, …` |
| Skip-confirmation flag | (not present) | `MCP_SKIP_CONFIRMATION` exists for tests, but `_guard_skip_confirmation()` **refuses to start** if it is set in production |

---

## 1. SQL Injection

### Findings

All SQL across `inspection_tools.py`, `incident_tools.py`, and `write_tools.py`
uses asyncpg **positional placeholders** (`$1, $2, …`) exclusively. There are zero
f-strings, zero string concatenation, and zero `%`-style formatting in any SQL
string — confirmed by a repository-wide search for `f"SELECT`/`f"INSERT`, `.format(`,
and `%(`, which returned no matches in `platform/mcp/`.

| File | Function | User inputs | Parameterised |
|------|----------|-------------|---------------|
| inspection_tools.py | `get_tssa_shutdown_elevators` | `limit` | ✅ `$1` |
| inspection_tools.py | `get_inspection_history` | `elevator_id`, `limit` | ✅ `$1`, `$2` (EXISTS check + SELECT) |
| inspection_tools.py | `get_elevators_needing_followup` | `limit` | ✅ `$1` |
| inspection_tools.py | `get_elevator_risk` | `elevator_id` | ✅ `$1` (EXISTS check + SELECT) |
| inspection_tools.py | `get_fleet_stats` | none | ✅ no params (3 aggregate queries) |
| incident_tools.py | `get_incident_count_last_year` | none | ✅ no params (date anchor is a hardcoded `DATE '2016-11-22'` literal) |
| incident_tools.py | `get_elevator_incidents` | `elevator_id`, `limit` | ✅ `$1`, `$2` |
| rag_tools.py | `search_maintenance_docs` | none reach SQL | ✅ ChromaDB only — no SQL |
| rag_tools.py | `search_incident_narratives` | none reach SQL | ✅ ChromaDB only — no SQL |
| write_tools.py | `schedule_inspection` (EXISTS/SELECT) | `elevator_id` | ✅ `$1` |
| write_tools.py | `schedule_inspection` (INSERT inspections) | `elevator_id`, `inspection_type`, `inspection_date` | ✅ `$1`, `$2`, `$3` (`inspection_id` from `nextval('mcp_inspection_id_seq')`) |
| write_tools.py | `schedule_inspection` (INSERT audit) | `elevator_id`, `inspection_id`, `inspection_date`, `inspection_type`, `reason` | ✅ `$1`–`$5` |
| write_tools.py | `_audit_error` (INSERT audit) | `elevator_id`, `inspection_date`, `inspection_type`, `reason`, `error_message` | ✅ `$1`–`$5` |

**Hardcoded SQL values** (not user-controlled, all safe): outcome literals
(`'passed'`, `'all orders resolved'`, `'follow up'`, `'Pending'`), the write
constants (`'success'`, `'error'`), the sequence name `mcp_inspection_id_seq`, the
date literal `DATE '2016-11-22'`, and all column/table names and sort directions.

### Verdict: ✅ PASS — No SQL injection surface

> **Note on narrative search.** `search_incident_narratives` no longer touches
> PostgreSQL. FOUNDATION-1b moved it to ChromaDB semantic search over the
> `incident_narratives` collection. The previous audit's `plainto_tsquery`
> reasoning is therefore obsolete and has been removed — there is now no FTS query
> to defend. ChromaDB query safety is covered in §3.

---

## 2. Input Validation

### Architecture

Every tool that takes user-facing parameters constructs a per-tool Pydantic model
(`tools/models.py`) as the first statement in its body, **before** any database or
ChromaDB call. All models use `ConfigDict(strict=True)`, so there is no implicit
type coercion: a string where an int is expected, or a non-`bool` for `confirmed`,
is rejected at the type layer before any validator runs. This sits in front of, and
is independent of, SQL parameterisation — two layers of defence.

| Model / validator | Checks |
|---|---|
| `_check_elevator_id(v)` | `> 0` and `≤ 9,999,999` |
| `_check_limit(v, max_val)` | `1 ≤ v ≤ max_val` (max is 50/100/200 for DB tools, 20 for search tools) |
| `_strip_nonempty(v, field, max_len)` | is `str`, non-empty after strip, `≤ max_len` |
| `validate_inspection_date` | parses `YYYY-MM-DD`, rejects dates in the past |
| `validate_inspection_type` | `""` or one of the five allowed values, **case-sensitive** |
| `validate_reason` | non-empty after strip, `≤ 500` chars |
| `confirmed` | strict `bool` — `"yes"` and `1` are rejected at the type layer |

### Bool-disguised-as-integer

`bool` is a subclass of `int` in Python, so `isinstance(True, int)` is `True`. In
the previous implementation this was defended with an explicit `isinstance(v, bool)`
check. That check is now **unnecessary**: Pydantic strict mode does not accept a
`bool` for an `int` field, so `True`/`False` never reach `_check_elevator_id` or
`_check_limit`. The defence moved from hand-written code to the type layer.

### Test coverage

`tools/test_validation.py` asserts that for every tool, invalid input raises
`ValidationError` **and** the data-access layer (`get_connection` / `rag_query`) is
never called — i.e. validation runs strictly before any I/O. Cases covered include
out-of-range IDs, zero/over-max limits, empty/whitespace/over-length strings,
past and malformed dates, and non-strict `confirmed` values (`"yes"`, `1`).

### Verdict: ✅ PASS — Two independent layers (strict Pydantic typing/range + parameterised execution)

---

## 3. ChromaDB Query Safety (`rag.py`, `rag_tools.py`)

ChromaDB does not use SQL; the injection model is different. The risks are a
malformed `where` filter, an unbounded `n_results`, or an attacker-controlled
collection name.

**Mitigations in place:**
- `query_text` is validated (max 500 chars for maintenance docs, 200 for incident
  narratives) and stripped before it is embedded.
- `n_results` / `limit` are bounded to `[1, 20]` by the model, so a query cannot ask
  ChromaDB for an unbounded result set.
- The `where` filter in `search_maintenance_docs` is built only from the
  module-level constant `MAINTENANCE_SOURCE_TYPE` (currently `None` → no filter) —
  **never from user input**. `search_incident_narratives` passes no `where` filter.
- `collection_name` is never user-controlled. Both collections
  (`maintenance_documents`, `incident_narratives`) are module-level constants.
- `rag_query` and both search tools wrap their bodies in `try/except` and surface
  errors as structured messages rather than raw tracebacks.
- `rag.py` additionally guards against a stale index: it checks both the embedding
  **dimension** and the stored **`model_version`** metadata against the current
  model, raising a clear rebuild instruction on mismatch. This is a correctness
  guard, not strictly a security one, but it prevents silently meaningless results.

**Verdict: ✅ PASS**

---

## 4. Write Operation Safety (`write_tools.py`, `server.py`)

`schedule_inspection` is the only tool that writes to the database.

- **Two-phase confirmation.** Phase 1 (`confirmed=False`) validates, checks the
  elevator exists, and returns a summary — **no write**. Phase 2 (`confirmed=True`)
  re-validates, re-checks, then inserts.
- **`confirmed` is a strict `bool`** — the model rejects `"yes"`/`1`, so the LLM
  cannot bypass the gate by passing a truthy non-bool.
- **Transactional write.** The `INSERT` into `inspections` and the matching
  `scheduling_audit_log` row run inside a single `async with conn.transaction():`,
  so an audit row can never diverge from the inspection it records.
- **Duplicate guard.** A partial unique index (`uq_pending_inspection`, scoped to
  `inspection_id >= 9000000`) makes a replay/double-submit raise
  `UniqueViolationError`, which the tool turns into a clear "already scheduled — no
  duplicate created" message instead of writing a second row.
- **ID isolation.** New IDs come from `mcp_inspection_id_seq` (starts at 9,000,000),
  far above the imported source-data range, so chatbot-created rows never collide
  with TSSA records.
- **Error auditing.** A *confirmed* attempt that fails mid-write is recorded
  best-effort via `_audit_error()` on a separate connection that never raises — an
  audit failure cannot mask the original error.

**`MCP_SKIP_CONFIRMATION` (test-only) is now guarded.** Setting this variable
bypasses Phase 1 and writes on the first call. It exists only for CI/tests. As of
this re-audit, `_guard_skip_confirmation()` runs in the server lifespan and
**refuses to start** if the variable is set while `APP_ENV`/`ENV`/`ENVIRONMENT` is
`production` or `prod`; in any other environment it logs a loud warning. This
closes the previous risk that the flag could be left on in a deployed environment.

**Verdict: ✅ PASS**

---

## 5. Environment Variable Handling (`db.py`)

- `_get_dsn()` reads `DATABASE_URL` first, then falls back to individual `DB_*`
  vars (same precedence as `etl_to_database.py` and `platform/api/db.go`).
- If neither is configured, it raises `RuntimeError` listing the **names** of the
  missing variables — never their values, and no `KeyError` leaks to the caller.
- Credentials are not logged. The connect-failure path wraps asyncpg's exception
  text, which describes the failure (e.g. connection refused) and does not echo the
  password.

**Verdict: ✅ PASS**

---

## 6. Connection Pool Safety (`db.py`)

The pool is now asyncpg, not psycopg2 — the previous threading-based reasoning no
longer applies.

- The pool is created **once** in `init_pool()` from the FastMCP lifespan, before
  the HTTP port opens. Because creation is single-shot at startup, there is no
  first-request race and **no double-checked locking is needed**.
- `init_pool()` runs a `SELECT 1` health check and **fails fast**: if PostgreSQL is
  unreachable, the server refuses to start instead of dying on the first tool call.
- `get_connection()` is an `async with _pool.acquire()` context manager. asyncpg
  returns the connection to the pool automatically on exit — including on exception
  — and resets its state, so connections cannot leak or carry transaction state
  between tool calls.
- `get_connection()` raises a clear `RuntimeError` if it is somehow called before
  `init_pool()` ran.
- The startup DDL (`CREATE SEQUENCE`/audit table/unique index) tolerates a
  restricted DB role: `InsufficientPrivilegeError` is caught so read tools still
  work on a least-privilege connection.

**Verdict: ✅ PASS**

---

## 7. Singleton Initialization Safety (`rag.py`)

- `_get_client()` and `_get_model()` both use `threading.Lock()` with
  double-checked locking — correct here because, unlike the DB pool, these are
  initialised lazily on first request rather than at startup.
- The heavy imports (`chromadb`, `sentence_transformers`) are inside these
  functions, so a missing optional dependency cannot take down the whole server at
  import time — it surfaces as a clear `RuntimeError` only if a RAG tool is called.
- Both raise `RuntimeError` with actionable messages if initialisation fails
  (missing ChromaDB store, missing model weights).

**Verdict: ✅ PASS**

---

## 8. Known Limitations & Accepted Risks

| Item | Risk | Mitigation | Status |
|------|------|-----------|--------|
| No MCP-layer authentication; server binds `0.0.0.0` | Any process that can reach the port can call the tools | Deployed behind the network/firewall boundary; not publicly exposed. Revisit (add auth) if the port is ever reachable from untrusted networks. | **Accepted** (internal tool) |
| Tool exceptions are returned as `{"error": true, "message": str(exc)}` | Internal error text (e.g. a DB error string) is returned to the calling layer and could surface to a user | Consumed by the Go API, which strips Pydantic boilerplate (`cleanValidationError()`) before display; messages contain no credentials | **Accepted** (low value to an attacker) |
| `MCP_SKIP_CONFIRMATION` disables the write confirmation gate | A write with no human approval | Refused at startup in `production`/`prod`; loud warning elsewhere; test-only | **Mitigated** |
| `ALLOW_EMPTY_CHROMADB` makes `rag_query` return `[]` instead of raising on a missing collection | A misconfigured index could silently return "no results" rather than erroring | CI/dev affordance only; do not set in production so a missing index fails loudly | **Accepted** (test/CI) |
| `narrative`/large text excluded from `get_elevator_incidents` | Large text not returned by default | Narrative search handled by `search_incident_narratives` | **By design** |
| `schedule_inspection` IDs come from a sequence, not an FK to source data | Not formally constrained to source records | Sequence starts at 9,000,000 — far above the imported-data range — and a partial unique index blocks duplicate pending rows | **Accepted** |
| Incident narrative text returned by search may contain incident detail | Operational incident descriptions surfaced to the chatbot | This is the intended use case (operations assistant); no data beyond what the operator is already authorised to see | **By design** |

---

## Summary

| Category | Result |
|----------|--------|
| SQL Injection | ✅ PASS |
| Input Validation | ✅ PASS |
| ChromaDB Query Safety | ✅ PASS |
| Write Operation Safety | ✅ PASS |
| Environment Variable Handling | ✅ PASS |
| Connection Pool Safety | ✅ PASS |
| Singleton Initialization Safety | ✅ PASS |

**All security checks pass.** No vulnerabilities were found in the current
implementation. The most significant change since the previous audit is that
incident narrative search no longer issues any SQL (it is ChromaDB-backed), which
removes the only full-text-search surface the prior audit had to reason about; and
the `MCP_SKIP_CONFIRMATION` test flag is now actively blocked from running in
production.
