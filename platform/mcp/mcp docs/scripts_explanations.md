# FastMCP Server — Scripts Explanation
**AND-107 | Date:** 2026-06-16

Detailed purpose and design rationale for every file in `platform/mcp/`.

---

## `server.py`

**What it does:**
The entry point for the entire MCP server. It creates the `FastMCP` application instance, registers all 10 tools by passing each function to `mcp.tool()`, and starts the Streamable HTTP server on the port defined by `MCP_PORT` (default 8765).

**Why it exists:**
FastMCP requires a single app object that knows about every available tool. `server.py` is the assembly point — it imports from all four tool files and wires them into the app. It also defines the server lifespan, which initializes the asyncpg connection pool (and runs the startup health check) before the first tool call is accepted.

**How to run:**
```bash
py -3 -m platform.mcp.server
```

**Key decisions:**
- Registers tools in two labeled groups (read vs. write) to make the list easier to scan.
- `host="0.0.0.0"` allows both local and container-to-container connections. Change to `127.0.0.1` to restrict to localhost only.
- `load_dotenv()` is called here so environment variables are available to all imports.
- The `lifespan` async context manager calls `init_pool()` on startup and `close_pool()` on shutdown. If PostgreSQL is unreachable at startup, `init_pool()` raises `RuntimeError` and the server refuses to start rather than failing silently on the first tool call.

---

## `db.py`

**What it does:**
Manages a single shared asyncpg connection pool for the entire server process. Exposes three public functions: `init_pool()` and `close_pool()` for lifecycle management (called from the FastMCP lifespan), and the `get_connection()` async context manager, which tools use to borrow a connection, execute queries, and automatically return the connection when done.

**Why it exists:**
Every tool needs database access but should not manage its own connections. Opening and closing a new connection per tool call would be slow and would exhaust PostgreSQL's connection limit quickly. A shared pool (min_size=2, max_size=10) amortizes the connection cost across all tool calls.

**Key functions:**

| Function | Purpose |
|----------|---------|
| `_get_dsn()` | Builds the connection string. Tries `DATABASE_URL` first (Neon/remote), falls back to individual `DB_*` vars (local Docker) and assembles a `postgresql://` URL. Raises `RuntimeError` with a list of missing variables if neither is configured. |
| `init_pool()` | Creates the asyncpg pool, runs a `SELECT 1` health check, and creates the inspection ID sequence. Called once from the FastMCP lifespan at startup. Raises `RuntimeError` if the database is unreachable — the server will not start. |
| `close_pool()` | Drains and closes the pool. Called from the FastMCP lifespan at shutdown. |
| `get_connection()` | Async context manager. Acquires a connection from the pool, yields it to the caller, and returns it to the pool automatically on exit. |

**Why asyncpg instead of psycopg2:**
The MCP server runs on uvicorn, which operates a single-threaded async event loop. psycopg2 is synchronous — every database call would block the event loop and prevent other tool calls from running concurrently. asyncpg is a native async PostgreSQL driver that integrates directly with the event loop, allowing true concurrent execution of multiple tool calls.

**Inspection ID sequence:**
`_SEQUENCE_DDL` (creating `mcp_inspection_id_seq` starting at 9,000,000) is defined here and executed inside `init_pool()`. This keeps write-related DDL decoupled from `write_tools.py` and guarantees the sequence exists before any tool call is accepted.

---

## `rag.py`

**What it does:**
Provides a single function, `rag_query()`, that takes a plain-text query string, generates a vector embedding for it, and retrieves the most semantically similar chunks from the ChromaDB collection. Returns a list of dicts with the chunk text, metadata, and cosine distance.

**Why it exists:**
`rag_tools.py` needs to search ChromaDB, but it should not manage the ChromaDB client or the embedding model directly. `rag.py` encapsulates both as lazy singletons — the ~1.3GB SentenceTransformer model is loaded once on first use and reused for every subsequent query.

**Key constants (must stay in sync with `rag_preprocessing.py`):**

| Constant | Value | Why it must match |
|----------|-------|-------------------|
| `CHROMADB_PATH` | `data/chromadb` (or `RAG_CHROMADB_PATH`) | Must point to the same store that was written during preprocessing |
| `COLLECTION_NAME` | `maintenance_documents` | Must match the collection name used in `rag_preprocessing.py` |
| `EMBEDDING_MODEL` | `BAAI/bge-large-en-v1.5` | Must match the model used to generate the stored embeddings |

**Why `normalize_embeddings=False`:**
`rag_preprocessing.py` encodes document chunks with `normalize_embeddings=False`. If query embeddings were encoded with `normalize_embeddings=True`, the vector magnitudes would differ and cosine similarity scores would be incorrect. Both sides must use the same setting.

**Thread safety:**
Both `_get_client()` and `_get_model()` use `threading.Lock()` with double-checked locking — the same pattern as `db.py` — to prevent concurrent initialization during the first request.

---

## `tools/_validators.py`

**What it does:**
A collection of five pure validation functions used by every tool before any database or ChromaDB call is made. Validates types, ranges, string lengths, date formats, and business rules.

**Why it exists:**
AND-107 requires treating every tool input the same way a web API treats user input. Centralizing validation in one file means:
- The same rules are applied consistently across all tools.
- Security testing is easier — you test one file, not ten functions spread across four files.
- No tool can accidentally skip validation by forgetting to call it inline.

**Functions:**

| Function | What it validates |
|----------|------------------|
| `validate_elevator_id(v)` | Must be an `int` (not `bool`), positive, and ≤ 9,999,999. The `bool` check is explicit because Python's `bool` is a subclass of `int` — without it, `True` would pass as elevator ID 1. |
| `validate_limit(v, max_val)` | Must be an `int` (not `bool`), between 1 and `max_val`. Each tool passes its own `max_val` (50, 100, or 200 depending on the query). |
| `validate_query_string(v, max_len)` | Must be a non-empty string after stripping whitespace, within `max_len` characters. Prevents oversized inputs from being sent to ChromaDB or PostgreSQL FTS. |
| `validate_inspection_date(v)` | Must be a string in `YYYY-MM-DD` format and must not be in the past. Rejects malformed dates before they reach the database. |
| `validate_reason(v)` | Must be a non-empty string after stripping, within 500 characters. Used by `schedule_inspection`. |

---

## `tools/inspection_tools.py`

**What it does:**
Implements five tools that cover the core elevator fleet queries: shutdown/compliance status, inspection history, follow-up lists, risk predictions, and fleet-wide statistics.

**Why it exists:**
These five tools all touch the `elevators`, `inspections`, and `predictions` tables — the same three tables the Go API's `GetElevators`, `GetFleetStats`, and `GetFleetAlerts` handlers query. Grouping them together mirrors the Go API's structure.

**Tools and their SQL patterns:**

| Tool | SQL pattern | Key detail |
|------|------------|------------|
| `get_tssa_shutdown_elevators` | CTE with `DISTINCT ON` to get latest inspection per elevator, then filter non-passing outcomes | No "shutdown" column exists in the schema — non-passing outcomes are the closest proxy |
| `get_inspection_history` | EXISTS check first, then paginated SELECT on inspections | Returns `{"found": False}` if elevator doesn't exist vs empty inspections list |
| `get_elevators_needing_followup` | Same CTE pattern, filter `LOWER(outcome) = 'follow up'` | Sorted oldest-first so operations prioritize the most overdue |
| `get_elevator_risk` | Two-step: EXISTS on elevators, then SELECT on predictions | Distinguishes "elevator not found" from "no prediction available" |
| `get_fleet_stats` | Three separate aggregate queries | No user inputs — zero injection surface; mirrors `GetFleetStats` in `handlers.go` exactly |

---

## `tools/incident_tools.py`

**What it does:**
Implements two tools that query the `incidents` table: a fleet-wide count for the previous calendar year, and a per-elevator incident list.

**Why it exists:**
Incident data is a separate concern from inspection data — different table, different join patterns, different business questions. Keeping it in its own file makes it easy to extend (e.g., add a severity breakdown tool) without touching `inspection_tools.py`.

**Tools:**

| Tool | Key detail |
|------|-----------|
| `get_incident_count_last_year` | No user parameters — date range computed entirely server-side using `CURRENT_DATE`. Zero injection surface. Returns total, fatal, and injury-related counts. |
| `get_elevator_incidents` | `narrative` column is intentionally excluded from results — it can be hundreds of words per row. Use `search_incident_narratives` for narrative text search. |

---

## `tools/rag_tools.py`

**What it does:**
Implements two search tools: semantic search over the ChromaDB maintenance document collection, and full-text search over incident narrative text stored in PostgreSQL.

**Why it exists:**
Document search requires a different access pattern from structured queries — ChromaDB for semantic matching and PostgreSQL FTS for keyword-based narrative search. Isolating these in one file makes the RAG layer easy to swap or extend (e.g., adding narrative embeddings to ChromaDB in FOUNDATION-1b).

**Tools:**

| Tool | Backend | Key detail |
|------|---------|-----------|
| `search_maintenance_docs` | ChromaDB via `rag.py` | Source format is `.txt` files (not PDF). `MAINTENANCE_SOURCE_TYPE` is `None` until `rag_preprocessing.py` is updated for `.txt` ingestion — currently searches all indexed documents. |
| `search_incident_narratives` | PostgreSQL FTS | Uses `plainto_tsquery` which treats the query as plain text — immune to tsquery syntax injection. `ts_headline` returns a highlighted excerpt from the narrative with matched terms marked by `**`. |

**`MAINTENANCE_SOURCE_TYPE` constant:**
This constant in `rag_tools.py` controls the `where` filter sent to ChromaDB. It is currently `None` (no filter). When the `.txt` preprocessing pipeline is implemented, set it to match the `source_type` metadata value written during ingestion (e.g. `"maintenance_txt"`).

---

## `tools/write_tools.py`

**What it does:**
Implements the only write tool in the server: `schedule_inspection`. It inserts a new row into the `inspections` table with `outcome = 'Pending'` and `inspection_type = 'Scheduled'`, but only after a mandatory two-phase confirmation flow.

**Why it exists:**
Write tools require stricter safety guarantees than read tools. Isolating the only write operation in its own file makes it immediately obvious which file to audit when reviewing data-modification risk. It also contains `_ensure_sequence()`, which creates the PostgreSQL sequence for ID generation at server startup.

**Two-phase confirmation:**

```
Phase 1 — confirmed=False (default):
  Validates all inputs → checks elevator exists → returns a plain-text
  summary string with all details → NO database write.

Phase 2 — confirmed=True:
  Re-validates all inputs → re-checks elevator exists → INSERTs row
  → returns new inspection_id and confirmation details.
```

The LLM presents the Phase 1 summary to the user ("You are about to schedule... Please confirm.") before calling Phase 2. `confirmed` must be a strict Python `bool` — `"yes"`, `1`, or any other truthy value raises `ValueError` immediately.

**Inspection ID sequence:**
The `inspections` table uses `INTEGER PRIMARY KEY` (not `SERIAL`), so PostgreSQL won't auto-generate IDs. Real TSSA inspection records loaded by the ETL pipeline have IDs up to ~143,181. The sequence `mcp_inspection_id_seq` starts at 9,000,000 — far above that range — ensuring MCP-scheduled inspections never collide with source data. The sequence DDL and its creation are now handled in `db.init_pool()` at server startup (see `db.py`).

---

## `requirements.txt`

**What it does:**
Declares the Python dependencies needed specifically for the MCP server. Intentionally separate from the root `requirements.txt`.

**Why it is separate:**
The root `requirements.txt` installs Flask, gunicorn, pdfplumber, and tiktoken — none of which the MCP server needs. A separate file produces a leaner install and allows the MCP server to have its own Docker layer that does not rebuild when Flask dependencies change.

**Dependencies:**

| Package | Version | Reason |
|---------|---------|--------|
| `fastmcp` | 2.5.2 | MCP framework — tool registration, protocol handling |
| `uvicorn` | 0.34.3 | ASGI server — required by Streamable HTTP transport |
| `asyncpg` | 0.30.0 | Async PostgreSQL driver — native async I/O for uvicorn's event loop |
| `chromadb` | 1.5.9 | Vector store client — matches root `requirements.txt` |
| `sentence-transformers` | 5.6.0 | Embedding model — must match `rag_preprocessing.py` |
| `python-dotenv` | 1.1.0 | Loads `.env` at startup |
| `numpy` | 2.4.4 | Transitive dep of `sentence-transformers` — pinned to match root |

---

## `security_audit.md`

**What it does:**
Documents the results of the SQL injection and input safety audit performed on all tool files. Covers seven categories: SQL injection, input validation, ChromaDB query safety, write operation safety, environment variable handling, connection pool safety, and singleton initialization safety.

**Why it exists:**
AND-107 explicitly requires testing the MCP server for vulnerabilities and documenting findings. This file is the deliverable for that requirement. All checks passed — see the file for full details and the table of accepted risks.

---

## `__init__.py` (two files)

**What they do:**
Both are empty files. `platform/mcp/__init__.py` makes `platform/mcp` a Python package so the server can be started with `py -3 -m platform.mcp.server`. `platform/mcp/tools/__init__.py` makes the `tools/` subdirectory a package so tool modules can be imported with `from platform.mcp.tools.inspection_tools import ...`.

**Why they are needed:**
Python requires `__init__.py` files for directory-based package imports. Without them, `python -m platform.mcp.server` would fail with `ModuleNotFoundError`.
