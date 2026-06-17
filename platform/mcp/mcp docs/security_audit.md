# FastMCP Server — Security Audit
**AND-107 | Date:** 2026-06-16 | **Scope:** `platform/mcp/`

---

## Methodology

AND-107 requires treating every MCP tool input the same way a web API treats user input — validate and sanitize everything. This audit was performed by reviewing every SQL query, every user-supplied parameter path, and every external resource access across all tool files.

---

## 1. SQL Injection

### Findings

All 11 queries across `inspection_tools.py` and `incident_tools.py` use psycopg2's `%(name)s` named-parameter syntax exclusively. Zero f-strings, zero string concatenation, zero `%s % value` old-style formatting in SQL.

| File | Function | User Inputs | Parameterized |
|------|----------|-------------|---------------|
| inspection_tools.py | `get_tssa_shutdown_elevators` | `limit` | ✅ `%(limit)s` |
| inspection_tools.py | `get_inspection_history` | `elevator_id`, `limit` | ✅ `%(eid)s`, `%(limit)s` |
| inspection_tools.py | `get_elevators_needing_followup` | `limit` | ✅ `%(limit)s` |
| inspection_tools.py | `get_elevator_risk` | `elevator_id` | ✅ `%(eid)s` |
| inspection_tools.py | `get_fleet_stats` | none | ✅ no params needed |
| incident_tools.py | `get_incident_count_last_year` | none | ✅ no params needed |
| incident_tools.py | `get_elevator_incidents` | `elevator_id`, `limit` | ✅ `%(eid)s`, `%(limit)s` |
| rag_tools.py | `search_maintenance_docs` | none (ChromaDB) | ✅ no SQL |
| rag_tools.py | `search_incident_narratives` | `query`, `limit` | ✅ `%(q)s`, `%(limit)s` |
| write_tools.py | `schedule_inspection` (EXISTS) | `elevator_id` | ✅ `%(eid)s` |
| write_tools.py | `schedule_inspection` (INSERT) | `elevator_id`, `inspection_date` | ✅ `%(elevator_id)s`, `%(inspection_date)s` |

**Hardcoded SQL values** (not user-controlled, all safe):
- String literals: `'passed'`, `'all orders resolved'`, `'follow up'`, `'none'`, `'Scheduled'`, `'Pending'`, `'maintenance_pdf'`
- Column names, table names, sort directions, SQL functions: all static

### Verdict: ✅ PASS — No SQL injection surface

---

## 2. Input Validation

### Architecture

All tool inputs are validated in `tools/_validators.py` before reaching any database or ChromaDB call. This provides a first layer of defence independent of parameterization.

| Validator | Checks |
|-----------|--------|
| `validate_elevator_id(v)` | Type is `int` (not `bool`), `> 0`, `≤ 9,999,999` |
| `validate_limit(v, max_val)` | Type is `int` (not `bool`), `≥ 1`, `≤ max_val` |
| `validate_query_string(v, max_len)` | Type is `str`, stripped non-empty, `≤ max_len` chars |
| `validate_inspection_date(v)` | Type is `str`, valid `YYYY-MM-DD`, `≥ today` |
| `validate_reason(v)` | Type is `str`, stripped non-empty, `≤ 500` chars |

### Boolean-disguised-as-integer check

`isinstance(value, bool)` is checked explicitly before `isinstance(value, int)` because in Python `bool` is a subclass of `int` — `isinstance(True, int)` returns `True`. Without the bool check, `True` (= 1) and `False` (= 0) would pass as valid elevator IDs.

### Verdict: ✅ PASS — Two independent validation layers (type/range + parameterized execution)

---

## 3. ChromaDB Query Safety (`rag.py`)

ChromaDB does not use SQL. Injection risk is different — a malformed `where` filter dict could cause a `chromadb.errors` exception or unexpected results.

**Mitigations in place:**
- `query_text` is validated via `validate_query_string()` before being embedded — length-capped at 500 chars
- `where` dicts are only constructed internally in `rag_tools.py` from the `MAINTENANCE_SOURCE_TYPE` constant — never built from raw user input
- `MAINTENANCE_SOURCE_TYPE` is `None` until the `.txt` ingestion pipeline sets the correct value; when `None`, no `where` filter is applied (searches all indexed documents)
- All ChromaDB calls are wrapped in `try/except` with meaningful error messages

**Source format note:** The RAG pipeline uses `.txt` files, not PDFs. The `source_type` metadata value in ChromaDB will be set when `rag_preprocessing.py` is updated for `.txt` ingestion. Update `MAINTENANCE_SOURCE_TYPE` in `rag_tools.py` at that time.

**Verdict: ✅ PASS**

---

## 4. Write Operation Safety (`write_tools.py`)

`schedule_inspection` is the only tool that writes to the database. Two-phase confirmation is enforced:

- **Phase 1** (`confirmed=False`): validates all inputs, checks elevator exists, returns a confirmation string — **no database write**
- **Phase 2** (`confirmed=True`): re-validates all inputs, re-checks elevator exists, then INSERTs — **write only after explicit confirmation**

`confirmed` parameter is validated as a strict `bool` — the LLM cannot bypass the confirmation step by passing `"yes"` or `1`.

`inspection_id` is generated via a PostgreSQL sequence (`mcp_inspection_id_seq`, starts at 9,000,000) created with `CREATE SEQUENCE IF NOT EXISTS` at server startup — IDs never overlap with source-data records.

**Verdict: ✅ PASS**

---

## 5. Environment Variable Handling (`db.py`)

- `_get_dsn()` checks for all required env vars upfront and raises `RuntimeError` with a clear message listing which vars are missing — no `KeyError` propagates to the caller
- `DATABASE_URL` takes priority over individual `DB_*` vars (matches `etl_to_database.py` and `platform/api/db.go`)
- Credentials are never logged or included in exception messages

**Verdict: ✅ PASS**

---

## 6. Connection Pool Safety (`db.py`)

- `ThreadedConnectionPool` is used (not `SimpleConnectionPool`) — thread-safe for Streamable HTTP concurrent calls
- `get_pool()` uses double-checked locking with `threading.Lock()` — prevents race condition on first initialization
- `get_connection()` context manager always calls `pool.putconn(conn)` in `finally` — connections are never leaked
- `conn.rollback()` on exception is wrapped in its own `try/except` — a broken connection during rollback does not mask the original error

**Verdict: ✅ PASS**

---

## 7. Singleton Initialization Safety (`rag.py`)

- `_get_client()` and `_get_model()` both use `threading.Lock()` with double-checked locking
- Both raise `RuntimeError` with actionable messages if initialization fails (missing ChromaDB store, missing model weights)

**Verdict: ✅ PASS**

---

## 8. Known Limitations & Accepted Risks

| Item | Risk | Mitigation | Status |
|------|------|-----------|--------|
| No MCP-layer authentication | Any process that can reach port 8765 can call tools | Deploy behind firewall / VPN; not exposed publicly | **Accepted** (internal dev tool) |
| `narrative` column excluded from `get_elevator_incidents` | Large text not returned by default | Narrative search handled by `search_incident_narratives` | **By design** |
| `schedule_inspection` uses a sequence starting at 9,000,000 | Not formally a FK to source data | Sequence is far above real data range (~43,002 max) | **Accepted** |
| FTS on `incidents.narrative` with `plainto_tsquery` | Malformed queries normalized by PostgreSQL | `plainto_tsquery` is immune to injection (treats input as plain text, not tsquery syntax) | **Safe** |

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

**All security checks pass.** No vulnerabilities found in the current implementation.
