"""
MCP tools for incident queries.

Tools: get_incident_count_last_year, get_elevator_incidents
"""

from platform.mcp.db import get_connection
from platform.mcp.tools._validators import validate_elevator_id, validate_limit


async def get_incident_count_last_year() -> dict:
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
    async with get_connection() as conn:
        row = await conn.fetchrow(sql)

    return dict(row)


async def get_elevator_incidents(elevator_id: int, limit: int = 20) -> dict:
    """
    Return incidents reported for a specific elevator, newest first.
    The narrative column is excluded by default — it can be very large.
    Use search_incident_narratives to search narrative text semantically.
    """
    elevator_id = validate_elevator_id(elevator_id)
    limit = validate_limit(limit, max_val=100)

    async with get_connection() as conn:
        exists = await conn.fetchval(
            "SELECT EXISTS(SELECT 1 FROM elevators WHERE elevator_id = $1)",
            elevator_id,
        )
        if not exists:
            return {"found": False, "elevator_id": elevator_id, "incidents": []}

        rows = await conn.fetch(
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
            WHERE elevator_id = $1
            ORDER BY date_of_occurrence DESC NULLS LAST
            LIMIT $2
            """,
            elevator_id,
            limit,
        )

    return {
        "found": True,
        "elevator_id": elevator_id,
        "total_returned": len(rows),
        "incidents": [dict(r) for r in rows],
    }
