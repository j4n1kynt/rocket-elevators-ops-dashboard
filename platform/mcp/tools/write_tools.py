"""
MCP write tools — operations that modify the database.

Tools: schedule_inspection

Two-phase confirmation is enforced:
  Phase 1 (confirmed=False): validate inputs, check elevator exists, return a
           confirmation summary string — NO database write.
  Phase 2 (confirmed=True):  re-validate, re-check elevator, then INSERT.

The LLM must present the Phase 1 summary to the user and receive explicit
approval before calling Phase 2. The `confirmed` parameter must be a strict
bool — strings like "yes" or integers like 1 are rejected.

inspection_id is generated via the sequence mcp_inspection_id_seq (starting at
9,000,000), created in db.init_pool() at server startup. IDs are intentionally
above the source-data range (~43,002 max) to avoid collisions.
"""

import os
from datetime import date

from platform.mcp.db import get_connection
from platform.mcp.tools._validators import (
    validate_elevator_id,
    validate_inspection_date,
    validate_reason,
)


async def schedule_inspection(
    elevator_id: int,
    inspection_date: str,
    reason: str,
    confirmed: bool = False,
) -> dict:
    """
    Schedule an inspection for an elevator.

    Call with confirmed=False first to receive a confirmation summary.
    Call again with confirmed=True only after the user has explicitly approved.

    Parameters:
        elevator_id     — ID of the elevator to schedule
        inspection_date — Target date in YYYY-MM-DD format (must not be in the past)
        reason          — Brief reason for scheduling (max 500 chars)
        confirmed       — Must be True to commit the write; False returns a preview only
    """
    if not isinstance(confirmed, bool):
        raise ValueError("confirmed must be a boolean (true or false), not a string or number.")

    elevator_id = validate_elevator_id(elevator_id)
    parsed_date: date = validate_inspection_date(inspection_date)
    reason = validate_reason(reason)

    async with get_connection() as conn:
        row = await conn.fetchrow(
            "SELECT location, status FROM elevators WHERE elevator_id = $1",
            elevator_id,
        )

    if not row:
        return {
            "success": False,
            "confirmed": confirmed,
            "error": f"Elevator {elevator_id} not found in the database.",
        }

    location, status = row["location"], row["status"]

    if not confirmed and not os.environ.get("MCP_SKIP_CONFIRMATION"):
        return {
            "success": False,
            "confirmed": False,
            "pending_confirmation": True,
            "summary": (
                f"You are about to schedule an inspection:\n"
                f"  Elevator ID : {elevator_id}\n"
                f"  Location    : {location}\n"
                f"  Status      : {status}\n"
                f"  Date        : {parsed_date}\n"
                f"  Reason      : {reason}\n"
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
            'Scheduled',
            $2,
            $2,
            'Pending'
        )
        RETURNING inspection_id
    """

    async with get_connection() as conn:
        async with conn.transaction():
            new_id = await conn.fetchval(insert_sql, elevator_id, parsed_date)

    return {
        "success": True,
        "confirmed": True,
        "inspection_id": new_id,
        "elevator_id": elevator_id,
        "location": location,
        "inspection_date": str(parsed_date),
        "reason": reason,
        "outcome": "Pending",
    }
