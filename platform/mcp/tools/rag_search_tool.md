# RAG Search Tool — FOUNDATION-3

## Overview

The RAG Search Tool is an MCP tool that performs semantic search over ChromaDB-indexed maintenance documents. It enables the OpsBot chatbot to retrieve relevant maintenance procedures in response to natural-language queries from the Go API.

---

## Architecture

```
Go API → MCP Server → search_maintenance_docs() → rag_query() → ChromaDB
```

| Component | File | Responsibility |
|---|---|---|
| MCP tool | `platform/mcp/tools/rag_tools.py` | Input validation, filtering, response shaping |
| ChromaDB query helper | `platform/mcp/rag.py` | Embedding generation, ChromaDB query execution |
| Preprocessing pipeline | `intelligence/rag_preprocessing.py` | Chunking, embedding, and indexing documents into ChromaDB |

---

## ChromaDB Setup

- **Collection:** `maintenance_documents`
- **Distance metric:** cosine (`hnsw:space: cosine`)
- **Embedding model:** `BAAI/bge-large-en-v1.5` (1024-dim) — used by both preprocessing and query
- **Chunk size:** 500 tokens with 50-token overlap
- **Metadata written per chunk:** `doc_name`, `chunk_sequence`, `token_count`, `source_type`, `created_at`, `model_version`

---

## Tool: `search_maintenance_docs`

**Signature:**
```python
def search_maintenance_docs(query: str, n_results: int = 5) -> dict
```

**Input validation (Pydantic):**
- `query`: non-empty string, max 500 characters
- `n_results`: integer in [1, 20], defaults to 5

### Processing pipeline

1. Validate inputs via `SearchMaintenanceDocsInput`
2. Call `rag_query()` — embeds the query and retrieves raw chunks from ChromaDB
3. If ChromaDB returns zero results → return no-results message
4. Convert each result's cosine distance to similarity score: `similarity = 1 - distance`
5. Filter out results below `SIMILARITY_THRESHOLD = 0.5`
6. If all results are filtered out → return low-confidence message
7. Return confident results with source attribution fields

### Response shape

All three branches include the same top-level keys:

**Successful results:**
```json
{
  "query": "How do I replace a hydraulic pump?",
  "message": null,
  "total_returned": 3,
  "results": [
    {
      "text": "...",
      "source_name": "Maintenance Document hydraulic_maintenance_guide",
      "similarity_score": 0.87
    }
  ]
}
```

**No confident matches (all results below threshold):**
```json
{
  "query": "What colour is hydraulic fluid?",
  "message": "No confident matches found",
  "total_returned": 0,
  "results": []
}
```

**No results (ChromaDB returned nothing):**
```json
{
  "query": "What is the capital of France?",
  "message": "No relevant documentation found",
  "total_returned": 0,
  "results": []
}
```

**Error (exception during execution):**
```json
{
  "error": true,
  "message": "ChromaDB collection 'maintenance_documents' not found: ..."
}
```

---

## Similarity Threshold

| Constant | Value | Location |
|---|---|---|
| `SIMILARITY_THRESHOLD` | `0.5` | Module level in `rag_tools.py` |

**Conversion formula:** ChromaDB returns cosine distance = `1 − cosine_similarity`. Inverting it gives `similarity = 1 - distance`. Distance range is [0, 2] theoretically; in practice [0, 1] for text embeddings.

**Guard:** `max(0.0, round(1 - distance, 4))` clamps the result to avoid negative similarity from floating-point noise on unnormalized embeddings.

---

## Source Attribution

Each result exposes three fields:

| Field | Source | Notes |
|---|---|---|
| `text` | ChromaDB `documents` | The raw chunk text |
| `source_name` | ChromaDB `metadata["doc_name"]` | Prefixed with `"Maintenance Document "` for readability |
| `similarity_score` | Computed from `distance` | Float in [0, 1], higher is more similar |

Internal pipeline fields (`distance`, `metadata`, `token_count`, `source_type`, `created_at`, `model_version`) are dropped from the response.

Safe fallback: if a chunk has no metadata (e.g. upserted without it), `source_name` defaults to `"Maintenance Document unknown"`.

---

## Error Handling

`search_maintenance_docs` wraps its entire body in `try/except`, matching the pattern used by `search_incident_narratives`:

```python
except ValidationError:
    raise                          # re-raised so callers and tests can catch it
except Exception as exc:
    return {"error": True, "message": str(exc)}
```

`ValidationError` must be explicitly re-raised before the broad `except Exception` — otherwise invalid inputs silently return an error dict instead of raising, breaking input validation behaviour.

---

## Tests

### Integration tests — `platform/mcp/tools/test_rag_search.py`

`rag_query` is mocked in all tests. No live ChromaDB required.

| Test | Mock returns | Assertion |
|---|---|---|
| `test_relevant_query_returns_confident_results` (3 queries) | `distance: 0.2` (similarity 0.8) | `total_returned >= 1`, `similarity_score >= 0.5`, attribution fields present |
| `test_marginal_query_returns_no_confident_matches` (3 queries) | `distance: 0.8` (similarity 0.2) | `message == "No confident matches found"`, `total_returned == 0` |
| `test_out_of_scope_query_returns_no_documentation` (3 queries) | `[]` | `message == "No relevant documentation found"`, `total_returned == 0` |

### Validation tests — `platform/mcp/tools/test_validation.py`

Covers invalid inputs (empty query, whitespace, too long, `n_results` out of range) and confirms `rag_query` is never called when validation fails.

---

## Decisions Made

**Filtering at the tool layer, not in `rag_query`**
The similarity threshold is applied inside `search_maintenance_docs`, not inside `rag_query`. `rag_query` is a general-purpose helper — threshold policy is specific to this tool and may differ for future tools.

**`section` field omitted**
The task description mentioned a `section` field for source attribution, but `rag_preprocessing.py` never writes a `section` key into ChromaDB metadata. The acceptance criteria does not require it either, so it was dropped. `chunk_sequence` exists in metadata but was not surfaced — it is an internal pipeline field, not meaningful to a chatbot consumer.

**Mocking `rag_query` in tests instead of hitting live ChromaDB**
Tests mock `rag_query` to control distance values directly. This makes the test suite fast, deterministic, and CI-safe without a populated ChromaDB. What is being tested is the filtering and messaging logic in `search_maintenance_docs`, not ChromaDB itself.

**Consistent `message` key across all response branches**
All three response branches include `"message"` at the same key. The success branch uses `"message": null`. This prevents `KeyError` on the Go API or chatbot side when reading the response.

**`SIMILARITY_THRESHOLD` as a module-level constant**
Initially defined inside the function body on every call, it was moved to module level for visibility and to make it easy to adjust without hunting inside the function.

---

## Known Issues

None at this time.

---

## Errors Fixed During Development

| Error | Root cause | Fix |
|---|---|---|
| `try:` block with no `except` — `SyntaxError` on import | Merge conflict resolution from `dev` left the `try:` wrapper without a closing `except` | Wrapped entire function body in `try/except`, matching `search_incident_narratives` pattern |
| `ValidationError` swallowed — `test_validation.py` 5 failures | Broad `except Exception` caught `pydantic.ValidationError` before it could propagate | Added explicit `except ValidationError: raise` before the broad handler; imported `ValidationError` from pydantic |
| `result["text"]` and `result["distance"]` with hard `[]` access | If ChromaDB returns a chunk without these keys, a `KeyError` would raise | Switched to `result.get("text", "")` and `result.get("distance", 1.0)` — distance defaults to 1.0 so similarity computes to 0.0 and gets filtered out |
| Embedding model mismatch — dimension error or meaningless similarities | `rag.py` used `all-MiniLM-L6-v2` (384-dim) while `rag_preprocessing.py` indexed with `BAAI/bge-large-en-v1.5` (1024-dim) | Updated `EMBEDDING_MODEL` in `rag.py` to `BAAI/bge-large-en-v1.5` — both files now use the same model and vector space |
| `metadata` access raising `TypeError` on `None` | ChromaDB returns `None` for chunks upserted without metadata | Added `(result.get("metadata") or {})` guard before calling `.get("doc_name")` |
