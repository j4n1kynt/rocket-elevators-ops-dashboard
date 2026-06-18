"""
CI fixture: seed a minimal ChromaDB collection so validate_rag_preprocessing.py
can run in CI without real PDFs.

Creates 36 synthetic chunks (6 per source doc × 6 doc names) using
BAAI/bge-large-en-v1.5 — the same model used by rag_preprocessing.py —
so the dimension and metadata checks in validate_rag_preprocessing.py pass.

Usage (CI):
    RAG_CHROMADB_PATH=/tmp/chromadb-ci PYTHONPATH=. python intelligence/tests/seed_rag_ci.py
"""

import os
import sys
from datetime import datetime, timezone

import chromadb
from sentence_transformers import SentenceTransformer

CHROMADB_PATH   = os.getenv("RAG_CHROMADB_PATH", "data/chromadb")
COLLECTION_NAME = "maintenance_documents"
EMBEDDING_MODEL = "BAAI/bge-large-en-v1.5"
DOC_NAMES       = ["10078", "10079", "10080", "10081", "10082", "10083"]

SYNTHETIC_TEXTS = [
    "Elevator safety inspection checklist. Verify door sensors and emergency stop.",
    "Hydraulic system maintenance procedure. Inspect cylinder and fluid levels.",
    "Annual inspection requirements for traction elevators. Safety devices test.",
    "Emergency lighting check procedure. Battery backup verification steps.",
    "Maintenance log entry format. Record date, technician, and parts replaced.",
    "Lubrication schedule for elevator guide rails and roller assemblies.",
]


def main() -> None:
    print(f"Loading model '{EMBEDDING_MODEL}'...")
    model = SentenceTransformer(EMBEDDING_MODEL)
    dim = model.get_sentence_embedding_dimension()
    print(f"Model loaded. Embedding dimension: {dim}")

    client = chromadb.PersistentClient(path=CHROMADB_PATH)

    try:
        client.delete_collection(COLLECTION_NAME)
    except Exception:
        pass

    coll = client.create_collection(
        COLLECTION_NAME,
        metadata={"hnsw:space": "cosine"},
    )

    ids, docs, metas, embeddings = [], [], [], []
    now = datetime.now(timezone.utc).isoformat()

    for doc_name in DOC_NAMES:
        for seq, text in enumerate(SYNTHETIC_TEXTS):
            chunk_text = f"[{doc_name}] {text}"
            emb = model.encode(chunk_text, normalize_embeddings=False).tolist()
            ids.append(f"{doc_name}_chunk_{seq:03d}")
            docs.append(chunk_text)
            metas.append({
                "doc_name":       doc_name,
                "chunk_sequence": seq,
                "token_count":    len(text.split()),
                "source_type":    "maintenance_pdf",
                "created_at":     now,
                "model_version":  EMBEDDING_MODEL,
            })
            embeddings.append(emb)

    coll.add(documents=docs, ids=ids, metadatas=metas, embeddings=embeddings)
    print(f"Seeded {len(docs)} chunks into '{COLLECTION_NAME}' at '{CHROMADB_PATH}'")
    sys.exit(0)


if __name__ == "__main__":
    main()
