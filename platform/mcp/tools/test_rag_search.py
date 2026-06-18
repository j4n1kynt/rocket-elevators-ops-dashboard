"""
Integration tests for search_maintenance_docs — covers filtering logic,
messaging, and source attribution across three query categories.

rag_query is mocked so tests run without a live ChromaDB instance.

Run: pytest platform/mcp/tools/test_rag_search.py -v
"""

import pytest
from unittest.mock import patch

from platform.mcp.tools.rag_tools import search_maintenance_docs

# Shared mock result templates
_RELEVANT_RESULT = {
    "text": "Inspect the hydraulic pump for wear and replace the piston seal if leaking.",
    "metadata": {"doc_name": "hydraulic_maintenance_guide"},
    "distance": 0.2,  # similarity 0.8 — above threshold
}

_MARGINAL_RESULT = {
    "text": "Maintenance visits are generally scheduled on a quarterly basis.",
    "metadata": {"doc_name": "general_maintenance_schedule"},
    "distance": 0.8,  # similarity 0.2 — below threshold
}


# ── Relevant queries ──────────────────────────────────────────────────────────

@pytest.mark.parametrize("query", [
    "How do I replace a hydraulic pump?",
    "What are the maintenance procedures for elevator cables?",
    "How to inspect the safety brake system?",
])
def test_relevant_query_returns_confident_results(query):
    with patch("platform.mcp.tools.rag_tools.rag_query", return_value=[_RELEVANT_RESULT]):
        result = search_maintenance_docs(query=query)

    assert result["total_returned"] >= 1
    assert result.get("message") is None
    for r in result["results"]:
        assert r["similarity_score"] >= 0.5
        assert "text" in r
        assert "source_name" in r
        assert r["source_name"].startswith("Maintenance Document")


# ── Marginal queries ──────────────────────────────────────────────────────────

@pytest.mark.parametrize("query", [
    "What is the colour of hydraulic fluid?",
    "How long does a typical maintenance visit take?",
    "Are there any winter-specific maintenance requirements?",
])
def test_marginal_query_returns_no_confident_matches(query):
    with patch("platform.mcp.tools.rag_tools.rag_query", return_value=[_MARGINAL_RESULT]):
        result = search_maintenance_docs(query=query)

    assert result["message"] == "No confident matches found"
    assert result["total_returned"] == 0
    assert result["results"] == []


# ── Out-of-scope queries ──────────────────────────────────────────────────────

@pytest.mark.parametrize("query", [
    "What is the capital of France?",
    "How do I bake a chocolate cake?",
    "What is the stock price of ACME Corp?",
])
def test_out_of_scope_query_returns_no_documentation(query):
    with patch("platform.mcp.tools.rag_tools.rag_query", return_value=[]):
        result = search_maintenance_docs(query=query)

    assert result["message"] == "No relevant documentation found"
    assert result["total_returned"] == 0
    assert result["results"] == []
