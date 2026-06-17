"""
Confirms that no DB or RAG query is attempted when Pydantic input validation fails.

Each test:
  1. Patches the relevant data-access call (get_connection or rag_query).
  2. Calls the tool with an invalid argument.
  3. Asserts a ValidationError is raised.
  4. Asserts the data-access mock was never called.

Run: pytest platform/mcp/tools/test_validation.py -v
"""

import pytest
from unittest.mock import patch
from pydantic import ValidationError

from platform.mcp.tools.incident_tools import get_elevator_incidents
from platform.mcp.tools.inspection_tools import (
    get_elevator_risk,
    get_elevators_needing_followup,
    get_inspection_history,
    get_tssa_shutdown_elevators,
)
from platform.mcp.tools.rag_tools import search_incident_narratives, search_maintenance_docs
from platform.mcp.tools.write_tools import schedule_inspection


# ── get_elevator_incidents ────────────────────────────────────────────────────

@pytest.mark.asyncio
@pytest.mark.parametrize("elevator_id,limit", [
    (0,          20),   # elevator_id not positive
    (-1,         20),   # elevator_id negative
    (10_000_000, 20),   # elevator_id exceeds max
    (1,           0),   # limit below minimum
    (1,         101),   # limit exceeds max
])
async def test_get_elevator_incidents_bad_input_skips_db(elevator_id, limit):
    with patch("platform.mcp.tools.incident_tools.get_connection") as mock_conn:
        with pytest.raises(ValidationError):
            await get_elevator_incidents(elevator_id=elevator_id, limit=limit)
        mock_conn.assert_not_called()


# ── get_tssa_shutdown_elevators ───────────────────────────────────────────────

@pytest.mark.asyncio
@pytest.mark.parametrize("limit", [0, 201])
async def test_get_tssa_shutdown_elevators_bad_input_skips_db(limit):
    with patch("platform.mcp.tools.inspection_tools.get_connection") as mock_conn:
        with pytest.raises(ValidationError):
            await get_tssa_shutdown_elevators(limit=limit)
        mock_conn.assert_not_called()


# ── get_inspection_history ────────────────────────────────────────────────────

@pytest.mark.asyncio
@pytest.mark.parametrize("elevator_id,limit", [
    (0,           20),
    (-5,          20),
    (10_000_000,  20),
    (1,            0),
    (1,          101),
])
async def test_get_inspection_history_bad_input_skips_db(elevator_id, limit):
    with patch("platform.mcp.tools.inspection_tools.get_connection") as mock_conn:
        with pytest.raises(ValidationError):
            await get_inspection_history(elevator_id=elevator_id, limit=limit)
        mock_conn.assert_not_called()


# ── get_elevators_needing_followup ────────────────────────────────────────────

@pytest.mark.asyncio
@pytest.mark.parametrize("limit", [0, 201])
async def test_get_elevators_needing_followup_bad_input_skips_db(limit):
    with patch("platform.mcp.tools.inspection_tools.get_connection") as mock_conn:
        with pytest.raises(ValidationError):
            await get_elevators_needing_followup(limit=limit)
        mock_conn.assert_not_called()


# ── get_elevator_risk ─────────────────────────────────────────────────────────

@pytest.mark.asyncio
@pytest.mark.parametrize("elevator_id", [0, -1, 10_000_000])
async def test_get_elevator_risk_bad_input_skips_db(elevator_id):
    with patch("platform.mcp.tools.inspection_tools.get_connection") as mock_conn:
        with pytest.raises(ValidationError):
            await get_elevator_risk(elevator_id=elevator_id)
        mock_conn.assert_not_called()


# ── schedule_inspection ───────────────────────────────────────────────────────

@pytest.mark.asyncio
@pytest.mark.parametrize("kwargs", [
    # elevator_id out of range
    {"elevator_id": 0,          "inspection_date": "2030-01-01", "reason": "test"},
    {"elevator_id": -1,         "inspection_date": "2030-01-01", "reason": "test"},
    {"elevator_id": 10_000_000, "inspection_date": "2030-01-01", "reason": "test"},
    # date in the past
    {"elevator_id": 1, "inspection_date": "2000-01-01", "reason": "test"},
    # date wrong format
    {"elevator_id": 1, "inspection_date": "01-01-2030", "reason": "test"},
    {"elevator_id": 1, "inspection_date": "not-a-date",  "reason": "test"},
    # empty / blank reason
    {"elevator_id": 1, "inspection_date": "2030-01-01", "reason": ""},
    {"elevator_id": 1, "inspection_date": "2030-01-01", "reason": "   "},
    # reason too long
    {"elevator_id": 1, "inspection_date": "2030-01-01", "reason": "x" * 501},
    # confirmed not a strict bool
    {"elevator_id": 1, "inspection_date": "2030-01-01", "reason": "test", "confirmed": "yes"},
    {"elevator_id": 1, "inspection_date": "2030-01-01", "reason": "test", "confirmed": 1},
])
async def test_schedule_inspection_bad_input_skips_db(kwargs):
    with patch("platform.mcp.tools.write_tools.get_connection") as mock_conn:
        with pytest.raises(ValidationError):
            await schedule_inspection(**kwargs)
        mock_conn.assert_not_called()


# ── search_maintenance_docs ───────────────────────────────────────────────────

@pytest.mark.parametrize("query,n_results", [
    ("",        5),     # empty query
    ("   ",     5),     # whitespace-only query
    ("x" * 501, 5),    # query too long
    ("test",    0),     # n_results below minimum
    ("test",   21),     # n_results exceeds max
])
def test_search_maintenance_docs_bad_input_skips_rag(query, n_results):
    with patch("platform.mcp.tools.rag_tools.rag_query") as mock_rag:
        with pytest.raises(ValidationError):
            search_maintenance_docs(query=query, n_results=n_results)
        mock_rag.assert_not_called()


# ── search_incident_narratives ────────────────────────────────────────────────

@pytest.mark.asyncio
@pytest.mark.parametrize("query,limit", [
    ("",        5),     # empty query
    ("   ",     5),     # whitespace-only query
    ("x" * 201, 5),    # query too long (max 200)
    ("test",    0),     # limit below minimum
    ("test",   21),     # limit exceeds max
])
async def test_search_incident_narratives_bad_input_skips_db(query, limit):
    with patch("platform.mcp.tools.rag_tools.get_connection") as mock_conn:
        with pytest.raises(ValidationError):
            await search_incident_narratives(query=query, limit=limit)
        mock_conn.assert_not_called()
