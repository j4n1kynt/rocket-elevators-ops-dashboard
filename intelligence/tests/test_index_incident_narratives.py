"""
Unit tests for index_incident_narratives.py (AND-107 FOUNDATION-1b).

Fast tests only — exercise the pure metadata-builder helper without touching
PostgreSQL, the embedding model, or ChromaDB.

Run:
  py -3 -m pytest intelligence/tests/test_index_incident_narratives.py -v
"""

import sys
from datetime import date
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent.parent))

from intelligence.index_incident_narratives import (
    COLLECTION_NAME,
    EMBEDDING_MODEL,
    MODEL_VERSION,
    build_metadata,
)


def test_pinned_contract_constants():
    # These MUST match platform/mcp/rag.py and the Go side.
    assert COLLECTION_NAME == "incident_narratives"
    assert EMBEDDING_MODEL == "BAAI/bge-small-en-v1.5"
    assert MODEL_VERSION == "bge-small-en-v1.5-v1"


def test_build_metadata_shape_and_types():
    row = {
        "incident_id": 4821,
        "elevator_id": 102,
        "date_of_occurrence": date(2025, 3, 14),
        "category": "Entrapment",
        "incident_summary": "Passenger trapped between floors",
        "narrative": "The car stalled between floors.",
    }

    meta = build_metadata(row)

    assert meta["incident_id"] == 4821
    assert meta["elevator_id"] == 102
    # date stored as a string per the pinned contract
    assert meta["date_of_occurrence"] == "2025-03-14"
    assert isinstance(meta["date_of_occurrence"], str)
    assert meta["category"] == "Entrapment"
    assert meta["incident_summary"] == "Passenger trapped between floors"
    assert meta["source_type"] == "incident"
    assert meta["model_version"] == MODEL_VERSION
    # narrative is the document text, not metadata
    assert "narrative" not in meta


def test_build_metadata_handles_null_date():
    row = {
        "incident_id": 7,
        "elevator_id": None,
        "date_of_occurrence": None,
        "category": None,
        "incident_summary": None,
        "narrative": "Some text.",
    }

    meta = build_metadata(row)

    # ChromaDB rejects None metadata values — coerce to empty string.
    assert meta["date_of_occurrence"] == ""
    assert meta["elevator_id"] == ""
    assert meta["category"] == ""
    assert meta["incident_summary"] == ""
    assert meta["incident_id"] == 7
    assert meta["source_type"] == "incident"
    assert meta["model_version"] == MODEL_VERSION
