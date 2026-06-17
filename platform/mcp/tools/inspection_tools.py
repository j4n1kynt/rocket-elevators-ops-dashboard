"""
MCP tools for elevator, inspection, risk, and fleet queries.

Tools: get_tssa_shutdown_elevators, get_inspection_history,
       get_elevators_needing_followup, get_elevator_risk, get_fleet_stats
"""

from platform.mcp.db import get_connection
from platform.mcp.tools.models import (
    GetElevatorsNeedingFollowupInput,
    GetElevatorRiskInput,
    GetInspectionHistoryInput,
    GetTssaShutdownElevatorsInput,
)


async def get_tssa_shutdown_elevators(limit: int = 50) -> dict:
    """
    Return elevators whose most recent inspection has a non-passing outcome.

    Note: the database has no explicit 'shutdown' flag. TSSA compliance orders
    are represented as non-passing inspection outcomes (anything other than
    'passed' or 'all orders resolved'). This query surfaces those elevators.
    """
    try:
        inp = GetTssaShutdownElevatorsInput(limit=limit)

        sql = """
            WITH latest AS (
                SELECT DISTINCT ON (elevator_id)
                    elevator_id,
                    latest_inspection_date,
                    outcome,
                    inspection_type
                FROM inspections
                ORDER BY elevator_id, latest_inspection_date DESC NULLS LAST
            )
            SELECT
                e.elevator_id,
                e.location,
                e.status,
                l.latest_inspection_date::text,
                l.outcome,
                l.inspection_type
            FROM elevators e
            JOIN latest l ON l.elevator_id = e.elevator_id
            WHERE LOWER(l.outcome) NOT IN ('passed', 'all orders resolved')
              AND l.outcome IS NOT NULL
            ORDER BY l.latest_inspection_date ASC NULLS LAST
            LIMIT $1
        """
        async with get_connection() as conn:
            rows = await conn.fetch(sql, inp.limit)

        return {
            "count": len(rows),
            "note": "No explicit shutdown flag exists in the database. Results show elevators with non-passing most-recent inspection outcomes.",
            "elevators": [dict(r) for r in rows],
        }
    except Exception as exc:
        return {"error": True, "message": str(exc)}


async def get_inspection_history(elevator_id: int, limit: int = 20) -> dict:
    """Return the inspection history for a specific elevator, newest first."""
    try:
        inp = GetInspectionHistoryInput(elevator_id=elevator_id, limit=limit)

        async with get_connection() as conn:
            exists = await conn.fetchval(
                "SELECT EXISTS(SELECT 1 FROM elevators WHERE elevator_id = $1)",
                inp.elevator_id,
            )
            if not exists:
                return {"found": False, "elevator_id": inp.elevator_id, "inspections": []}

            rows = await conn.fetch(
                """
                SELECT
                    inspection_id,
                    COALESCE(inspection_type, '') AS inspection_type,
                    earliest_inspection_date::text,
                    latest_inspection_date::text,
                    outcome
                FROM inspections
                WHERE elevator_id = $1
                ORDER BY latest_inspection_date DESC NULLS LAST
                LIMIT $2
                """,
                inp.elevator_id,
                inp.limit,
            )

        return {
            "found": True,
            "elevator_id": inp.elevator_id,
            "total_returned": len(rows),
            "inspections": [dict(r) for r in rows],
        }
    except Exception as exc:
        return {"error": True, "message": str(exc)}


async def get_elevators_needing_followup(limit: int = 50) -> dict:
    """
    Return elevators whose most recent inspection outcome is 'Follow up'.
    Sorted oldest-first so operations can prioritize the most overdue.
    """
    try:
        inp = GetElevatorsNeedingFollowupInput(limit=limit)

        sql = """
            WITH latest AS (
                SELECT DISTINCT ON (elevator_id)
                    elevator_id,
                    latest_inspection_date,
                    outcome,
                    inspection_type
                FROM inspections
                ORDER BY elevator_id, latest_inspection_date DESC NULLS LAST
            )
            SELECT
                e.elevator_id,
                e.location,
                e.status,
                l.latest_inspection_date::text,
                l.outcome,
                l.inspection_type
            FROM elevators e
            JOIN latest l ON l.elevator_id = e.elevator_id
            WHERE LOWER(l.outcome) = 'follow up'
            ORDER BY l.latest_inspection_date ASC NULLS LAST
            LIMIT $1
        """
        async with get_connection() as conn:
            rows = await conn.fetch(sql, inp.limit)

        return {
            "count": len(rows),
            "elevators": [dict(r) for r in rows],
        }
    except Exception as exc:
        return {"error": True, "message": str(exc)}


async def get_elevator_risk(elevator_id: int) -> dict:
    """
    Return the ML risk prediction for a specific elevator.
    Distinguishes between 'elevator not found' and 'no prediction available'.
    """
    try:
        inp = GetElevatorRiskInput(elevator_id=elevator_id)

        async with get_connection() as conn:
            exists = await conn.fetchval(
                "SELECT EXISTS(SELECT 1 FROM elevators WHERE elevator_id = $1)",
                inp.elevator_id,
            )
            if not exists:
                return {"elevator_found": False, "prediction_found": False, "elevator_id": inp.elevator_id}

            row = await conn.fetchrow(
                """
                SELECT
                    elevator_id,
                    risk_score::float8,
                    risk_level,
                    risk_explanation,
                    model_version,
                    prediction_date::text
                FROM predictions
                WHERE elevator_id = $1
                """,
                inp.elevator_id,
            )
            if not row:
                return {"elevator_found": True, "prediction_found": False, "elevator_id": inp.elevator_id}

        return {"elevator_found": True, "prediction_found": True, **dict(row)}
    except Exception as exc:
        return {"error": True, "message": str(exc)}


async def get_fleet_stats() -> dict:
    """Return fleet-wide statistics: risk distribution, inspection pass rate, equipment types."""
    try:
        async with get_connection() as conn:
            risk_row = await conn.fetchrow("""
                SELECT
                    COUNT(*) AS total,
                    COUNT(CASE WHEN LOWER(p.risk_level) = 'low'    THEN 1 END) AS low,
                    COUNT(CASE WHEN LOWER(p.risk_level) = 'medium' THEN 1 END) AS medium,
                    COUNT(CASE WHEN LOWER(p.risk_level) = 'high'   THEN 1 END) AS high,
                    COUNT(CASE WHEN p.risk_level IS NULL            THEN 1 END) AS unknown
                FROM elevators e
                LEFT JOIN predictions p ON p.elevator_id = e.elevator_id
            """)

            passing_count = await conn.fetchval("""
                SELECT COUNT(DISTINCT elevator_id)
                FROM inspections
                WHERE LOWER(outcome) IN ('passed', 'all orders resolved')
            """)

            type_rows = await conn.fetch("""
                SELECT COALESCE(elevator_type, 'Unknown') AS type, COUNT(*) AS cnt
                FROM elevators
                GROUP BY elevator_type
                ORDER BY cnt DESC
            """)

        total = risk_row["total"] or 0
        pass_rate = round((passing_count / total * 100), 2) if total else 0.0

        return {
            "total_elevators": total,
            "risk_distribution": {
                "low": risk_row["low"],
                "medium": risk_row["medium"],
                "high": risk_row["high"],
                "unknown": risk_row["unknown"],
            },
            "inspection_pass_rate_pct": pass_rate,
            "equipment_type_distribution": {row["type"]: row["cnt"] for row in type_rows},
        }
    except Exception as exc:
        return {"error": True, "message": str(exc)}
