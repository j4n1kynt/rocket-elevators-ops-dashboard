"""
MCP tools for document and narrative search.

Tools: search_maintenance_docs, search_incident_narratives

search_maintenance_docs    — semantic search over ChromaDB (maintenance .txt documents)
search_incident_narratives — full-text search over incidents.narrative (PostgreSQL FTS)

Source type note: the RAG pipeline will use .txt files (not PDFs). When
rag_preprocessing.py is updated for .txt ingestion, set MAINTENANCE_SOURCE_TYPE
to match the source_type value written into ChromaDB metadata at that time.
Until then, the where filter is omitted so searches cover all indexed documents.

Note: incident narratives are NOT in ChromaDB (rag_preprocessing.py only processes
the maintenance document folder). FTS via PostgreSQL is used until FOUNDATION-1b
adds narratives to the vector store, at which point this tool can switch to
rag_query() with where={"source_type": "incident"}.
"""

from platform.mcp.db import get_connection
from platform.mcp.rag import rag_query
from platform.mcp.tools._validators import validate_limit, validate_query_string

# Update this constant once rag_preprocessing.py is updated for .txt ingestion.
# It must match the source_type value written into ChromaDB metadata during preprocessing.
# Example future value: "maintenance_txt"
MAINTENANCE_SOURCE_TYPE: str | None = None  # None = no filter, search all indexed documents


def search_maintenance_docs(query: str, n_results: int = 5) -> dict:
    """
    Semantically search maintenance documents stored in ChromaDB.
    Source format is .txt (not PDF). Returns the closest matching chunks
    with source document name and similarity distance.
    Lower distance = more similar (cosine distance, range 0–2).
    """
    query = validate_query_string(query, max_len=500)
    n_results = validate_limit(n_results, max_val=20)

    where = {"source_type": MAINTENANCE_SOURCE_TYPE} if MAINTENANCE_SOURCE_TYPE else None

    try:
        results = rag_query(
            query_text=query,
            n_results=n_results,
            where=where,
        )
    except RuntimeError:
        raise
    except Exception as exc:
        raise RuntimeError(f"Maintenance document search failed: {exc}") from exc

    return {
        "query": query,
        "total_returned": len(results),
        "results": results,
    }


def search_incident_narratives(query: str, limit: int = 5) -> dict:
    """
    Full-text search across incident narrative text using PostgreSQL FTS.
    Uses plainto_tsquery — treats the query as plain text, immune to tsquery injection.
    Returns matching incident records with a highlighted excerpt from the narrative.
    """
    query = validate_query_string(query, max_len=200)
    limit = validate_limit(limit, max_val=20)

    sql = """
        SELECT
            incident_id,
            elevator_id,
            date_of_occurrence::text,
            category,
            incident_summary,
            ts_headline(
                'english',
                narrative,
                plainto_tsquery('english', %(q)s),
                'MaxWords=50, MinWords=20, StartSel=**, StopSel=**'
            ) AS narrative_excerpt,
            ts_rank(
                to_tsvector('english', narrative),
                plainto_tsquery('english', %(q)s)
            ) AS relevance_score
        FROM incidents
        WHERE narrative IS NOT NULL
          AND to_tsvector('english', narrative) @@ plainto_tsquery('english', %(q)s)
        ORDER BY relevance_score DESC
        LIMIT %(limit)s
    """

    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(sql, {"q": query, "limit": limit})
            rows = cur.fetchall()
            cols = [d[0] for d in cur.description]

    return {
        "query": query,
        "total_returned": len(rows),
        "results": [dict(zip(cols, row)) for row in rows],
    }
