"""
FastMCP server for the Rocket Elevators operations chatbot — AND-107.

Exposes 10 tools that query PostgreSQL and ChromaDB directly.

Runtime architecture:
  Production : User → Go API → MCP server → PostgreSQL / ChromaDB
  Development: Claude Code → MCP server (via .mcp.json, Streamable HTTP)

Transport: Streamable HTTP on MCP_PORT (default 8765).
The Go API and Claude Code both connect via http://localhost:MCP_PORT/mcp.

Start the server:
  py -3 -m platform.mcp.server

Requires:
  - PostgreSQL running and reachable (see .env DATABASE_URL or DB_* vars)
  - ChromaDB populated (py -3 intelligence/rag_preprocessing.py)
  - Dependencies installed (pip install -r platform/mcp/requirements.txt)
"""

import asyncio
import logging
import os
from contextlib import asynccontextmanager

from dotenv import load_dotenv
from fastmcp import FastMCP

from platform.mcp.db import close_pool, init_pool
from platform.mcp.rag import warm_rag
from platform.mcp.tools.incident_tools import (
    get_elevator_incidents,
    get_incident_count_last_year,
)
from platform.mcp.tools.inspection_tools import (
    get_elevator_risk,
    get_elevators_needing_followup,
    get_fleet_stats,
    get_inspection_history,
    get_tssa_shutdown_elevators,
)
from platform.mcp.tools.rag_tools import (
    search_incident_narratives,
    search_maintenance_docs,
)
from platform.mcp.tools.write_tools import schedule_inspection

load_dotenv()

logger = logging.getLogger(__name__)


def _guard_skip_confirmation() -> None:
    """
    MCP_SKIP_CONFIRMATION bypasses the human-approval gate and writes on the first
    call. It is a test-only affordance. Refuse to start if it is set in a
    production environment; warn loudly everywhere else.
    """
    if not os.environ.get("MCP_SKIP_CONFIRMATION"):
        return
    env = (
        os.environ.get("APP_ENV")
        or os.environ.get("ENV")
        or os.environ.get("ENVIRONMENT")
        or ""
    ).strip().lower()
    if env in ("production", "prod"):
        raise RuntimeError(
            "MCP_SKIP_CONFIRMATION is set in a production environment "
            f"({env!r}). This disables the confirmation gate for inspection "
            "scheduling — refusing to start."
        )
    logger.warning(
        "MCP_SKIP_CONFIRMATION is set — inspection scheduling will WRITE to the "
        "database without a confirmation step. Use only in tests."
    )


async def _warm_rag_at_startup() -> None:
    """
    Warm the RAG embedding model off the request path. Non-fatal: if ChromaDB is
    not populated (or sentence-transformers is unavailable), the server still
    starts so the PostgreSQL-backed tools keep working. Set MCP_SKIP_RAG_WARMUP to
    skip entirely (e.g. in tests or DB-only deployments).

    Run in a worker thread so the heavy synchronous model load does not block the
    event loop during startup.
    """
    if os.environ.get("MCP_SKIP_RAG_WARMUP"):
        logger.info("MCP_SKIP_RAG_WARMUP set — skipping RAG warm-up")
        return
    try:
        await asyncio.to_thread(warm_rag)
        logger.info("RAG embedding model warmed at startup")
    except Exception as exc:  # noqa: BLE001 — warm-up must never block startup
        logger.warning(
            "RAG warm-up skipped (%s) — the first RAG query will pay the "
            "cold-start cost and may hit the MCP timeout.",
            exc,
        )


@asynccontextmanager
async def lifespan(app: FastMCP):
    """
    Server lifespan: create the asyncpg pool (includes health check + sequence
    creation) and warm the RAG embedding model on startup, drain the pool on
    shutdown.
    """
    _guard_skip_confirmation()
    await init_pool()
    await _warm_rag_at_startup()
    yield
    await close_pool()


mcp = FastMCP(
    name="rocket-elevators",
    instructions=(
        "Live database and document search tools for the Rocket Elevators "
        "operations dashboard. Queries PostgreSQL (elevator, inspection, incident, "
        "risk data) and ChromaDB (maintenance documents)."
    ),
    lifespan=lifespan,
)

# ── Read tools ────────────────────────────────────────────────────────────────
mcp.tool(get_tssa_shutdown_elevators)
mcp.tool(get_inspection_history)
mcp.tool(get_elevators_needing_followup)
mcp.tool(get_elevator_risk)
mcp.tool(get_fleet_stats)
mcp.tool(get_incident_count_last_year)
mcp.tool(get_elevator_incidents)
mcp.tool(search_maintenance_docs)
mcp.tool(search_incident_narratives)

# ── Write tools ───────────────────────────────────────────────────────────────
mcp.tool(schedule_inspection)


if __name__ == "__main__":
    # Prefer MCP_PORT; fall back to PORT (injected by Render and similar hosts).
    port = int(os.environ.get("MCP_PORT") or os.environ.get("PORT", "8765"))
    mcp.run(transport="streamable-http", host="0.0.0.0", port=port)
