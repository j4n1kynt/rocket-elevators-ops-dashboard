"""
MCP tools for elevator, inspection, risk, and fleet queries.

Tools: get_tssa_shutdown_elevators, get_inspection_history,
       get_elevators_needing_followup, get_elevator_risk, get_fleet_stats
"""

from platform.mcp.db import get_connection
from platform.mcp.tools._validators import validate_elevator_id, validate_limit


def get_tssa_shutdown_elevators(limit: int = 50) -> dict:
    """
    Return elevators whose most recent inspection has a non-passing outcome.

    Note: the database has no explicit 'shutdown' flag. TSSA compliance orders
    are represented as non-passing inspection outcomes (anything other than
    'passed' or 'all orders resolved'). This query surfaces those elevators.
    """
    limit = validate_limit(limit, max_val=200)

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
        LIMIT %(limit)s
    """
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(sql, {"limit": limit})
            rows = cur.fetchall()
            cols = [d[0] for d in cur.description]

    return {
        "count": len(rows),
        "note": "No explicit shutdown flag exists in the database. Results show elevators with non-passing most-recent inspection outcomes.",
        "elevators": [dict(zip(cols, row)) for row in rows],
    }


def get_inspection_history(elevator_id: int, limit: int = 20) -> dict:
    """Return the inspection history for a specific elevator, newest first."""
    elevator_id = validate_elevator_id(elevator_id)
    limit = validate_limit(limit, max_val=100)

    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "SELECT EXISTS(SELECT 1 FROM elevators WHERE elevator_id = %(eid)s)",
                {"eid": elevator_id},
            )
            if not cur.fetchone()[0]:
                return {"found": False, "elevator_id": elevator_id, "inspections": []}

            cur.execute(
                """
                SELECT
                    inspection_id,
                    COALESCE(inspection_type, '') AS inspection_type,
                    earliest_inspection_date::text,
                    latest_inspection_date::text,
                    outcome
                FROM inspections
                WHERE elevator_id = %(eid)s
                ORDER BY latest_inspection_date DESC NULLS LAST
                LIMIT %(limit)s
                """,
                {"eid": elevator_id, "limit": limit},
            )
            rows = cur.fetchall()
            cols = [d[0] for d in cur.description]

    return {
        "found": True,
        "elevator_id": elevator_id,
        "total_returned": len(rows),
        "inspections": [dict(zip(cols, row)) for row in rows],
    }


def get_elevators_needing_followup(limit: int = 50) -> dict:
    """
    Return elevators whose most recent inspection outcome is 'Follow up'.
    Sorted oldest-first so operations can prioritize the most overdue.
    """
    limit = validate_limit(limit, max_val=200)

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
        LIMIT %(limit)s
    """
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(sql, {"limit": limit})
            rows = cur.fetchall()
            cols = [d[0] for d in cur.description]

    return {
        "count": len(rows),
        "elevators": [dict(zip(cols, row)) for row in rows],
    }


def get_elevator_risk(elevator_id: int) -> dict:
    """
    Return the ML risk prediction for a specific elevator.
    Distinguishes between 'elevator not found' and 'no prediction available'.
    """
    elevator_id = validate_elevator_id(elevator_id)

    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "SELECT EXISTS(SELECT 1 FROM elevators WHERE elevator_id = %(eid)s)",
                {"eid": elevator_id},
            )
            if not cur.fetchone()[0]:
                return {"elevator_found": False, "prediction_found": False, "elevator_id": elevator_id}

            cur.execute(
                """
                SELECT
                    elevator_id,
                    risk_score::float8,
                    risk_level,
                    risk_explanation,
                    model_version,
                    prediction_date::text
                FROM predictions
                WHERE elevator_id = %(eid)s
                """,
                {"eid": elevator_id},
            )
            row = cur.fetchone()
            if not row:
                return {"elevator_found": True, "prediction_found": False, "elevator_id": elevator_id}

            cols = [d[0] for d in cur.description]

    return {"elevator_found": True, "prediction_found": True, **dict(zip(cols, row))}


def get_fleet_stats() -> dict:
    """Return fleet-wide statistics: risk distribution, inspection pass rate, equipment types."""
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute("""
                SELECT
                    COUNT(*) AS total,
                    COUNT(CASE WHEN LOWER(p.risk_level) = 'low'    THEN 1 END) AS low,
                    COUNT(CASE WHEN LOWER(p.risk_level) = 'medium' THEN 1 END) AS medium,
                    COUNT(CASE WHEN LOWER(p.risk_level) = 'high'   THEN 1 END) AS high,
                    COUNT(CASE WHEN p.risk_level IS NULL            THEN 1 END) AS unknown
                FROM elevators e
                LEFT JOIN predictions p ON p.elevator_id = e.elevator_id
            """)
            risk_row = cur.fetchone()

            cur.execute("""
                SELECT COUNT(DISTINCT elevator_id)
                FROM inspections
                WHERE LOWER(outcome) IN ('passed', 'all orders resolved')
            """)
            passing_count = cur.fetchone()[0]

            cur.execute("""
                SELECT COALESCE(elevator_type, 'Unknown') AS type, COUNT(*) AS cnt
                FROM elevators
                GROUP BY elevator_type
                ORDER BY cnt DESC
            """)
            type_rows = cur.fetchall()

    total = risk_row[0] or 0
    pass_rate = round((passing_count / total * 100), 2) if total else 0.0

    return {
        "total_elevators": total,
        "risk_distribution": {
            "low": risk_row[1],
            "medium": risk_row[2],
            "high": risk_row[3],
            "unknown": risk_row[4],
        },
        "inspection_pass_rate_pct": pass_rate,
        "equipment_type_distribution": {row[0]: row[1] for row in type_rows},
    }
