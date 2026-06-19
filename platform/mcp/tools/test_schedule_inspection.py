"""
Integration tests for schedule_inspection — the full scheduling flow.

Covers the four scenarios required by the scheduling card:
  1. Success path        — a confirmed write creates an inspection row AND a
                           'success' audit row, inside one transaction.
  2. Validation failure  — a confirmed write against a nonexistent elevator returns
                           an error, writes an 'error' audit row, and creates no
                           inspection. (Input-shape validation — bad dates/types/IDs
                           — is covered in test_validation.py.)
  3. Duplicate (idempotency) — a second identical confirmed write is blocked by the
                           uq_pending_inspection guard; only one inspection exists.
  4. Phase 1 preview     — confirmed=False returns a summary and writes NOTHING.

Cancellation has no MCP call (the Go layer short-circuits before reaching this
tool), so it is verified in platform/api/chat_test.go, not here.

Requires PostgreSQL (see .env / DATABASE_URL). Mirrors the connection + seeding
pattern in test_risk_assessment.py. The fixture creates the sequence, audit table,
and duplicate-guard index itself (reusing the DDL from db.py) so the test is
self-contained and does not depend on migrations 002/003 having been applied.

Run: PYTHONPATH=. pytest platform/mcp/tools/test_schedule_inspection.py -v
"""

import asyncio
import contextlib
import os
from unittest.mock import patch

import asyncpg
import pytest
from dotenv import load_dotenv

from platform.mcp.db import _AUDIT_LOG_DDL, _PENDING_UNIQUE_DDL, _SEQUENCE_DDL
from platform.mcp.tools.write_tools import schedule_inspection

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


async def _fetch(query: str, *args):
    conn = await asyncpg.connect(_get_dsn())
    try:
        return await conn.fetch(query, *args)
    finally:
        await conn.close()


# ── Synthetic test data ───────────────────────────────────────────────────────

# IDs well above real fleet data (max ~43,002) and the MCP sequence floor.
_ELEVATOR_ID = 9_000_500
_NONEXISTENT_ID = 9_999_998
_ALL_TEST_IDS = [_ELEVATOR_ID, _NONEXISTENT_ID]

# Distinct future dates per test so independent tests never trip the duplicate
# guard against each other. All are far in the future (past dates are rejected).
_DATE_SUCCESS = "2030-01-15"
_DATE_PREVIEW = "2030-02-20"
_DATE_DUPLICATE = "2030-04-30"


@pytest.fixture(scope="module", autouse=True)
def seed_and_teardown():
    """
    Ensure the schema objects schedule_inspection needs exist (sequence, audit
    table, duplicate-guard index), seed one ACTIVE elevator, and clean everything
    up afterward. Reuses db.py's DDL so there is a single source of truth.
    """

    async def _seed() -> None:
        conn = await asyncpg.connect(_get_dsn())
        try:
            # Schema objects normally created by db.init_pool() / migrations 002+003.
            await conn.execute(_SEQUENCE_DDL)
            await conn.execute(_AUDIT_LOG_DDL)
            await conn.execute(_PENDING_UNIQUE_DDL)
            # A clean slate in case a previous aborted run left rows behind.
            await conn.execute(
                "DELETE FROM scheduling_audit_log WHERE elevator_id = ANY($1::int[])",
                _ALL_TEST_IDS,
            )
            await conn.execute(
                "DELETE FROM inspections WHERE elevator_id = ANY($1::int[])",
                _ALL_TEST_IDS,
            )
            await conn.execute(
                """
                INSERT INTO elevators (elevator_id, location, license_number, status)
                VALUES ($1, $2, $3, $4)
                ON CONFLICT (elevator_id) DO NOTHING
                """,
                _ELEVATOR_ID,
                "Test Site — Scheduling Unit",
                "TEST-SCHED-001",
                "ACTIVE",
            )
        finally:
            await conn.close()

    async def _cleanup() -> None:
        conn = await asyncpg.connect(_get_dsn())
        try:
            # audit_log has no FK — delete explicitly; inspections cascade via elevator.
            await conn.execute(
                "DELETE FROM scheduling_audit_log WHERE elevator_id = ANY($1::int[])",
                _ALL_TEST_IDS,
            )
            await conn.execute(
                "DELETE FROM elevators WHERE elevator_id = ANY($1::int[])",
                _ALL_TEST_IDS,
            )
        finally:
            await conn.close()

    asyncio.run(_seed())
    yield
    asyncio.run(_cleanup())


# ── 1. Success path ─────────────────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_success_path_writes_inspection_and_audit():
    """A confirmed write creates the inspection and a matching 'success' audit row."""
    with patch_get_connection():
        result = await schedule_inspection(
            elevator_id=_ELEVATOR_ID,
            inspection_date=_DATE_SUCCESS,
            reason="Annual periodic check",
            inspection_type="Periodic",
            confirmed=True,
        )

    assert result["success"] is True
    assert result["confirmed"] is True
    assert result["outcome"] == "Pending"
    new_id = result["inspection_id"]
    assert new_id >= 9_000_000  # came from mcp_inspection_id_seq

    inspections = await _fetch(
        "SELECT * FROM inspections WHERE inspection_id = $1", new_id
    )
    assert len(inspections) == 1
    row = inspections[0]
    assert row["elevator_id"] == _ELEVATOR_ID
    assert row["inspection_type"] == "Periodic"
    assert str(row["earliest_inspection_date"]) == _DATE_SUCCESS
    assert row["outcome"] == "Pending"

    audit = await _fetch(
        "SELECT * FROM scheduling_audit_log WHERE inspection_id = $1", new_id
    )
    assert len(audit) == 1
    arow = audit[0]
    assert arow["outcome"] == "success"
    assert arow["elevator_id"] == _ELEVATOR_ID
    assert arow["inspection_type"] == "Periodic"
    assert arow["error_message"] is None


# ── 2. Validation failure (runtime: elevator does not exist) ─────────────────────


@pytest.mark.asyncio
async def test_confirmed_write_to_missing_elevator_errors_and_audits():
    """A confirmed write to a nonexistent elevator: no inspection, one 'error' audit row."""
    with patch_get_connection():
        result = await schedule_inspection(
            elevator_id=_NONEXISTENT_ID,
            inspection_date="2030-03-25",
            reason="Should fail — elevator absent",
            inspection_type="Periodic",
            confirmed=True,
        )

    assert result["success"] is False
    assert "not found" in result["error"].lower()

    inspections = await _fetch(
        "SELECT * FROM inspections WHERE elevator_id = $1", _NONEXISTENT_ID
    )
    assert len(inspections) == 0

    audit = await _fetch(
        "SELECT * FROM scheduling_audit_log WHERE elevator_id = $1 AND outcome = 'error'",
        _NONEXISTENT_ID,
    )
    assert len(audit) == 1
    assert audit[0]["inspection_id"] is None
    assert "not found" in audit[0]["error_message"].lower()


@pytest.mark.asyncio
async def test_preview_of_missing_elevator_does_not_audit():
    """A Phase 1 preview of a bad ID is a failed lookup, not an auditable action."""
    with patch_get_connection(), _no_skip_confirmation():
        result = await schedule_inspection(
            elevator_id=_NONEXISTENT_ID,
            inspection_date="2030-03-26",
            reason="Preview only",
            confirmed=False,
        )

    assert result["success"] is False
    # No 'error' audit row was written for the unconfirmed lookup at this new date.
    audit = await _fetch(
        """SELECT * FROM scheduling_audit_log
           WHERE elevator_id = $1 AND inspection_date::text = $2""",
        _NONEXISTENT_ID,
        "2030-03-26",
    )
    assert len(audit) == 0


# ── 3. Duplicate confirmation (idempotency) ──────────────────────────────────────


@pytest.mark.asyncio
async def test_duplicate_confirmation_is_idempotent():
    """A second identical confirmed write is blocked; only one inspection survives."""
    args = dict(
        elevator_id=_ELEVATOR_ID,
        inspection_date=_DATE_DUPLICATE,
        reason="Duplicate guard check",
        inspection_type="Followup",
        confirmed=True,
    )

    with patch_get_connection():
        first = await schedule_inspection(**args)
        second = await schedule_inspection(**args)

    assert first["success"] is True
    assert second["success"] is False
    assert "already scheduled" in second["error"].lower()

    inspections = await _fetch(
        """SELECT * FROM inspections
           WHERE elevator_id = $1 AND earliest_inspection_date::text = $2
             AND inspection_type = 'Followup' AND outcome = 'Pending'""",
        _ELEVATOR_ID,
        _DATE_DUPLICATE,
    )
    assert len(inspections) == 1  # the duplicate was NOT written

    # The blocked attempt is recorded as an 'error' row alongside the 'success' row.
    audit = await _fetch(
        """SELECT outcome FROM scheduling_audit_log
           WHERE elevator_id = $1 AND inspection_date::text = $2
           ORDER BY performed_at""",
        _ELEVATOR_ID,
        _DATE_DUPLICATE,
    )
    outcomes = sorted(r["outcome"] for r in audit)
    assert outcomes == ["error", "success"]


# ── 4. Phase 1 preview writes nothing ────────────────────────────────────────────


@pytest.mark.asyncio
async def test_phase1_preview_writes_nothing():
    """confirmed=False returns a summary and touches neither table."""
    with patch_get_connection(), _no_skip_confirmation():
        result = await schedule_inspection(
            elevator_id=_ELEVATOR_ID,
            inspection_date=_DATE_PREVIEW,
            reason="Preview path",
            inspection_type="Periodic",
            confirmed=False,
        )

    assert result["success"] is False
    assert result["pending_confirmation"] is True
    assert "summary" in result and result["summary"]

    inspections = await _fetch(
        """SELECT * FROM inspections
           WHERE elevator_id = $1 AND earliest_inspection_date::text = $2""",
        _ELEVATOR_ID,
        _DATE_PREVIEW,
    )
    assert len(inspections) == 0

    audit = await _fetch(
        """SELECT * FROM scheduling_audit_log
           WHERE elevator_id = $1 AND inspection_date::text = $2""",
        _ELEVATOR_ID,
        _DATE_PREVIEW,
    )
    assert len(audit) == 0


# ── Helpers ──────────────────────────────────────────────────────────────────────


def patch_get_connection():
    """Patch write_tools.get_connection to use a direct connection (no pool)."""
    return patch("platform.mcp.tools.write_tools.get_connection", _direct_connection)


@contextlib.contextmanager
def _no_skip_confirmation():
    """
    Force the two-phase preview on, regardless of the ambient MCP_SKIP_CONFIRMATION
    (CI sets it to "1", which would otherwise make confirmed=False write immediately).
    """
    saved = os.environ.pop("MCP_SKIP_CONFIRMATION", None)
    try:
        yield
    finally:
        if saved is not None:
            os.environ["MCP_SKIP_CONFIRMATION"] = saved
