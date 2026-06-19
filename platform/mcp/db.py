"""
PostgreSQL connection pool for the FastMCP server.

Uses asyncpg for native async I/O, compatible with uvicorn's event loop.
The pool is created once in init_pool() (called from the FastMCP lifespan)
and shared for the lifetime of the server process.

Uses the same DATABASE_URL -> DB_* fallback logic as etl_to_database.py
and platform/api/db.go.

Do not import from etl_to_database.py — that script has side-effectful
module-level code.
"""

import contextlib
import logging
import os

import asyncpg
from dotenv import load_dotenv

logger = logging.getLogger(__name__)

load_dotenv()

_pool: asyncpg.Pool | None = None

# Inspection ID sequence used by schedule_inspection in write_tools.py.
# Created here so init_pool() can guarantee it exists before any tool call.
_SEQUENCE_DDL = """
    CREATE SEQUENCE IF NOT EXISTS mcp_inspection_id_seq
    START 9000000
    INCREMENT 1
    NO CYCLE
"""

# Audit log table — mirrors 002_audit_log.sql migration.
# No FK constraints: audit rows are immutable and must not cascade-delete.
_AUDIT_LOG_DDL = """
    CREATE TABLE IF NOT EXISTS scheduling_audit_log (
        log_id          BIGSERIAL   PRIMARY KEY,
        elevator_id     INTEGER     NOT NULL,
        inspection_id   INTEGER,
        inspection_date DATE        NOT NULL,
        inspection_type TEXT        NOT NULL,
        reason          TEXT,
        outcome         TEXT        NOT NULL CHECK (outcome IN ('success', 'error')),
        error_message   TEXT,
        performed_at    TIMESTAMPTZ NOT NULL DEFAULT now()
    );
    CREATE INDEX IF NOT EXISTS idx_audit_log_elevator_id  ON scheduling_audit_log (elevator_id);
    CREATE INDEX IF NOT EXISTS idx_audit_log_performed_at ON scheduling_audit_log (performed_at);
"""

# Duplicate pending-inspection guard — mirrors 003_pending_inspection_unique.sql.
# Scoped to chatbot-created rows (inspection_id >= 9,000,000) so it never
# conflicts with imported source data.
_PENDING_UNIQUE_DDL = """
    CREATE UNIQUE INDEX IF NOT EXISTS uq_pending_inspection
        ON inspections (elevator_id, earliest_inspection_date, inspection_type)
        WHERE outcome = 'Pending' AND inspection_id >= 9000000
"""


def _get_dsn() -> str:
    url = os.environ.get("DATABASE_URL")
    if url:
        return url

    missing = [v for v in ("DB_HOST", "DB_USER", "DB_PASSWORD", "DB_NAME") if not os.environ.get(v)]
    if missing:
        raise RuntimeError(
            f"Missing required environment variables: {', '.join(missing)}. "
            "Set DATABASE_URL for a remote database, or set DB_HOST / DB_USER / "
            "DB_PASSWORD / DB_NAME for local Docker. See .env.example."
        )

    host = os.environ["DB_HOST"]
    port = os.environ.get("DB_PORT", "5432")
    user = os.environ["DB_USER"]
    password = os.environ["DB_PASSWORD"]
    dbname = os.environ["DB_NAME"]
    return f"postgresql://{user}:{password}@{host}:{port}/{dbname}"


async def init_pool() -> None:
    """
    Create the asyncpg pool, verify connectivity, and ensure the inspection
    ID sequence exists. Called once from the FastMCP lifespan at server startup.
    Raises RuntimeError if the database is unreachable or misconfigured.
    """
    global _pool
    try:
        _pool = await asyncpg.create_pool(_get_dsn(), min_size=2, max_size=10)
    except (asyncpg.InvalidCatalogNameError, OSError, Exception) as exc:
        raise RuntimeError(
            f"Failed to connect to PostgreSQL: {exc}. "
            "Ensure the database is running and credentials in .env are correct."
        ) from exc

    async with _pool.acquire() as conn:
        await conn.execute("SELECT 1")       # startup health check
        try:
            await conn.execute(_SEQUENCE_DDL)
            await conn.execute(_AUDIT_LOG_DDL)
        except asyncpg.InsufficientPrivilegeError:
            # Non-fatal in CI / read-only roles — schedule_inspection will fail if
            # called, but all read tools remain functional.
            pass

        # The unique index can fail if pre-existing chatbot rows already contain a
        # duplicate; degrade gracefully (log + continue) rather than crash startup.
        try:
            await conn.execute(_PENDING_UNIQUE_DDL)
        except asyncpg.InsufficientPrivilegeError:
            pass
        except Exception as exc:
            logger.warning(
                "Could not create uq_pending_inspection index (duplicate-pending "
                "guard inactive): %s",
                exc,
            )


async def close_pool() -> None:
    """Drain and close the pool. Called from the FastMCP lifespan at server shutdown."""
    global _pool
    if _pool is not None:
        await _pool.close()
        _pool = None


@contextlib.asynccontextmanager
async def get_connection():
    """
    Async context manager — acquires a connection from the pool for one tool call
    and returns it automatically on exit.
    """
    if _pool is None:
        raise RuntimeError(
            "Connection pool is not initialized. "
            "Ensure init_pool() ran at server startup via the lifespan hook."
        )
    async with _pool.acquire() as conn:
        yield conn
