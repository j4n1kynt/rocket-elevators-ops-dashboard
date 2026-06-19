"""
MCP tools for incident queries.

Tools: get_incident_count_last_year, get_elevator_incidents
"""

from pydantic import ValidationError

from platform.mcp.db import get_connection
from platform.mcp.tools.models import GetElevatorIncidentsInput


async def get_incident_count_last_year() -> dict:
    """
    Return the total number of incidents reported in the previous calendar year.
    Date range is always computed server-side — no user input, no injection surface.
    """
    try:
        sql = """
            SELECT
                COUNT(*)                                                              AS total_incidents,
                COUNT(CASE WHEN fatal_injury THEN 1 END)                             AS fatal_incidents,
                COUNT(CASE WHEN injury_severity != 'none' THEN 1 END)                AS injury_incidents,
                (EXTRACT(YEAR FROM DATE '2016-11-22') - 1)::int                      AS year_queried
            FROM incidents
            WHERE date_of_occurrence >= DATE_TRUNC('year', DATE '2016-11-22') - INTERVAL '1 year'
              AND date_of_occurrence <  DATE_TRUNC('year', DATE '2016-11-22')
        """
        async with get_connection() as conn:
            row = await conn.fetchrow(sql)

        return {"source": "incidents table (aggregate)", **dict(row)}
    except ValidationError:
        raise
    except Exception as exc:
        return {"error": True, "message": str(exc)}


async def get_elevator_incidents(elevator_id: int, limit: int = 20) -> dict:
    """
    Return incidents reported for a specific elevator, newest first.
    The narrative column is excluded by default — it can be very large.
    Use search_incident_narratives to search narrative text semantically.
    """
    try:
        inp = GetElevatorIncidentsInput(elevator_id=elevator_id, limit=limit)

        async with get_connection() as conn:
            exists = await conn.fetchval(
                "SELECT EXISTS(SELECT 1 FROM elevators WHERE elevator_id = $1)",
                inp.elevator_id,
            )
            if not exists:
                return {"found": False, "elevator_id": inp.elevator_id, "incidents": []}

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
                inp.elevator_id,
                inp.limit,
            )

        return {
            "found": True,
            "elevator_id": inp.elevator_id,
            "total_returned": len(rows),
            "source": "incidents table",
            "incidents": [dict(r) for r in rows],
        }
    except ValidationError:
        raise
    except Exception as exc:
        return {"error": True, "message": str(exc)}
