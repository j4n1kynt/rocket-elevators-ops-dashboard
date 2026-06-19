"""
Unit tests for response-level `source` attribution on the PostgreSQL data tools.

Every data-backed answer must cite an accurate source (Company.md lines 78-79).
These tests assert that each tool's SUCCESS response carries a descriptive
`source` string so the LLM can cite the specific table it read from.

get_connection is mocked with a fake connection so the tests need no live
database — the same patch target used in test_validation.py.

Run: PYTHONPATH=. ALLOW_EMPTY_CHROMADB=1 py -3.12 -m pytest \
        platform/mcp/tools/test_source_attribution.py -v
"""

import contextlib
from unittest.mock import patch

import pytest

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


# ── Fake connection ─────────────────────────────────────────────────────────

class _FakeConn:
    """Minimal asyncpg-connection stand-in driven by canned return values."""

    def __init__(self, *, fetch=None, fetchrow=None, fetchval=None):
        self._fetch = fetch if fetch is not None else []
        self._fetchrow = fetchrow
        self._fetchval = fetchval

    async def fetch(self, *args, **kwargs):
        return self._fetch

    async def fetchrow(self, *args, **kwargs):
        # Pop the next queued row when a list of rows was provided
        # (get_fleet_stats issues several fetchrow/fetchval calls).
        if isinstance(self._fetchrow, list):
            return self._fetchrow.pop(0)
        return self._fetchrow

    async def fetchval(self, *args, **kwargs):
        if isinstance(self._fetchval, list):
            return self._fetchval.pop(0)
        return self._fetchval


def _fake_connection(**kwargs):
    conn = _FakeConn(**kwargs)

    @contextlib.asynccontextmanager
    async def _cm():
        yield conn

    return _cm


# ── get_tssa_shutdown_elevators ───────────────────────────────────────────────

@pytest.mark.asyncio
async def test_tssa_shutdown_success_has_source():
    rows = [{"elevator_id": 1, "location": "Site A", "status": "ACTIVE",
             "latest_inspection_date": "2015-03-20", "outcome": "Follow up",
             "inspection_type": "Periodic"}]
    with patch("platform.mcp.tools.inspection_tools.get_connection",
               _fake_connection(fetch=rows)):
        result = await get_tssa_shutdown_elevators(limit=50)

    assert result["source"] == "inspections table (most-recent inspection per elevator)"
    assert "note" in result  # existing field preserved


# ── get_inspection_history ────────────────────────────────────────────────────

@pytest.mark.asyncio
async def test_inspection_history_success_has_source():
    rows = [{"inspection_id": 7, "inspection_type": "Periodic",
             "earliest_inspection_date": "2014-01-01",
             "latest_inspection_date": "2015-03-20", "outcome": "Passed"}]
    with patch("platform.mcp.tools.inspection_tools.get_connection",
               _fake_connection(fetch=rows, fetchval=True)):
        result = await get_inspection_history(elevator_id=1, limit=20)

    assert result["found"] is True
    assert result["source"] == "inspections table"


# ── get_elevators_needing_followup ────────────────────────────────────────────

@pytest.mark.asyncio
async def test_followup_success_has_source():
    rows = [{"elevator_id": 2, "location": "Site B", "status": "ACTIVE",
             "latest_inspection_date": "2015-06-01", "outcome": "Follow up",
             "inspection_type": "Periodic"}]
    with patch("platform.mcp.tools.inspection_tools.get_connection",
               _fake_connection(fetch=rows)):
        result = await get_elevators_needing_followup(limit=50)

    assert result["source"] == "inspections table (most-recent inspection per elevator)"


# ── get_elevator_risk ─────────────────────────────────────────────────────────

@pytest.mark.asyncio
async def test_elevator_risk_success_has_source():
    row = {"elevator_id": 1, "risk_score": 0.91, "risk_level": "HIGH",
           "risk_explanation": "Repeated failures.", "model_version": "v1",
           "prediction_date": "2026-06-18"}
    with patch("platform.mcp.tools.inspection_tools.get_connection",
               _fake_connection(fetchrow=row, fetchval=True)):
        result = await get_elevator_risk(elevator_id=1)

    assert result["prediction_found"] is True
    assert result["source"] == "predictions table"
    # provenance fields already on the row stay intact
    assert result["model_version"] == "v1"
    assert result["prediction_date"] == "2026-06-18"


# ── get_fleet_stats ───────────────────────────────────────────────────────────

@pytest.mark.asyncio
async def test_fleet_stats_success_has_source():
    risk_row = {"total": 10, "low": 5, "medium": 3, "high": 2, "unknown": 0}
    type_rows = [{"type": "Passenger", "cnt": 7}, {"type": "Freight", "cnt": 3}]
    # get_fleet_stats: fetchrow (risk), fetchval (passing count), fetch (types)
    with patch("platform.mcp.tools.inspection_tools.get_connection",
               _fake_connection(fetch=type_rows, fetchrow=risk_row, fetchval=8)):
        result = await get_fleet_stats()

    assert result["source"] == "fleet database (aggregate)"
    assert result["total_elevators"] == 10


# ── get_incident_count_last_year ──────────────────────────────────────────────

@pytest.mark.asyncio
async def test_incident_count_success_has_source():
    row = {"total_incidents": 12, "fatal_incidents": 0,
           "injury_incidents": 3, "year_queried": 2015}
    with patch("platform.mcp.tools.incident_tools.get_connection",
               _fake_connection(fetchrow=row)):
        result = await get_incident_count_last_year()

    assert result["source"] == "incidents table (aggregate)"
    assert result["total_incidents"] == 12


# ── get_elevator_incidents ────────────────────────────────────────────────────

@pytest.mark.asyncio
async def test_elevator_incidents_success_has_source():
    rows = [{"incident_id": 1234, "creation_date": "2015-04-01",
             "date_of_occurrence": "2015-03-20", "category": "Entrapment",
             "incident_summary": "Stuck", "root_cause": "Door fault",
             "injury_severity": "none", "fatal_injury": False}]
    with patch("platform.mcp.tools.incident_tools.get_connection",
               _fake_connection(fetch=rows, fetchval=True)):
        result = await get_elevator_incidents(elevator_id=1, limit=20)

    assert result["found"] is True
    assert result["source"] == "incidents table"
