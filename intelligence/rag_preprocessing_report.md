# RAG Preprocessing Report
**Run date:** 2026-06-16 18:22 UTC
**Duration:** 27.4s

## Summary
| Metric | Value |
|--------|-------|
| PDFs processed | 6/6 |
| Total raw text | 209,725 chars |
| Chunks created | 275 |
| Embeddings OK  | 275/275 |
| ChromaDB count | 275 |
| Duration       | 27.4s |

## Extension Points
- Incident narratives: add `chunk_incident_narratives()` with `source_type=incident` (FOUNDATION-1b)
- Fine-tuned retriever: swap `EMBEDDING_MODEL` and store in `models/rag-retriever-v1/` (FOUNDATION-2)
- Query API: expose ChromaDB search via Go API `/api/rag/query` (FOUNDATION-3)