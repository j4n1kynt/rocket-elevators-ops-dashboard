"""
MCP tools for incident queries.

Tools: get_incident_count_last_year, get_elevator_incidents
"""

from platform.mcp.db import get_connection
from platform.mcp.tools._validators import validate_elevator_id, validate_limit


def get_incident_count_last_year() -> dict:
    """
    Return the total number of incidents reported in the previous calendar year.
    Date range is always computed server-side — no user input, no injection surface.
    """
    sql = """
        SELECT
            COUNT(*)                                                     AS total_incidents,
            COUNT(CASE WHEN fatal_injury THEN 1 END)                    AS fatal_incidents,
            COUNT(CASE WHEN injury_severity != 'none' THEN 1 END)       AS injury_incidents,
            (EXTRACT(YEAR FROM CURRENT_DATE) - 1)::int                  AS year_queried
        FROM incidents
        WHERE date_of_occurrence >= DATE_TRUNC('year', CURRENT_DATE) - INTERVAL '1 year'
          AND date_of_occurrence <  DATE_TRUNC('year', CURRENT_DATE)
    """
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(sql)
            row = cur.fetchone()
            cols = [d[0] for d in cur.description]

    return dict(zip(cols, row))


def get_elevator_incidents(elevator_id: int, limit: int = 20) -> dict:
    """
    Return incidents reported for a specific elevator, newest first.
    The narrative column is excluded by default — it can be very large.
    Use search_incident_narratives to search narrative text semantically.
    """
    elevator_id = validate_elevator_id(elevator_id)
    limit = validate_limit(limit, max_val=100)

    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "SELECT EXISTS(SELECT 1 FROM elevators WHERE elevator_id = %(eid)s)",
                {"eid": elevator_id},
            )
            if not cur.fetchone()[0]:
                return {"found": False, "elevator_id": elevator_id, "incidents": []}

            cur.execute(
                """
                SELECT
                    incident_id,
                    creation_date::text,
                    date_of_occurrence::text,
                    category,
                    incident_summary,
                    root_cause,
                    injury_severity,
                    fatal_injury
                FROM incidents
                WHERE elevator_id = %(eid)s
                ORDER BY date_of_occurrence DESC NULLS LAST
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
        "incidents": [dict(zip(cols, row)) for row in rows],
    }
