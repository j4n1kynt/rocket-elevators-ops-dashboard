# asyncpg Connection Pool & Startup Health Check
**AND-107 | Date:** 2026-06-16

---

## Why we made this change

The original MCP server used psycopg2's `ThreadedConnectionPool`. That worked fine on paper, but there was a fundamental mismatch: the server runs on uvicorn, which is a single-threaded async event loop. Every time a tool called `get_connection()`, it blocked that loop while waiting for PostgreSQL to respond. Under any real load — multiple tool calls arriving at the same time — those calls would queue up behind each other even though none of them were actually doing CPU work. They were just waiting on the network.

asyncpg is a PostgreSQL driver built from the ground up for async I/O. It talks directly to PostgreSQL over the wire protocol without going through libpq, and it integrates natively with Python's event loop. Swapping psycopg2 for asyncpg means tool calls can truly run concurrently — one tool is waiting on its database query while another is already processing its result, all on the same thread.

---

## What changed in db.py

This file was rewritten completely. The thread-safe lazy initialization pattern (`threading.Lock()` with double-checked locking) is gone because asyncpg doesn't need it — the pool is created once during server startup via the lifespan hook, and by the time any tool call arrives, the pool already exists.

The new public surface is three functions:

**`init_pool()`** is called once when the server starts. It creates an asyncpg pool with `min_size=2` and `max_size=10` (same ceiling as before, with a slightly higher floor so connections are warm from the start). Before it returns, it does two things: runs a `SELECT 1` against the database as a startup health check, and executes the `CREATE SEQUENCE IF NOT EXISTS mcp_inspection_id_seq` DDL. The health check is what makes startup fail fast — if PostgreSQL isn't reachable, the server refuses to start with a clear error instead of dying silently on the first tool call thirty seconds later.

**`close_pool()`** is called when the server shuts down. It drains the pool gracefully, letting in-flight queries finish before closing connections.

**`get_connection()`** is now an async context manager. Tools use it with `async with get_connection() as conn:` and get back a live asyncpg connection. When the block exits — whether normally or through an exception — asyncpg returns the connection to the pool automatically.

One non-obvious detail: the inspection ID sequence (`mcp_inspection_id_seq`) used to live in `write_tools.py` and was created by a function called `_ensure_sequence()` that ran before `mcp.run()`. We moved it into `init_pool()` instead. This keeps the startup sequence in one place and removes the only reason `server.py` needed to import from `write_tools` at the top level. The DDL is `CREATE SEQUENCE IF NOT EXISTS`, so it's idempotent — safe to run on every startup.

The DSN builder (`_get_dsn()`) follows the same `DATABASE_URL → DB_*` fallback logic as the rest of the project, but now builds a `postgresql://user:password@host:port/dbname` URL instead of a libpq keyword string, because that's what asyncpg expects.

---

## What changed in server.py

The main change here is the lifespan context manager. FastMCP supports ASGI-style lifespan hooks, and we wired ours up via the `lifespan=` constructor argument:

```python
@asynccontextmanager
async def lifespan(app: FastMCP):
    await init_pool()   # creates pool, runs SELECT 1, creates sequence
    yield               # server is running and accepting requests
    await close_pool()  # graceful shutdown
```

Everything between `init_pool()` and `yield` happens before the HTTP server opens its port. If `init_pool()` raises — because the database is down or the credentials are wrong — the process exits immediately with a traceback pointing directly at the problem. This replaces the old pattern where `_ensure_sequence()` was called synchronously in the `if __name__ == "__main__":` block, which was fragile because it ran outside the event loop.

The old `__main__` block is now just two lines: read the port from the environment and call `mcp.run()`.

---

## What changed in the tool files

All four tool files were converted from sync to async. The changes are mechanical but total:

- Every tool function got `async def` instead of `def`
- `with get_connection() as conn:` became `async with get_connection() as conn:`
- psycopg2 cursors (`conn.cursor()`, `cur.execute()`, `cur.fetchall()`) were replaced with asyncpg's cursor-free API (`await conn.fetch()`, `await conn.fetchrow()`, `await conn.fetchval()`, `await conn.execute()`)
- SQL parameter placeholders changed from `%(name)s` to positional `$1`, `$2`, etc. — that's asyncpg's format
- asyncpg returns `Record` objects instead of tuples; `dict(record)` converts them cleanly for the JSON responses

The only tool with any real logic change is `schedule_inspection` in `write_tools.py`. The INSERT is now wrapped in `async with conn.transaction():` to make the transactional intent explicit. In asyncpg, statements outside an explicit transaction block auto-commit, so the INSERT would have committed either way — but wrapping it makes the code easier to reason about.

---

## Fixes found during testing

Getting the server to actually start turned up four pre-existing problems that had nothing to do with the asyncpg migration.

**`platform/__init__.py` was missing.** The documented run command `py -3 -m platform.mcp.server` had never worked. Python found the stdlib `platform` module (a `.py` file, not a package) before the local `platform/` directory, and failed with `'platform' is not a package`. The fix was to create `platform/__init__.py`. But a plain empty file caused the opposite problem: now the local package shadowed the stdlib module, and third-party packages that call `platform.python_implementation()` (like `attrs`, deep in FastMCP's dependency tree) broke with `AttributeError`. The solution was a `__getattr__` hook in `platform/__init__.py` that lazily loads the real stdlib `platform` module from `sys.path` (skipping the current directory) and proxies any attribute lookup to it. From the outside, `import platform; platform.python_implementation()` works exactly as expected.

**`chromadb` and `sentence-transformers` don't have Python 3.14 wheels yet.** The only Python version installed on this machine is 3.14, which was released recently enough that several packages in `platform/mcp/requirements.txt` can't build for it. `rag.py` had top-level `import chromadb` and `from sentence_transformers import SentenceTransformer` statements, which caused an import error when the server started even though no RAG tool was being called. The fix was to move both imports inside the functions that actually use them (`_get_client()` and `_get_model()`). The server now starts cleanly without those packages; the RAG tools will raise a clear error at call time if they're invoked without `chromadb` installed, which is correct behavior.

**FastMCP 3.4.2 was installed, but `requirements.txt` pinned 2.5.2.** The two versions have a breaking API difference: `FastMCP(description=...)` was renamed to `FastMCP(instructions=...)` in 3.x. The constructor call in `server.py` was updated and the pin in `requirements.txt` was corrected to `3.4.2`.

**`py -3 -m pip install` vs `pip install` hit different environments.** The first install attempt used the shell's `pip` command, which installed packages into a different Python environment than the one `py -3` resolves to. Re-running with `py -3 -m pip install` targeted the correct interpreter.

---

## End state

After all of the above, the server starts in about 2 seconds, passes the PostgreSQL health check, creates the sequence, and serves all 10 tools. The tool test run confirmed every DB-backed tool returns well-formed responses with no unexpected nulls. The `risk_explanation` field on elevator 4821 is `null` by design — that field is only populated for HIGH-risk elevators by `generate_explanations.py`.
