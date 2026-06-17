"""
ChromaDB query helper for the FastMCP server.

Reads from the same collection populated by intelligence/rag_preprocessing.py.
EMBEDDING_MODEL and normalize_embeddings must stay in sync with rag_preprocessing.py constants.

Do not import from rag_preprocessing.py — that script is a write pipeline with async PDF
extraction logic that should not run at server startup.
"""

import os
import threading
from pathlib import Path

from dotenv import load_dotenv

load_dotenv()

# Must match constants in intelligence/rag_preprocessing.py
CHROMADB_PATH = os.environ.get("MCP_CHROMADB_PATH", "data/chromadb")
COLLECTION_NAME = "maintenance_documents"
EMBEDDING_MODEL = "all-MiniLM-L6-v2"

_client = None
_model = None
_client_lock = threading.Lock()
_model_lock = threading.Lock()


def _get_client():
    global _client
    if _client is None:
        with _client_lock:
            if _client is None:
                import chromadb  # lazy — not needed at server startup
                resolved = str(Path(CHROMADB_PATH).resolve())
                try:
                    _client = chromadb.PersistentClient(path=resolved)
                except Exception as exc:
                    raise RuntimeError(
                        f"Failed to open ChromaDB at '{resolved}': {exc}. "
                        "Run 'py -3 intelligence/rag_preprocessing.py' first to populate the store."
                    ) from exc
    return _client


def _get_model():
    global _model
    if _model is None:
        with _model_lock:
            if _model is None:
                from sentence_transformers import SentenceTransformer  # lazy — not needed at server startup
                try:
                    _model = SentenceTransformer(EMBEDDING_MODEL)
                except Exception as exc:
                    raise RuntimeError(
                        f"Failed to load embedding model '{EMBEDDING_MODEL}': {exc}. "
                        "Ensure sentence-transformers is installed: "
                        "pip install -r platform/mcp/requirements.txt"
                    ) from exc
    return _model


def rag_query(
    query_text: str,
    n_results: int = 5,
    collection_name: str = COLLECTION_NAME,
    where: dict | None = None,
) -> list[dict]:
    """
    Embed query_text and retrieve the closest chunks from ChromaDB.

    normalize_embeddings=False must match rag_preprocessing.py — using True here when
    False was used during indexing produces incorrect cosine similarities.

    Returns an empty list if the collection exists but has no matching results.
    """
    client = _get_client()
    model = _get_model()

    try:
        collection = client.get_collection(name=collection_name)
    except Exception as exc:
        raise RuntimeError(
            f"ChromaDB collection '{collection_name}' not found: {exc}. "
            "Run 'py -3 intelligence/rag_preprocessing.py' to create and populate it."
        ) from exc

    try:
        embedding = model.encode(query_text, normalize_embeddings=False).tolist()
    except Exception as exc:
        raise RuntimeError(f"Failed to generate embedding for query: {exc}") from exc

    kwargs: dict = {"query_embeddings": [embedding], "n_results": n_results}
    if where:
        kwargs["where"] = where

    try:
        results = collection.query(**kwargs)
    except Exception as exc:
        raise RuntimeError(f"ChromaDB query failed: {exc}") from exc

    docs = results.get("documents", [[]])[0]
    if not docs:
        return []

    return [
        {
            "text": doc,
            "metadata": results["metadatas"][0][i],
            "distance": results["distances"][0][i],
        }
        for i, doc in enumerate(docs)
    ]
