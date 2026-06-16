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
9,000,000), created at server startup. IDs are intentionally above the source-data
range (~43,002 max) to avoid collisions.
"""

from datetime import date

from platform.mcp.db import get_connection
from platform.mcp.tools._validators import (
    validate_elevator_id,
    validate_inspection_date,
    validate_reason,
)

_SEQUENCE_DDL = """
    CREATE SEQUENCE IF NOT EXISTS mcp_inspection_id_seq
    START 9000000
    INCREMENT 1
    NO CYCLE
"""


def _ensure_sequence() -> None:
    """Create the inspection ID sequence if it does not exist. Called at server startup."""
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(_SEQUENCE_DDL)
        conn.commit()


def schedule_inspection(
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

    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "SELECT location, status FROM elevators WHERE elevator_id = %(eid)s",
                {"eid": elevator_id},
            )
            row = cur.fetchone()

    if not row:
        return {
            "success": False,
            "confirmed": confirmed,
            "error": f"Elevator {elevator_id} not found in the database.",
        }

    location, status = row

    if not confirmed:
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
            %(elevator_id)s,
            'Scheduled',
            %(inspection_date)s,
            %(inspection_date)s,
            'Pending'
        )
        RETURNING inspection_id
    """

    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(insert_sql, {
                "elevator_id": elevator_id,
                "inspection_date": parsed_date,
            })
            new_id = cur.fetchone()[0]
        conn.commit()

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
