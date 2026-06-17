# FastMCP Server — Project Structure
**AND-107 | Date:** 2026-06-16

Every decision in this file is final and reflects what was implemented.

---

## Context

AND-107 upgrades the chatbot from advisory-only (EVAL-1) to live data access.
The FastMCP server exposes 10 tools that query PostgreSQL and ChromaDB directly.

**Runtime architecture:**
```
Production:  User → Go API → MCP server → PostgreSQL / ChromaDB
Development: Claude Code → MCP server  (via .mcp.json, for debugging/tuning)
```

The Go API calls the MCP server as an HTTP client on behalf of the chatbot.
Claude Code can call the same tools directly during development — this is a
convenience, not the production path.

**Transport: Streamable HTTP** on `MCP_PORT` (default 8765).
Both the Go API and Claude Code connect via `http://localhost:8765/mcp`.

---

## Folder Tree

```
rocket-elevators-ops-dashboard/
│
├── .mcp.json                          # Claude Code MCP config (Streamable HTTP, port 8765)
├── .env.example                       # Updated — added MCP_CHROMADB_PATH + MCP_PORT
├── FastMCP_PStructure.md              # This file
│
└── platform/
    └── mcp/
        ├── __init__.py                # Empty — enables: py -3 -m platform.mcp.server
        ├── server.py                  # FastMCP app — registers all 10 tools, starts HTTP server
        ├── db.py                      # PostgreSQL ThreadedConnectionPool
        ├── rag.py                     # ChromaDB PersistentClient + SentenceTransformer query helper
        ├── requirements.txt           # MCP-only dependencies (7 packages)
        ├── security_audit.md          # SQL injection and input validation audit results
        └── tools/
            ├── __init__.py            # Empty
            ├── _validators.py         # Shared input validation (all tools import from here)
            ├── inspection_tools.py    # Tools 1, 2, 5, 6, 10
            ├── incident_tools.py      # Tools 3, 4
            ├── rag_tools.py           # Tools 7, 8
            └── write_tools.py         # Tool 9 — schedule_inspection (two-phase confirmation)
```

---

## File Purposes

| File | Purpose |
|------|---------|
| `server.py` | Entry point. Creates the `FastMCP` app, registers all 10 tools, calls `_ensure_sequence()` at startup, starts Streamable HTTP on `MCP_PORT`. |
| `db.py` | Manages a `ThreadedConnectionPool` (minconn=1, maxconn=10). `get_connection()` is a context manager used by every tool. Raises `RuntimeError` with clear messages on missing env vars or connection failure. |
| `rag.py` | Lazy-initialized ChromaDB `PersistentClient` and `SentenceTransformer` singletons with `threading.Lock()`. Exposes `rag_query()` used by `rag_tools.py`. |
| `requirements.txt` | MCP-only deps — does not include Flask, gunicorn, pdfplumber, or tiktoken. |
| `security_audit.md` | Documents SQL injection audit results, input validation coverage, ChromaDB safety, write operation safety, and known accepted risks. |
| `tools/_validators.py` | Five validation functions: `validate_elevator_id`, `validate_limit`, `validate_query_string`, `validate_inspection_date`, `validate_reason`. All tools call these before any DB access. |
| `tools/inspection_tools.py` | Tools 1, 2, 5, 6, 10 — fleet and inspection queries. |
| `tools/incident_tools.py` | Tools 3, 4 — incident count and per-elevator incidents. |
| `tools/rag_tools.py` | Tools 7, 8 — maintenance document search (ChromaDB) and incident narrative search (PostgreSQL FTS). |
| `tools/write_tools.py` | Tool 9 — `schedule_inspection` with two-phase confirmation and PostgreSQL sequence for ID generation. |

---

## `requirements.txt`

```
fastmcp==2.5.2
uvicorn==0.34.3
psycopg2-binary==2.9.9
chromadb==1.5.9
sentence-transformers==5.6.0
python-dotenv==1.1.0
numpy==2.4.4
```

Install: `pip install -r platform/mcp/requirements.txt`

- `fastmcp` — MCP framework
- `uvicorn` — ASGI server required by Streamable HTTP transport
- `psycopg2-binary` — same version as root `requirements.txt`
- `chromadb` — same version as root `requirements.txt`; shares `data/chromadb` store
- `sentence-transformers` — must match `EMBEDDING_MODEL = "all-MiniLM-L6-v2"` in `rag_preprocessing.py`
- `python-dotenv` — loads `.env` at startup
- `numpy` — pinned to match root `requirements.txt`

---

## `.env` Variables Added

```bash
MCP_CHROMADB_PATH=data/chromadb   # Path to ChromaDB store (relative to project root)
MCP_PORT=8765                      # Streamable HTTP port
```

PostgreSQL variables (`DATABASE_URL` / `DB_*`) are reused from the existing `.env` — no new DB vars needed.

Port allocation:
| Service | Port |
|---------|------|
| Go API | 8080 |
| Flask dashboard | 5000 |
| Ollama | 11434 |
| **FastMCP server** | **8765** |

---

## `.mcp.json`

```json
{
  "mcpServers": {
    "rocket-elevators": {
      "type": "http",
      "url": "http://localhost:8765/mcp"
    }
  }
}
```

The MCP server must be running before Claude Code can connect.
Start it with: `py -3 -m platform.mcp.server`

---

## 10 Tools

| # | Tool | File | Data source | Inputs |
|---|------|------|-------------|--------|
| 1 | `get_tssa_shutdown_elevators` | inspection_tools.py | elevators + inspections (non-passing outcomes via CTE + DISTINCT ON) | `limit` (max 200) |
| 2 | `get_inspection_history` | inspection_tools.py | inspections | `elevator_id`, `limit` (max 100) |
| 3 | `get_incident_count_last_year` | incident_tools.py | incidents | none |
| 4 | `get_elevator_incidents` | incident_tools.py | incidents | `elevator_id`, `limit` (max 100) |
| 5 | `get_elevators_needing_followup` | inspection_tools.py | elevators + inspections (`LOWER(outcome) = 'follow up'`) | `limit` (max 200) |
| 6 | `get_elevator_risk` | inspection_tools.py | predictions | `elevator_id` |
| 7 | `search_maintenance_docs` | rag_tools.py | ChromaDB (`maintenance_documents` collection) | `query` (max 500 chars), `n_results` (max 20) |
| 8 | `search_incident_narratives` | rag_tools.py | incidents.narrative (PostgreSQL FTS via `plainto_tsquery`) | `query` (max 200 chars), `limit` (max 20) |
| 9 | `schedule_inspection` | write_tools.py | inspections (INSERT) | `elevator_id`, `inspection_date` (YYYY-MM-DD), `reason`, `confirmed` (bool) |
| 10 | `get_fleet_stats` | inspection_tools.py | elevators + predictions + inspections (3 aggregate queries) | none |

---

## Key Design Decisions

### Location: `platform/mcp/`
Sibling of `platform/api/` (Go API). Both are data-access services for the platform layer. The MCP server is not an analysis tool, so it does not belong in `intelligence/`.

### Separate `requirements.txt`
The root `requirements.txt` serves Flask and the ETL pipeline. The MCP server does not need Flask, gunicorn, pdfplumber, or tiktoken. A separate file enables a clean install and an independent Docker layer.

### `db.py` is new (not imported from `etl_to_database.py`)
`etl_to_database.py` has side-effectful module-level code (prints, `load_dotenv`, function calls). Importing it would trigger those side effects at server startup. `db.py` reproduces the same `DATABASE_URL → DB_*` fallback pattern cleanly.

### `ThreadedConnectionPool` (not `SimpleConnectionPool`)
Streamable HTTP transport can receive concurrent tool calls. `SimpleConnectionPool` is not thread-safe. `ThreadedConnectionPool` with maxconn=10 handles concurrent requests safely.

### `rag.py` is new (not imported from `rag_preprocessing.py`)
`rag_preprocessing.py` is a write pipeline with async PDF extraction. The MCP server only needs to *read* from ChromaDB. `rag.py` is a minimal read-only client.

### RAG source format: `.txt` files (not PDF)
The RAG pipeline will ingest `.txt` files, not PDFs. `rag_preprocessing.py` has not yet been updated for `.txt` ingestion. `MAINTENANCE_SOURCE_TYPE` in `rag_tools.py` is `None` until the preprocessing script is updated — when `None`, the `where` filter is omitted and all indexed documents are searched.

**Action required when `.txt` ingestion is implemented:** Set `MAINTENANCE_SOURCE_TYPE` in `rag_tools.py` to match the `source_type` value written into ChromaDB metadata (e.g. `"maintenance_txt"`).

### Tool 8 uses PostgreSQL FTS (not ChromaDB)
Incident narratives are not in the ChromaDB collection — `rag_preprocessing.py` only processes the maintenance document folder. PostgreSQL `plainto_tsquery` / `tsvector` provides full-text search without additional preprocessing. `plainto_tsquery` is immune to tsquery injection — it treats input as plain text.

### `schedule_inspection` two-phase confirmation
`confirmed=False` (default) validates inputs and returns a summary — no write. `confirmed=True` commits the INSERT. `confirmed` must be a strict Python `bool` — strings and integers are rejected. This matches the AND-107 requirement: *"Confirm details with user before database write. Wait for explicit approval."*

### `mcp_inspection_id_seq` starts at 9,000,000
`inspections.inspection_id` is `INTEGER PRIMARY KEY` (not SERIAL). The ETL pipeline loads source records with IDs up to ~143,181. The sequence starts at 9,000,000 to avoid any collision. Created with `CREATE SEQUENCE IF NOT EXISTS` at server startup — idempotent.

### Streamable HTTP (not stdio)
stdio transport is single-threaded and requires Claude Code to spawn the process. Streamable HTTP runs as a persistent server, supports concurrent requests, and matches the production architecture where the Go API calls the MCP server over HTTP. The same URL (`http://localhost:8765/mcp`) is used by both the Go API and Claude Code.

---

## Security Summary

Two independent protection layers on every tool input:

1. **`_validators.py`** — type checking, range bounds, string length caps, date validation
2. **Parameterized queries** — `%(name)s` psycopg2 syntax; zero f-strings or string concatenation in SQL

Full audit results: `platform/mcp/security_audit.md`

---

## Implementation Sequence

Files were created in this order to respect import dependencies:

1. `platform/mcp/__init__.py`
2. `platform/mcp/tools/__init__.py`
3. `platform/mcp/requirements.txt`
4. `platform/mcp/db.py`
5. `platform/mcp/rag.py`
6. `platform/mcp/tools/_validators.py`
7. `platform/mcp/tools/inspection_tools.py`
8. `platform/mcp/tools/incident_tools.py`
9. `platform/mcp/tools/rag_tools.py`
10. `platform/mcp/tools/write_tools.py`
11. `platform/mcp/server.py`
12. `.mcp.json`
13. `.env.example` (updated)

---

## How to Run

```bash
# 1. Install dependencies
pip install -r platform/mcp/requirements.txt

# 2. Ensure PostgreSQL is running
docker-compose up

# 3. Ensure ChromaDB is populated
py -3 intelligence/rag_preprocessing.py --pdf-path /path/to/docs

# 4. Start the MCP server
py -3 -m platform.mcp.server
# Server listening on http://localhost:8765/mcp

# 5. Verify Claude Code can connect
# Open Claude Code in this project — rocket-elevators tools will appear automatically
```

---

## Verification Checklist

- [ ] `py -3 -c "import platform.mcp.server"` — imports without error
- [ ] `py -3 -m platform.mcp.server` — server starts and prints listening port
- [ ] `.mcp.json` exists at project root
- [ ] `FastMCP_PStructure.md` exists at project root
- [ ] `pip install -r platform/mcp/requirements.txt` — installs cleanly
- [ ] Claude Code shows `rocket-elevators` tools in the MCP panel after server starts
