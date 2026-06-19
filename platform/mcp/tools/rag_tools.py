"""
MCP tools for document and narrative search.

Tools: search_maintenance_docs, search_incident_narratives

search_maintenance_docs    — semantic search over ChromaDB (maintenance .txt documents)
search_incident_narratives — semantic search over incident narratives in ChromaDB

Source type note: the RAG pipeline will use .txt files (not PDFs). When
rag_preprocessing.py is updated for .txt ingestion, set MAINTENANCE_SOURCE_TYPE
to match the source_type value written into ChromaDB metadata at that time.
Until then, the where filter is omitted so searches cover all indexed documents.

Note: incident narratives live in their own ChromaDB collection
("incident_narratives"), populated by intelligence/index_incident_narratives.py
(FOUNDATION-1b). Semantic embeddings let retrieval succeed even when the query
phrasing does not match the narrative text — which PostgreSQL FTS could not do.
"""

from pydantic import ValidationError

from platform.mcp.rag import rag_query
from platform.mcp.tools.models import SearchIncidentNarrativesInput, SearchMaintenanceDocsInput

# Update this constant once rag_preprocessing.py is updated for .txt ingestion.
# It must match the source_type value written into ChromaDB metadata during preprocessing.
# Example future value: "maintenance_txt"
MAINTENANCE_SOURCE_TYPE: str | None = None  # None = no filter, search all indexed documents

# Dedicated ChromaDB collection for incident narratives, populated by
# intelligence/index_incident_narratives.py. Kept separate from maintenance docs.
INCIDENT_COLLECTION = "incident_narratives"

# Cosine distance from ChromaDB is 1 − cosine_similarity. Clamped to [0, 1] to guard
# against floating-point noise on unnormalized embeddings producing distance > 1.
SIMILARITY_THRESHOLD = 0.5

# Incident narratives need a HIGHER cut than maintenance docs. Measured against the
# real 2,382-narrative index: relevant queries score 0.76–0.90 at top-1, while clearly
# irrelevant queries (e.g. "best pasta recipe") still floor at ~0.51–0.57 because
# bge-small-en-v1.5 with unnormalized embeddings compresses the similarity range.
# 0.65 sits in the empty gap between the two clusters — rejects junk, keeps real hits.
INCIDENT_SIMILARITY_THRESHOLD = 0.65


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

        scored = []
        for result in results:
            meta = result.get("metadata") or {}
            scored.append({
                "text": result.get("text", ""),
                "source_name": f"Maintenance Document {meta.get('doc_name', 'unknown')}",
                "similarity_score": max(0.0, round(1 - result.get("distance", 1.0), 4)),
            })

        confident = [r for r in scored if r["similarity_score"] >= SIMILARITY_THRESHOLD]

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
    except ValidationError:
        raise
    except Exception as exc:
        return {"error": True, "message": str(exc)}


def search_incident_narratives(query: str, limit: int = 5) -> dict:
    """
    Semantically search incident narratives stored in ChromaDB.
    Embeddings let retrieval succeed even when the query phrasing does not match
    the narrative wording. Returns the closest incidents with source attribution
    and similarity score (0–1, higher is more similar). Results below
    INCIDENT_SIMILARITY_THRESHOLD are filtered out.
    """
    try:
        inp = SearchIncidentNarrativesInput(query=query, limit=limit)

        results = rag_query(
            query_text=inp.query,
            n_results=inp.limit,
            collection_name=INCIDENT_COLLECTION,
        )

        if not results:
            return {
                "query": inp.query,
                "message": "No relevant incidents found",
                "total_returned": 0,
                "results": [],
            }

        scored = []
        for result in results:
            meta = result.get("metadata") or {}
            incident_id = meta.get("incident_id")
            date_of_occurrence = meta.get("date_of_occurrence")
            scored.append({
                "incident_id": incident_id,
                "elevator_id": meta.get("elevator_id"),
                "date_of_occurrence": date_of_occurrence,
                "category": meta.get("category"),
                "incident_summary": meta.get("incident_summary"),
                "narrative": result.get("text", ""),
                "source_name": f"Incident #{incident_id} ({date_of_occurrence})",
                "similarity_score": max(0.0, round(1 - result.get("distance", 1.0), 4)),
            })

        confident = [r for r in scored if r["similarity_score"] >= INCIDENT_SIMILARITY_THRESHOLD]

        if not confident:
            return {
                "query": inp.query,
                "message": "No relevant incidents found",
                "total_returned": 0,
                "results": [],
            }

        return {
            "query": inp.query,
            "message": None,
            "total_returned": len(confident),
            "results": confident,
        }
    except ValidationError:
        raise
    except Exception as exc:
        return {"error": True, "message": str(exc)}
