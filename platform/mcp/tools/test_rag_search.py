"""
Integration tests for search_maintenance_docs and search_incident_narratives —
covers filtering logic, messaging, and source attribution across query categories.

rag_query is mocked so tests run without a live ChromaDB instance.

Run: pytest platform/mcp/tools/test_rag_search.py -v
"""

import pytest
from unittest.mock import patch

from platform.mcp.tools.rag_tools import (
    INCIDENT_SIMILARITY_THRESHOLD,
    search_incident_narratives,
    search_maintenance_docs,
)

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


# ── Incident narrative search (semantic via ChromaDB) ───────────────────────────

_INCIDENT_RELEVANT = {
    "text": "The car stalled between the third and fourth floors and the doors would not open.",
    "metadata": {
        "incident_id": 4821,
        "elevator_id": 102,
        "date_of_occurrence": "2025-03-14",
        "category": "Entrapment",
        "incident_summary": "Passenger trapped between floors",
        "source_type": "incident",
        "model_version": "bge-small-en-v1.5-v1",
    },
    "distance": 0.18,  # similarity 0.82 — above threshold
}

_INCIDENT_MARGINAL = {
    "text": "Routine annual inspection completed with no findings.",
    "metadata": {
        "incident_id": 990,
        "elevator_id": 55,
        "date_of_occurrence": "2024-11-01",
        "category": "Inspection",
        "incident_summary": "Annual inspection",
        "source_type": "incident",
        "model_version": "bge-small-en-v1.5-v1",
    },
    "distance": 0.8,  # similarity 0.2 — below threshold
}


def test_incident_relevant_query_returns_confident_results():
    with patch(
        "platform.mcp.tools.rag_tools.rag_query", return_value=[_INCIDENT_RELEVANT]
    ) as mock_query:
        result = search_incident_narratives(query="people stuck inside the lift")

    # Must query the dedicated incident collection, not maintenance.
    _, kwargs = mock_query.call_args
    assert kwargs["collection_name"] == "incident_narratives"

    assert result.get("message") is None
    assert result["total_returned"] == 1
    r = result["results"][0]
    assert r["incident_id"] == 4821
    assert r["elevator_id"] == 102
    assert r["date_of_occurrence"] == "2025-03-14"
    assert r["category"] == "Entrapment"
    assert r["incident_summary"] == "Passenger trapped between floors"
    assert r["narrative"].startswith("The car stalled")
    assert r["source_name"] == "Incident #4821 (2025-03-14)"
    assert r["similarity_score"] >= INCIDENT_SIMILARITY_THRESHOLD


# A score of ~0.57 is what irrelevant queries floor at on the real index
# (bge-small unnormalized). It sits ABOVE the maintenance 0.5 cut but must be
# rejected by the stricter incident threshold (0.65). This pins that decision.
_INCIDENT_JUNK_FLOOR = {
    "text": "Routine valve adjustment, no incident.",
    "metadata": {
        "incident_id": 12,
        "elevator_id": 7,
        "date_of_occurrence": "2024-01-01",
        "category": "Maintenance",
        "incident_summary": "Valve adjustment",
        "source_type": "incident",
        "model_version": "bge-small-en-v1.5-v1",
    },
    "distance": 0.43,  # similarity 0.57 — above 0.5 but below the 0.65 incident cut
}


def test_incident_junk_floor_score_filtered_out():
    with patch(
        "platform.mcp.tools.rag_tools.rag_query", return_value=[_INCIDENT_JUNK_FLOOR]
    ):
        result = search_incident_narratives(query="best pasta recipe for dinner")

    assert result["message"] == "No relevant incidents found"
    assert result["total_returned"] == 0
    assert result["results"] == []


def test_incident_marginal_query_filtered_out():
    with patch(
        "platform.mcp.tools.rag_tools.rag_query", return_value=[_INCIDENT_MARGINAL]
    ):
        result = search_incident_narratives(query="something only loosely related")

    assert result["message"] == "No relevant incidents found"
    assert result["total_returned"] == 0
    assert result["results"] == []


def test_incident_no_results_returns_message():
    with patch("platform.mcp.tools.rag_tools.rag_query", return_value=[]):
        result = search_incident_narratives(query="nonexistent topic")

    assert result["message"] == "No relevant incidents found"
    assert result["total_returned"] == 0
    assert result["results"] == []
