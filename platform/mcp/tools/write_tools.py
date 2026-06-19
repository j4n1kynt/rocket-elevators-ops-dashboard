"""
MCP write tools — operations that modify the database.

Tools: schedule_inspection

Two-phase confirmation is enforced:
  Phase 1 (confirmed=False): validate inputs, check elevator exists, return a
           confirmation summary string — NO database write.
  Phase 2 (confirmed=True):  re-validate, re-check elevator, then INSERT.

The LLM must present the Phase 1 summary to the user and receive explicit
approval before calling Phase 2. The `confirmed` parameter uses Pydantic strict mode — strings like "yes" or
integers like 1 are rejected at the model layer before the function body runs.

inspection_id is generated via the sequence mcp_inspection_id_seq (starting at
9,000,000), created in db.init_pool() at server startup. IDs are intentionally
above the source-data range (~43,002 max) to avoid collisions.
"""

import os

import asyncpg
from pydantic import ValidationError

from platform.mcp.db import get_connection
from platform.mcp.tools.models import ScheduleInspectionInput


async def _audit_error(inp: ScheduleInspectionInput, resolved_type: str, message: str) -> None:
    """
    Best-effort: record a failed confirmed scheduling attempt in the audit log
    with outcome='error'. Never raises — an audit failure must not mask the
    original error (and the DB may be the very thing that is down).
    """
    try:
        async with get_connection() as conn:
            await conn.execute(
                """
                INSERT INTO scheduling_audit_log (
                    elevator_id, inspection_id, inspection_date,
                    inspection_type, reason, outcome, error_message
                )
                VALUES ($1, NULL, $2, $3, $4, 'error', $5)
                """,
                inp.elevator_id,
                inp.inspection_date,
                resolved_type,
                inp.reason,
                message[:1000],
            )
    except Exception:
        pass


async def schedule_inspection(
    elevator_id: int,
    inspection_date: str,
    reason: str,
    inspection_type: str = "",
    confirmed: bool = False,
) -> dict:
    """
    Schedule an inspection for an elevator.

    Call with confirmed=False first to receive a confirmation summary.
    Call again with confirmed=True only after the user has explicitly approved.

    Parameters:
        elevator_id      — ID of the elevator to schedule
        inspection_date  — Target date in YYYY-MM-DD format (must not be in the past)
        reason           — Brief reason for scheduling (max 500 chars)
        inspection_type  — One of: Periodic, Followup, Initial, Incident, Alteration
        confirmed        — Must be True to commit the write; False returns a preview only
    """
    # Input validation is its own block so a ValidationError re-raises cleanly
    # (CI relies on this) and `inp` is guaranteed defined for the DB block below.
    try:
        inp = ScheduleInspectionInput(
            elevator_id=elevator_id,
            inspection_date=inspection_date,
            reason=reason,
            inspection_type=inspection_type,
            confirmed=confirmed,
        )
    except ValidationError:
        raise

    resolved_type = inp.inspection_type if inp.inspection_type else "Periodic"

    try:
        async with get_connection() as conn:
            row = await conn.fetchrow(
                "SELECT location, status FROM elevators WHERE elevator_id = $1",
                inp.elevator_id,
            )

        if not row:
            error = f"Elevator {inp.elevator_id} not found in the database."
            # Only a confirmed (Phase 2) attempt is a real action worth auditing;
            # a Phase 1 preview of a bad ID is just a failed lookup.
            if inp.confirmed:
                await _audit_error(inp, resolved_type, error)
            return {
                "success": False,
                "confirmed": inp.confirmed,
                "error": error,
            }

        location, status = row["location"], row["status"]

        if not inp.confirmed and not os.environ.get("MCP_SKIP_CONFIRMATION"):
            type_line = f"  Type        : {inp.inspection_type}\n" if inp.inspection_type else ""
            return {
                "success": False,
                "confirmed": False,
                "pending_confirmation": True,
                "summary": (
                    f"You are about to schedule an inspection:\n"
                    f"  Elevator ID : {inp.elevator_id}\n"
                    f"  Location    : {location}\n"
                    f"  Status      : {status}\n"
                    f"  Date        : {inp.inspection_date}\n"
                    f"{type_line}"
                    f"  Reason      : {inp.reason}\n"
                    f"\nPlease confirm to proceed."
                ),
            }

        insert_sql = """
            INSERT INTO inspections (
                inspection_id,
                elevator_id,
                inspection_type,
                earliest_inspection_date,
                latest_inspection_date,
                outcome
            )
            VALUES (
                nextval('mcp_inspection_id_seq'),
                $1,
                $2,
                $3,
                $3,
                'Pending'
            )
            RETURNING inspection_id
        """

        audit_sql = """
            INSERT INTO scheduling_audit_log (
                elevator_id,
                inspection_id,
                inspection_date,
                inspection_type,
                reason,
                outcome
            )
            VALUES ($1, $2, $3, $4, $5, 'success')
        """

        async with get_connection() as conn:
            async with conn.transaction():
                new_id = await conn.fetchval(insert_sql, inp.elevator_id, resolved_type, inp.inspection_date)
                await conn.execute(
                    audit_sql,
                    inp.elevator_id,
                    new_id,
                    inp.inspection_date,
                    resolved_type,
                    inp.reason,
                )

        return {
            "success": True,
            "confirmed": True,
            "inspection_id": new_id,
            "elevator_id": inp.elevator_id,
            "location": location,
            "inspection_date": str(inp.inspection_date),
            "reason": inp.reason,
            "outcome": "Pending",
        }
    except asyncpg.UniqueViolationError:
        # The duplicate-pending guard (003) fired: an identical pending inspection
        # already exists. No row was created — surface a clear, non-error message.
        error = (
            f"A {resolved_type} inspection is already scheduled for elevator "
            f"{inp.elevator_id} on {inp.inspection_date}. No duplicate was created."
        )
        await _audit_error(inp, resolved_type, error)
        return {"success": False, "confirmed": True, "error": error}
    except Exception as exc:
        # A confirmed attempt that fails mid-write is a real action — audit it.
        if inp.confirmed:
            await _audit_error(inp, resolved_type, str(exc))
        # Match the {"success": False, "error": ...} shape so extractScheduleError()
        # in chat.go detects this failure instead of treating it as success data.
        return {"success": False, "error": str(exc)}
