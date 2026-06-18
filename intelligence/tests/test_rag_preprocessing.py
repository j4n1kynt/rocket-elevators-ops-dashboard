"""
Unit and integration tests for rag_preprocessing.py (AND-107 FOUNDATION-1).

Run unit tests only (fast, no model download):
  py -3 -m pytest intelligence/tests/test_rag_preprocessing.py -v

Run embedding smoke test (downloads model ~120MB on first run):
  py -3 -m pytest intelligence/tests/test_rag_preprocessing.py -v -m slow

Run full integration test against real PDFs + ChromaDB dry run:
  py -3 -m pytest intelligence/tests/test_rag_preprocessing.py -v -m integration
"""

import asyncio
import sys
from pathlib import Path

import numpy as np
import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent.parent))

from intelligence.rag_preprocessing import (
    MIN_CHUNK_CHARS,
    CHUNK_SIZE_TOKENS,
    OVERLAP_TOKENS,
    PDF_SOURCE_PATH,
    clean_text,
    chunk_documents,
    validate_chunks,
)


# ── Fixtures ───────────────────────────────────────────────────────────────────

_LONG_DOC = {
    "doc_name":   "test_doc",
    "raw_text":   ". ".join(["The elevator safety inspection was conducted"] * 120) + ".",
    "page_count": 3,
}


# ── clean_text ─────────────────────────────────────────────────────────────────

def test_clean_removes_null_bytes():
    cleaned, artifacts = clean_text("hello\x00world")
    assert "\x00" not in cleaned
    assert "null_bytes" in artifacts


def test_clean_collapses_whitespace():
    cleaned, _ = clean_text("hello   world\t\ttabs")
    assert "   " not in cleaned
    assert "\t\t" not in cleaned


def test_clean_passes_clean_text():
    text = "This is normal text.\nSecond paragraph."
    cleaned, artifacts = clean_text(text)
    assert cleaned == text.strip()
    assert artifacts == []


def test_clean_removes_control_chars():
    cleaned, artifacts = clean_text("text\x07bell\x1besc")
    assert "\x07" not in cleaned
    assert "\x1b" not in cleaned
    assert "control_chars" in artifacts


# ── chunk_documents ────────────────────────────────────────────────────────────

def test_chunk_produces_at_least_one_chunk():
    chunks, _ = chunk_documents([_LONG_DOC])
    assert len(chunks) > 0


def test_chunk_token_count_within_bounds():
    chunks, _ = chunk_documents([_LONG_DOC])
    # Allow 1.5x slack — last chunk and sentence overflows are expected
    for c in chunks:
        assert c["token_count"] <= CHUNK_SIZE_TOKENS * 1.5, (
            f"chunk {c['chunk_id']} too large: {c['token_count']} tokens"
        )


def test_chunk_overlap_shares_words_between_consecutive():
    chunks, _ = chunk_documents([_LONG_DOC])
    if len(chunks) < 2:
        pytest.skip("Not enough chunks to test overlap")
    c0_words = set(chunks[0]["text"].split())
    c1_words = set(chunks[1]["text"].split())
    assert c0_words & c1_words, "Expected word overlap between consecutive chunks"


def test_chunk_ids_are_unique():
    chunks, _ = chunk_documents([_LONG_DOC])
    ids = [c["chunk_id"] for c in chunks]
    assert len(ids) == len(set(ids)), "Duplicate chunk IDs found"


def test_chunk_id_format():
    chunks, _ = chunk_documents([_LONG_DOC])
    for c in chunks:
        assert c["chunk_id"].startswith("test_doc_chunk_"), f"Unexpected ID format: {c['chunk_id']}"


def test_chunk_min_text_length():
    chunks, _ = chunk_documents([_LONG_DOC])
    for c in chunks:
        assert len(c["text"]) >= MIN_CHUNK_CHARS, (
            f"Chunk {c['chunk_id']} below min length: {len(c['text'])} chars"
        )


# ── validate_chunks ────────────────────────────────────────────────────────────

def test_validate_passes_good_chunk():
    good = [{"chunk_id": "doc_chunk_000", "text": "x" * 200, "token_count": 50, "chunk_sequence": 0}]
    valid, skips = validate_chunks(good)
    assert len(valid) == 1
    assert skips == {}


def test_validate_rejects_missing_id():
    bad = [{"chunk_id": "", "text": "x" * 200, "token_count": 50, "chunk_sequence": 0}]
    valid, skips = validate_chunks(bad)
    assert len(valid) == 0


def test_validate_rejects_duplicate_ids():
    dups = [
        {"chunk_id": "same_id", "text": "x" * 200, "token_count": 50, "chunk_sequence": 0},
        {"chunk_id": "same_id", "text": "y" * 200, "token_count": 50, "chunk_sequence": 1},
    ]
    valid, skips = validate_chunks(dups)
    assert len(valid) == 1
    assert "same_id" in skips


def test_validate_rejects_short_text():
    bad = [{"chunk_id": "doc_chunk_000", "text": "hi", "token_count": 5, "chunk_sequence": 0}]
    valid, skips = validate_chunks(bad)
    assert len(valid) == 0


def test_validate_rejects_zero_tokens():
    bad = [{"chunk_id": "doc_chunk_000", "text": "x" * 200, "token_count": 0, "chunk_sequence": 0}]
    valid, skips = validate_chunks(bad)
    assert len(valid) == 0


# ── Embedding smoke test ───────────────────────────────────────────────────────

@pytest.mark.slow
def test_embedding_shape_and_no_nan():
    """Embed one sentence and check 1024-dim output with no NaN values."""
    from sentence_transformers import SentenceTransformer
    model = SentenceTransformer("BAAI/bge-large-en-v1.5")
    vec = model.encode(["elevator safety inspection"], show_progress_bar=False)[0]
    assert vec.shape == (1024,), f"Expected (1024,), got {vec.shape}"
    assert not np.isnan(vec).any(), "NaN values found in embedding"


# ── Integration test ───────────────────────────────────────────────────────────

@pytest.mark.integration
def test_full_pipeline_dry_run():
    """
    End-to-end dry run on real PDFs. Validates extract → chunk → embed
    without writing to ChromaDB. Requires PDFs at PDF_SOURCE_PATH.
    """
    from intelligence.rag_preprocessing import run_pipeline

    pdf_paths = sorted(Path(PDF_SOURCE_PATH).glob("*.pdf"))
    if not pdf_paths:
        pytest.skip(f"PDFs not found at {PDF_SOURCE_PATH}")

    if sys.platform == "win32":
        asyncio.set_event_loop_policy(asyncio.WindowsSelectorEventLoopPolicy())

    stats = asyncio.run(run_pipeline(pdf_paths, dry_run=True))

    assert stats["pdfs_processed"] == len(pdf_paths), "Not all PDFs processed"
    assert stats["chunks_created"] > 0, "No chunks created"
    assert stats["embeddings_ok"] == stats["chunks_created"], "Some embeddings failed"
