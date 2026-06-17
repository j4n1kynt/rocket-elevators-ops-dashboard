"""
PostgreSQL connection pool for the FastMCP server.

Uses the same DATABASE_URL -> DB_* fallback logic as etl_to_database.py and platform/api/db.go.
Uses ThreadedConnectionPool (not SimpleConnectionPool) because Streamable HTTP transport
can receive concurrent tool calls.

Do not import from etl_to_database.py — that script has side-effectful module-level code.
"""

import contextlib
import os
import threading

import psycopg2
import psycopg2.pool
from dotenv import load_dotenv

load_dotenv()

_pool: psycopg2.pool.ThreadedConnectionPool | None = None
_pool_lock = threading.Lock()


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

    return (
        f"host={os.environ['DB_HOST']} "
        f"port={os.environ.get('DB_PORT', '5432')} "
        f"user={os.environ['DB_USER']} "
        f"password={os.environ['DB_PASSWORD']} "
        f"dbname={os.environ['DB_NAME']}"
    )


def get_pool() -> psycopg2.pool.ThreadedConnectionPool:
    global _pool
    if _pool is None:
        with _pool_lock:
            if _pool is None:  # double-checked locking — safe under GIL + lock
                try:
                    _pool = psycopg2.pool.ThreadedConnectionPool(
                        minconn=1,
                        maxconn=10,
                        dsn=_get_dsn(),
                    )
                except psycopg2.OperationalError as exc:
                    raise RuntimeError(
                        f"Failed to connect to PostgreSQL: {exc}. "
                        "Ensure the database is running and credentials in .env are correct."
                    ) from exc
    return _pool


@contextlib.contextmanager
def get_connection():
    """Context manager — always returns the connection to the pool on exit."""
    try:
        pool = get_pool()
        conn = pool.getconn()
    except psycopg2.pool.PoolError as exc:
        raise RuntimeError(
            f"Connection pool exhausted or unavailable: {exc}. "
            "The pool allows up to 10 concurrent connections."
        ) from exc

    try:
        yield conn
    except Exception:
        try:
            conn.rollback()
        except Exception:
            pass  # connection may already be broken; rollback failure is non-fatal
        raise
    finally:
        pool.putconn(conn)
