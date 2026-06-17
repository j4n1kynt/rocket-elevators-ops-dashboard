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
from platform.mcp.tools.models import SearchIncidentNarrativesInput, SearchMaintenanceDocsInput

# Update this constant once rag_preprocessing.py is updated for .txt ingestion.
# It must match the source_type value written into ChromaDB metadata during preprocessing.
# Example future value: "maintenance_txt"
MAINTENANCE_SOURCE_TYPE: str | None = None  # None = no filter, search all indexed documents

# Cosine distance from ChromaDB is 1 − cosine_similarity. Clamped to [0, 1] to guard
# against floating-point noise on unnormalized embeddings producing distance > 1.
SIMILARITY_THRESHOLD = 0.5


def search_maintenance_docs(query: str, n_results: int = 5) -> dict:
    """
    Semantically search maintenance documents stored in ChromaDB.
    Source format is .txt (not PDF). Returns the closest matching chunks
    with source document name and similarity score (0–1, higher is more similar).
    Results below SIMILARITY_THRESHOLD are filtered out.
    """
    try:
        inp = SearchMaintenanceDocsInput(query=query, n_results=n_results)

        where = {"source_type": MAINTENANCE_SOURCE_TYPE} if MAINTENANCE_SOURCE_TYPE else None

        results = rag_query(
            query_text=inp.query,
            n_results=inp.n_results,
            where=where,
        )

    if not results:
        return {
            "query": inp.query,
            "message": "No relevant documentation found",
            "total_returned": 0,
            "results": [],
        }

    for result in results:
        result["similarity_score"] = max(0.0, round(1 - result["distance"], 4))

    confident = [r for r in results if r["similarity_score"] >= SIMILARITY_THRESHOLD]

    if not confident:
        return {
            "query": inp.query,
            "message": "No confident matches found",
            "total_returned": 0,
            "results": [],
        }

    return {
        "query": inp.query,
        "message": None,
        "total_returned": len(confident),
        "results": confident,
    }


async def search_incident_narratives(query: str, limit: int = 5) -> dict:
    """
    Full-text search across incident narrative text using PostgreSQL FTS.
    Uses plainto_tsquery — treats the query as plain text, immune to tsquery injection.
    Returns matching incident records with a highlighted excerpt from the narrative.
    """
    try:
        inp = SearchIncidentNarrativesInput(query=query, limit=limit)

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
                    plainto_tsquery('english', $1),
                    'MaxWords=50, MinWords=20, StartSel=**, StopSel=**'
                ) AS narrative_excerpt,
                ts_rank(
                    to_tsvector('english', narrative),
                    plainto_tsquery('english', $1)
                ) AS relevance_score
            FROM incidents
            WHERE narrative IS NOT NULL
              AND to_tsvector('english', narrative) @@ plainto_tsquery('english', $1)
            ORDER BY relevance_score DESC
            LIMIT $2
        """

        async with get_connection() as conn:
            rows = await conn.fetch(sql, inp.query, inp.limit)

        return {
            "query": inp.query,
            "total_returned": len(rows),
            "results": [dict(r) for r in rows],
        }
    except Exception as exc:
        return {"error": True, "message": str(exc)}
