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

import os

from dotenv import load_dotenv
from fastmcp import FastMCP

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
from platform.mcp.tools.write_tools import _ensure_sequence, schedule_inspection

load_dotenv()

mcp = FastMCP(
    name="rocket-elevators",
    description=(
        "Live database and document search tools for the Rocket Elevators "
        "operations dashboard. Queries PostgreSQL (elevator, inspection, incident, "
        "risk data) and ChromaDB (maintenance documents)."
    ),
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
    port = int(os.environ.get("MCP_PORT", 8765))

    # Create the inspection ID sequence if it does not exist yet.
    # Must run before any tool call that writes to the inspections table.
    _ensure_sequence()

    mcp.run(transport="streamable-http", host="0.0.0.0", port=port)
