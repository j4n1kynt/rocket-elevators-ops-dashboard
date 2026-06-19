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
CHROMADB_PATH = os.environ.get("RAG_CHROMADB_PATH", "data/chromadb")
COLLECTION_NAME = "maintenance_documents"
EMBEDDING_MODEL = "BAAI/bge-small-en-v1.5"
MODEL_VERSION   = "bge-small-en-v1.5-v1"  # written into chunk metadata by rag_preprocessing.py

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


def warm_rag() -> None:
    """
    Eagerly load the ChromaDB client and the embedding model so the first real
    query does not pay the cold-start cost.

    On a constrained host (e.g. Render free tier) the instance spins down when
    idle; the first query after wake must both start the container and lazily load
    the ~125 MB sentence-transformer model, which can exceed the Go API's per-call
    MCP timeout and force a no-data advisory fallback. Calling this at server
    startup moves that cost off the request path.

    Raises on failure (e.g. ChromaDB not yet populated). The caller decides whether
    that is fatal — at startup it should be treated as non-fatal so DB-only tools
    still work. A trivial encode is included because SentenceTransformer defers some
    weight initialization until the first encode, not construction.
    """
    _get_client()
    model = _get_model()
    model.encode("warmup", normalize_embeddings=False)


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
        if os.environ.get("ALLOW_EMPTY_CHROMADB"):
            return []
        raise RuntimeError(
            f"ChromaDB collection '{collection_name}' not found: {exc}. "
            "Run 'py -3 intelligence/rag_preprocessing.py' to create and populate it."
        ) from exc

    try:
        embedding = model.encode(query_text, normalize_embeddings=False).tolist()
    except Exception as exc:
        raise RuntimeError(f"Failed to generate embedding for query: {exc}") from exc

    # Guard against a stale index built with a different embedding model.
    # Dimension check alone is no longer sufficient: bge-small-en-v1.5 and the
    # old all-MiniLM-L6-v2 are both 384-dim, so the sizes would match but results
    # would be meaningless. Also check model_version stored in chunk metadata.
    try:
        peek = collection.peek(limit=1)
        stored = peek.get("embeddings")
        stored_meta = peek.get("metadatas")
        if stored is not None and len(stored) > 0 and len(stored[0]) != len(embedding):
            raise RuntimeError(
                f"Embedding dimension mismatch: index has {len(stored[0])}-dim vectors "
                f"but '{EMBEDDING_MODEL}' produces {len(embedding)}-dim. "
                "Re-run 'py -3 intelligence/rag_preprocessing.py --force' to rebuild the index."
            )
        if stored_meta and len(stored_meta) > 0:
            stored_version = (stored_meta[0] or {}).get("model_version", "")
            if stored_version and stored_version != MODEL_VERSION:
                raise RuntimeError(
                    f"Index model version mismatch: index was built with '{stored_version}' "
                    f"but current model version is '{MODEL_VERSION}'. "
                    "Re-run 'py -3 intelligence/rag_preprocessing.py --force' to rebuild the index."
                )
    except RuntimeError:
        raise
    except Exception:
        pass  # peek is best-effort; don't block queries if unavailable

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
