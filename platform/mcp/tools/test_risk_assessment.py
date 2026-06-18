"""
Integration tests for get_elevator_risk — all three response shapes.

Seeds three minimal test records (two with predictions, one without),
runs four scenarios, then cleans up. Works against both the local
Docker DB and the CI GitHub Actions service container.

Requires: PostgreSQL running with credentials from .env or DATABASE_URL.
Run: pytest platform/mcp/tools/test_risk_assessment.py -v
"""

import asyncio
import contextlib
import os
from datetime import date
from unittest.mock import patch

import asyncpg
import pytest
from dotenv import load_dotenv

from platform.mcp.tools.inspection_tools import get_elevator_risk

load_dotenv()

# ── Connection ────────────────────────────────────────────────────────────────


def _get_dsn() -> str:
    url = os.environ.get("DATABASE_URL")
    if url:
        return url
    host = os.environ.get("DB_HOST", "localhost")
    port = os.environ.get("DB_PORT", "5433")
    user = os.environ["DB_USER"]
    password = os.environ["DB_PASSWORD"]
    dbname = os.environ["DB_NAME"]
    return f"postgresql://{user}:{password}@{host}:{port}/{dbname}"


@contextlib.asynccontextmanager
async def _direct_connection():
    """Single asyncpg connection — bypasses the MCP pool so tests need no init_pool()."""
    conn = await asyncpg.connect(_get_dsn())
    try:
        yield conn
    finally:
        await conn.close()


# ── Synthetic test data ───────────────────────────────────────────────────────

# IDs in a range well above real fleet data to avoid conflicts
_ID_WITH_EXPLANATION = 9_000_001
_ID_NULL_EXPLANATION = 9_000_002
_ID_NO_PREDICTION    = 9_000_003
_ID_NONEXISTENT      = 9_999_999


@pytest.fixture(scope="module", autouse=True)
def seed_and_teardown():
    """
    Insert synthetic elevators + predictions before any test in this module;
    cascade-delete them via elevators FK after the last test completes.
    ON CONFLICT DO NOTHING is safe if IDs somehow already exist.
    """

    async def _seed() -> None:
        conn = await asyncpg.connect(_get_dsn())
        try:
            await conn.executemany(
                """
                INSERT INTO elevators (elevator_id, location, license_number, status)
                VALUES ($1, $2, $3, $4)
                ON CONFLICT (elevator_id) DO NOTHING
                """,
                [
                    (_ID_WITH_EXPLANATION, "Test Site A – Unit 1", "TEST-RISK-001", "ACTIVE"),
                    (_ID_NULL_EXPLANATION, "Test Site B – Unit 2", "TEST-RISK-002", "ACTIVE"),
                    (_ID_NO_PREDICTION,    "Test Site C – Unit 3", "TEST-RISK-003", "ACTIVE"),
                ],
            )
            await conn.executemany(
                """
                INSERT INTO predictions
                    (elevator_id, risk_score, risk_level, risk_explanation, model_version, prediction_date)
                VALUES ($1, $2, $3, $4, $5, $6)
                ON CONFLICT (elevator_id) DO NOTHING
                """,
                [
                    (
                        _ID_WITH_EXPLANATION,
                        0.9123,
                        "HIGH",
                        "Repeated failed inspections and outstanding compliance orders.",
                        "v1-test",
                        date(2026, 6, 18),
                    ),
                    (
                        _ID_NULL_EXPLANATION,
                        0.7456,
                        "MEDIUM",
                        None,
                        "v1-test",
                        date(2026, 6, 18),
                    ),
                ],
            )
        finally:
            await conn.close()

    async def _cleanup() -> None:
        conn = await asyncpg.connect(_get_dsn())
        try:
            # ON DELETE CASCADE propagates to predictions
            await conn.execute(
                "DELETE FROM elevators WHERE elevator_id = ANY($1::int[])",
                [_ID_WITH_EXPLANATION, _ID_NULL_EXPLANATION, _ID_NO_PREDICTION],
            )
        finally:
            await conn.close()

    asyncio.run(_seed())
    yield
    asyncio.run(_cleanup())


# ── Tests ─────────────────────────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_high_risk_with_explanation_returns_all_fields():
    """HIGH-risk elevator with explanation → all required fields present."""
    with patch("platform.mcp.tools.inspection_tools.get_connection", _direct_connection):
        result = await get_elevator_risk(elevator_id=_ID_WITH_EXPLANATION)

    assert result["elevator_found"] is True
    assert result["prediction_found"] is True
    assert result["risk_level"] == "HIGH"
    assert isinstance(result["risk_score"], float)
    assert result["risk_explanation"] is not None and result["risk_explanation"] != ""
    assert result["model_version"] is not None
    assert result["prediction_date"] is not None


@pytest.mark.asyncio
async def test_null_explanation_returns_prediction_without_explanation():
    """Elevator with prediction but NULL explanation → prediction_found=True, explanation absent."""
    with patch("platform.mcp.tools.inspection_tools.get_connection", _direct_connection):
        result = await get_elevator_risk(elevator_id=_ID_NULL_EXPLANATION)

    assert result["elevator_found"] is True
    assert result["prediction_found"] is True
    assert result.get("risk_explanation") is None
    assert isinstance(result["risk_score"], float)
    assert result["risk_level"] is not None


@pytest.mark.asyncio
async def test_elevator_without_prediction_returns_no_prediction():
    """Elevator in fleet but absent from predictions → elevator_found=True, prediction_found=False."""
    with patch("platform.mcp.tools.inspection_tools.get_connection", _direct_connection):
        result = await get_elevator_risk(elevator_id=_ID_NO_PREDICTION)

    assert result["elevator_found"] is True
    assert result["prediction_found"] is False
    assert "risk_score" not in result
    assert "risk_level" not in result


@pytest.mark.asyncio
async def test_nonexistent_elevator_returns_not_found():
    """An elevator ID that does not exist in the fleet → both flags False."""
    with patch("platform.mcp.tools.inspection_tools.get_connection", _direct_connection):
        result = await get_elevator_risk(elevator_id=_ID_NONEXISTENT)

    assert result["elevator_found"] is False
    assert result["prediction_found"] is False
