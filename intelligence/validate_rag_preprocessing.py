"""
FOUNDATION-1: Validate ChromaDB collection after rag_preprocessing.py.
Usage: py -3 intelligence/validate_rag_preprocessing.py
Exits 0 if all checks pass, 1 if any fail.
"""

import os
import sys
import numpy as np
import chromadb
from sentence_transformers import SentenceTransformer

CHROMADB_PATH   = os.getenv("RAG_CHROMADB_PATH", "data/chromadb")
COLLECTION_NAME = "maintenance_documents"
MIN_CHUNKS      = 30    # floor for 6 PDFs at 500-token chunk size
EXPECTED_DIM    = 1024


def _ok(flag):
    return "OK" if flag else "FAIL"


def main():
    print("=== FOUNDATION-1 RAG Preprocessing Validation ===\n")

    client = chromadb.PersistentClient(path=CHROMADB_PATH)
    try:
        coll = client.get_collection(COLLECTION_NAME)
    except Exception:
        print(f"FAIL: collection '{COLLECTION_NAME}' not found. Run rag_preprocessing.py first.")
        sys.exit(1)

    failures = []

    # [1] Count
    count = coll.count()
    ok = count >= MIN_CHUNKS
    print(f"[1] Collection count: {count}  [{_ok(ok)}]  (expected >={MIN_CHUNKS})")
    if not ok:
        failures.append(f"count {count} < {MIN_CHUNKS}")

    # [2] Metadata completeness
    sample = coll.get(limit=10, include=["metadatas", "embeddings", "documents"])
    required_keys = {"doc_name", "chunk_sequence", "token_count", "source_type", "created_at", "model_version"}
    missing = [k for m in sample["metadatas"] for k in required_keys if k not in m]
    meta_ok = len(missing) == 0
    print(f"[2] Metadata complete: [{_ok(meta_ok)}]" + (f"  missing keys: {set(missing)}" if not meta_ok else ""))
    if not meta_ok:
        failures.append("incomplete metadata")

    # [3] Embedding validity (shape + NaN)
    embed_ok = True
    for vec in sample["embeddings"]:
        arr = np.array(vec)
        if arr.shape[0] != EXPECTED_DIM or np.isnan(arr).any():
            embed_ok = False
            break
    print(f"[3] Embeddings valid (dim={EXPECTED_DIM}, no NaN): [{_ok(embed_ok)}]")
    if not embed_ok:
        failures.append("invalid embeddings")

    # [4] Similarity search — embed query with our own model to avoid ChromaDB ONNX download
    try:
        _model = SentenceTransformer("BAAI/bge-large-en-v1.5")
        query_vec = _model.encode(["elevator safety inspection"], show_progress_bar=False).tolist()
        results = coll.query(query_embeddings=query_vec, n_results=3)
        search_ok = len(results["ids"][0]) == 3
    except Exception as exc:
        search_ok = False
        print(f"    search error: {exc}")
    print(f"[4] Similarity search works: [{_ok(search_ok)}]")
    if not search_ok:
        failures.append("similarity search failed")

    if search_ok:
        print("\nTop 3 results for 'elevator safety inspection':")
        for i, (doc_id, doc_text, meta) in enumerate(zip(
            results["ids"][0], results["documents"][0], results["metadatas"][0]
        )):
            print(f"  {i+1}. [{meta['doc_name']}] {doc_id}")
            print(f"     {doc_text[:120].strip()}...")

    # [5] Doc coverage — all 6 source docs represented
    all_meta = coll.get(limit=count, include=["metadatas"])
    doc_names = {m["doc_name"] for m in all_meta["metadatas"]}
    expected_docs = {"10078", "10079", "10080", "10081", "10082", "10083"}
    covered = doc_names & expected_docs
    coverage_ok = covered == expected_docs
    print(f"[5] Doc coverage ({len(covered)}/6): [{_ok(coverage_ok)}]" + (
        f"  missing: {expected_docs - covered}" if not coverage_ok else ""
    ))
    if not coverage_ok:
        failures.append(f"missing docs: {expected_docs - covered}")

    print()
    if failures:
        print(f"FAIL: {len(failures)} check(s) failed: {', '.join(failures)}")
        sys.exit(1)
    else:
        print("PASS: All checks passed")
        sys.exit(0)


if __name__ == "__main__":
    main()
