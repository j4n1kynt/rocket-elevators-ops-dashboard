"""
AND-107 FOUNDATION-1: RAG Document Preprocessing
Extracts, cleans, chunks, embeds, and stores 6 maintenance PDFs in ChromaDB.

Usage:
  py -3 intelligence/rag_preprocessing.py              # process all 6 PDFs (idempotent)
  py -3 intelligence/rag_preprocessing.py --limit 2    # test on first 2 PDFs
  py -3 intelligence/rag_preprocessing.py --force      # delete collection and reprocess
  py -3 intelligence/rag_preprocessing.py --dry-run    # validate pipeline, skip ChromaDB write
  py -3 intelligence/rag_preprocessing.py --pdf-path /path/to/pdfs  # override PDF directory
"""

import argparse
import asyncio
import re
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

import chromadb
import nltk
import numpy as np
import pdfplumber
import tiktoken
from sentence_transformers import SentenceTransformer

# ── Config ─────────────────────────────────────────────────────────────────────
PDF_SOURCE_PATH       = Path("C:/Users/juanjanica/Documents/Proyecto_CodeBoxx/rocket-elevators-ops-dashboard/LLM_Docs")
CHROMADB_PATH         = "data/chromadb"
COLLECTION_NAME       = "maintenance_documents"
EMBEDDING_MODEL       = "all-MiniLM-L6-v2"
# all-MiniLM-L6-v2 has a hard 256 WordPiece token limit; cl100k_base produces
# ~1.3–1.4x fewer tokens than WordPiece for the same text, so 180 tiktoken
# tokens stays safely under 256 WordPiece tokens after conversion.
CHUNK_SIZE_TOKENS     = 180
OVERLAP_TOKENS        = 20
EMBEDDING_CONCURRENCY = 8
BATCH_SIZE            = 32
MIN_CHUNK_CHARS       = 100
MIN_DOC_CHARS         = 500
MODEL_VERSION         = "all-MiniLM-L6-v2-v1"

# Download punkt sentence tokenizer on first run (~5MB, cached after)
try:
    nltk.data.find("tokenizers/punkt_tab")
except LookupError:
    ok = nltk.download("punkt_tab", quiet=False)
    if not ok:
        raise RuntimeError(
            "Failed to download NLTK punkt_tab tokenizer. "
            "Run: python -m nltk.downloader punkt_tab"
        )

_TOKENIZER = tiktoken.get_encoding("cl100k_base")


# ── 1. Extract ─────────────────────────────────────────────────────────────────

def extract_pdfs(pdf_paths: list) -> tuple:
    """Extract raw text from each PDF. Returns (docs, skip_reasons)."""
    docs = []
    skip_reasons = {}

    for path in pdf_paths:
        try:
            with pdfplumber.open(path) as pdf:
                pages = [page.extract_text() or "" for page in pdf.pages]
            text = "\n".join(pages)
        except Exception as exc:
            skip_reasons[path.stem] = f"extraction_error: {exc}"
            continue

        if len(text) < MIN_DOC_CHARS:
            skip_reasons[path.stem] = f"text_too_short: {len(text)} chars"
            continue

        docs.append({
            "doc_name":   path.stem,
            "path":       str(path),
            "raw_text":   text,
            "page_count": len(pages),
        })

    return docs, skip_reasons


# ── 2. Clean ───────────────────────────────────────────────────────────────────

def clean_text(text: str) -> tuple:
    """
    Normalize raw PDF text. Returns (cleaned_text, artifacts_removed).
    Removes null bytes (PostgreSQL pattern from etl_to_database.py),
    control chars, and excessive whitespace.
    """
    artifacts = []

    # Null bytes cause silent failures in downstream storage — strip them
    if "\x00" in text:
        artifacts.append("null_bytes")
        text = text.replace("\x00", "")

    cleaned = re.sub(r"[\x01-\x08\x0b\x0c\x0e-\x1f\x7f]", "", text)
    if cleaned != text:
        artifacts.append("control_chars")
    text = cleaned

    # Collapse runs of 3+ blank lines to 2 (preserve paragraph breaks)
    text = re.sub(r"\n{3,}", "\n\n", text)

    # Collapse runs of spaces/tabs to a single space
    text = re.sub(r"[ \t]{2,}", " ", text)

    text = "\n".join(line.rstrip() for line in text.splitlines())

    return text.strip(), artifacts


# ── 3. Chunk ───────────────────────────────────────────────────────────────────

def _count_tokens(text: str) -> int:
    return len(_TOKENIZER.encode(text))


def chunk_documents(docs: list) -> tuple:
    """
    Split each document into sentence-aware chunks of ~CHUNK_SIZE_TOKENS tokens
    with OVERLAP_TOKENS of trailing context carried into the next chunk.
    Sentence boundaries are preserved via nltk.sent_tokenize.
    Returns (chunks, skip_reasons).
    """
    chunks = []
    skip_reasons = {}

    for doc in docs:
        cleaned, artifacts = clean_text(doc["raw_text"])
        if artifacts:
            print(f"  {doc['doc_name']}: cleaned artifacts: {artifacts}")

        sentences = nltk.sent_tokenize(cleaned)
        doc_chunks = []
        current_sents = []
        current_tokens = 0
        seq = 0

        for sent in sentences:
            sent_tokens = _count_tokens(sent)

            # Flush buffer when this sentence would push us over the limit
            if current_tokens + sent_tokens > CHUNK_SIZE_TOKENS and current_sents:
                chunk_text = " ".join(current_sents)
                doc_chunks.append({
                    "chunk_id":       f"{doc['doc_name']}_chunk_{seq:03d}",
                    "doc_name":       doc["doc_name"],
                    "text":           chunk_text,
                    "token_count":    current_tokens,
                    "chunk_sequence": seq,
                })
                seq += 1

                # Carry last OVERLAP_TOKENS worth of sentences into next chunk.
                # If the last sentence alone exceeds the budget, include it anyway
                # so chunk boundaries always have at least one sentence of context.
                overlap_sents = []
                overlap_tokens = 0
                for s in reversed(current_sents):
                    s_tok = _count_tokens(s)
                    if overlap_tokens + s_tok > OVERLAP_TOKENS:
                        if not overlap_sents:
                            overlap_sents.insert(0, s)
                        break
                    overlap_sents.insert(0, s)
                    overlap_tokens += s_tok

                current_sents = overlap_sents + [sent]
                current_tokens = overlap_tokens + sent_tokens
            else:
                current_sents.append(sent)
                current_tokens += sent_tokens

        # Flush final buffer
        if current_sents:
            chunk_text = " ".join(current_sents)
            doc_chunks.append({
                "chunk_id":       f"{doc['doc_name']}_chunk_{seq:03d}",
                "doc_name":       doc["doc_name"],
                "text":           chunk_text,
                "token_count":    current_tokens,
                "chunk_sequence": seq,
            })

        valid = [c for c in doc_chunks if len(c["text"]) >= MIN_CHUNK_CHARS]
        invalid = len(doc_chunks) - len(valid)
        if invalid > 0:
            skip_reasons[doc["doc_name"]] = f"{invalid} chunks too short (<{MIN_CHUNK_CHARS} chars)"

        chunks.extend(valid)
        print(f"  {doc['doc_name']}: {len(valid)} chunks ({doc.get('page_count', '?')} pages)")

    return chunks, skip_reasons


# ── 4. Validate ────────────────────────────────────────────────────────────────

def validate_chunks(chunks: list) -> tuple:
    """
    Pre-storage validation: check text length, unique IDs, and token count.
    Returns (valid_chunks, skip_reasons).
    """
    valid = []
    skip_reasons = {}
    seen_ids = set()

    for c in chunks:
        cid   = c.get("chunk_id", "")
        text  = c.get("text", "")
        tokens = c.get("token_count", 0)

        if not cid:
            skip_reasons[f"seq_{c.get('chunk_sequence', '?')}"] = "missing chunk_id"
            continue
        if cid in seen_ids:
            skip_reasons[cid] = "duplicate chunk_id"
            continue
        if not text or len(text) < MIN_CHUNK_CHARS:
            skip_reasons[cid] = f"text too short: {len(text)} chars"
            continue
        if tokens <= 0:
            skip_reasons[cid] = "zero token count"
            continue

        seen_ids.add(cid)
        valid.append(c)

    return valid, skip_reasons


# ── 5. Embed (async) ───────────────────────────────────────────────────────────

async def generate_embeddings_async(chunks: list) -> tuple:
    """
    Generate 384-dim embeddings for all chunks using sentence-transformers.
    Batches are dispatched concurrently via asyncio.Semaphore(EMBEDDING_CONCURRENCY)
    and asyncio.to_thread — reuses the bounded concurrency pattern from
    generate_explanations.py (Semaphore + to_thread + gather(return_exceptions=True)).
    Returns (enriched_chunks, failed_count).
    """
    model = SentenceTransformer(EMBEDDING_MODEL)
    semaphore = asyncio.Semaphore(EMBEDDING_CONCURRENCY)

    def _encode_batch(texts: list) -> np.ndarray:
        return model.encode(
            texts,
            batch_size=len(texts),
            show_progress_bar=False,
            normalize_embeddings=False,
        )

    async def embed_batch(batch: list) -> list:
        async with semaphore:
            texts = [c["text"] for c in batch]
            vectors = await asyncio.to_thread(_encode_batch, texts)
            result = []
            for chunk, vector in zip(batch, vectors):
                if np.isnan(vector).any():
                    result.append({**chunk, "embedding": None, "skip_reason": "nan_embedding"})
                else:
                    result.append({**chunk, "embedding": vector.tolist()})
            return result

    batches = [chunks[i:i + BATCH_SIZE] for i in range(0, len(chunks), BATCH_SIZE)]
    print(f"  embedding {len(chunks)} chunks in {len(batches)} batch(es) of <={BATCH_SIZE}...")

    # return_exceptions=True prevents one failed batch from cancelling the rest
    results = await asyncio.gather(*[embed_batch(b) for b in batches], return_exceptions=True)

    enriched = []
    failed = 0
    for i, (batch, result) in enumerate(zip(batches, results)):
        if isinstance(result, Exception):
            print(f"  FAIL batch {i}: {type(result).__name__}: {result}")
            failed += len(batch)
            for c in batch:
                enriched.append({**c, "embedding": None, "skip_reason": f"batch_error: {type(result).__name__}"})
        else:
            for c in result:
                if c.get("embedding") is None:
                    failed += 1
                enriched.append(c)

    return enriched, failed


# ── 6. Store ───────────────────────────────────────────────────────────────────

def store_in_chromadb(chunks: list, collection) -> tuple:
    """
    Upsert valid (embedded) chunks into ChromaDB in batches of BATCH_SIZE.
    ChromaDB upsert is idempotent — re-runs with the same chunk_id overwrite
    rather than duplicate. Returns (inserted, skipped_no_embedding).
    """
    valid = [c for c in chunks if c.get("embedding") is not None]
    skipped = len(chunks) - len(valid)

    if not valid:
        return 0, skipped

    created_at = datetime.now(timezone.utc).isoformat()
    inserted = 0

    for batch_start in range(0, len(valid), BATCH_SIZE):
        batch = valid[batch_start:batch_start + BATCH_SIZE]
        collection.upsert(
            ids        = [c["chunk_id"] for c in batch],
            documents  = [c["text"] for c in batch],
            embeddings = [c["embedding"] for c in batch],
            metadatas  = [{
                "doc_name":       c["doc_name"],
                "chunk_sequence": c["chunk_sequence"],
                "token_count":    c["token_count"],
                "source_type":    "maintenance_pdf",
                "created_at":     created_at,
                "model_version":  MODEL_VERSION,
            } for c in batch],
        )
        inserted += len(batch)

    return inserted, skipped


# ── 7. Report ──────────────────────────────────────────────────────────────────

def generate_report(stats: dict) -> str:
    """Generate a markdown preprocessing report written to intelligence/rag_preprocessing_report.md."""
    lines = [
        "# RAG Preprocessing Report",
        f"**Run date:** {stats['run_date']}",
        f"**Duration:** {stats.get('duration_s', 0):.1f}s",
        "",
        "## Summary",
        "| Metric | Value |",
        "|--------|-------|",
        f"| PDFs processed | {stats['pdfs_processed']}/{stats['pdfs_total']} |",
        f"| Total raw text | {stats['total_raw_chars']:,} chars |",
        f"| Chunks created | {stats['chunks_created']} |",
        f"| Embeddings OK  | {stats['embeddings_ok']}/{stats['chunks_created']} |",
        f"| ChromaDB count | {stats['chromadb_count']} |",
        f"| Duration       | {stats.get('duration_s', 0):.1f}s |",
        "",
    ]

    if stats.get("extract_skips"):
        lines += ["## Extraction Failures", ""]
        for doc, reason in stats["extract_skips"].items():
            lines.append(f"- `{doc}`: {reason}")
        lines.append("")

    if stats.get("chunk_skips"):
        lines += ["## Chunk Warnings", ""]
        for doc, reason in stats["chunk_skips"].items():
            lines.append(f"- `{doc}`: {reason}")
        lines.append("")

    if stats.get("validate_skips"):
        lines += ["## Validation Failures", ""]
        for cid, reason in stats["validate_skips"].items():
            lines.append(f"- `{cid}`: {reason}")
        lines.append("")

    lines += [
        "## Extension Points",
        "- Incident narratives: add `chunk_incident_narratives()` with `source_type=incident` (FOUNDATION-1b)",
        "- Fine-tuned retriever: swap `EMBEDDING_MODEL` and store in `models/rag-retriever-v1/` (FOUNDATION-2)",
        "- Query API: expose ChromaDB search via Go API `/api/rag/query` (FOUNDATION-3)",
    ]

    return "\n".join(lines)


# ── Pipeline orchestrator ──────────────────────────────────────────────────────

async def run_pipeline(pdf_paths: list, force: bool = False, dry_run: bool = False) -> dict:
    t_start = time.time()
    stats = {
        "run_date":        datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC"),
        "pdfs_total":      len(pdf_paths),
        "pdfs_processed":  0,
        "total_raw_chars": 0,
        "chunks_created":  0,
        "embeddings_ok":   0,
        "chromadb_count":  0,
    }

    # [1/5] Extract
    print(f"\n[1/5] Extracting text from {len(pdf_paths)} PDFs...")
    docs, extract_skips = extract_pdfs(pdf_paths)
    stats["pdfs_processed"]  = len(docs)
    stats["extract_skips"]   = extract_skips
    stats["total_raw_chars"] = sum(len(d["raw_text"]) for d in docs)
    for doc, reason in extract_skips.items():
        print(f"  SKIP {doc}: {reason}")
    print(f"  extracted: {len(docs)}/{len(pdf_paths)} PDFs  ({stats['total_raw_chars']:,} chars total)")

    if not docs:
        print("  No documents extracted — aborting.")
        stats["duration_s"] = time.time() - t_start
        return stats

    # [2/5] Chunk + validate
    print(f"\n[2/5] Chunking ({CHUNK_SIZE_TOKENS}-token target, {OVERLAP_TOKENS}-token overlap)...")
    chunks, chunk_skips = chunk_documents(docs)
    valid_chunks, validate_skips = validate_chunks(chunks)
    stats["chunks_created"]  = len(valid_chunks)
    stats["chunk_skips"]     = chunk_skips
    stats["validate_skips"]  = validate_skips
    print(f"  valid chunks: {len(valid_chunks)}  (dropped: {len(chunks) - len(valid_chunks)})")

    if not valid_chunks:
        print("  No valid chunks — aborting.")
        stats["duration_s"] = time.time() - t_start
        return stats

    # [3/5] Embed
    print(f"\n[3/5] Generating embeddings ({EMBEDDING_MODEL})...")
    enriched, embed_failed = await generate_embeddings_async(valid_chunks)
    stats["embeddings_ok"] = len(enriched) - embed_failed
    print(f"  embeddings: {stats['embeddings_ok']}/{len(enriched)} OK  ({embed_failed} failed)")

    if dry_run:
        print("\n  [DRY RUN] Skipping ChromaDB write.")
        stats["duration_s"] = time.time() - t_start
        return stats

    # [4/5] Store
    print(f"\n[4/5] Storing in ChromaDB (collection: {COLLECTION_NAME})...")
    client = chromadb.PersistentClient(path=CHROMADB_PATH)

    if force:
        try:
            client.delete_collection(COLLECTION_NAME)
            print("  --force: existing collection deleted")
        except Exception:
            pass

    collection = client.get_or_create_collection(
        name=COLLECTION_NAME,
        metadata={"hnsw:space": "cosine"},
    )

    before_count = collection.count()
    inserted, skipped_no_embed = store_in_chromadb(enriched, collection)
    after_count = collection.count()
    stats["chromadb_count"] = after_count

    net_new = after_count - before_count
    print(f"  upserted: {inserted}  |  net new: {net_new}  |  skipped (no embedding): {skipped_no_embed}")
    print(f"  collection total: {after_count}")

    # [5/5] Report
    stats["duration_s"] = time.time() - t_start
    report = generate_report(stats)
    report_path = Path("intelligence/rag_preprocessing_report.md")
    report_path.write_text(report, encoding="utf-8")
    print(f"\n[5/5] Report saved -> {report_path}")

    return stats


# ── Entry point ────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="AND-107 FOUNDATION-1: RAG document preprocessing")
    parser.add_argument("--limit",    type=int, default=None, help="Process only first N PDFs (testing)")
    parser.add_argument("--force",    action="store_true",    help="Delete existing collection and reprocess")
    parser.add_argument("--dry-run",  action="store_true",    help="Run pipeline without writing to ChromaDB")
    parser.add_argument("--pdf-path", type=str, default=None, help="Override PDF source directory")
    args = parser.parse_args()

    pdf_dir = Path(args.pdf_path) if args.pdf_path else PDF_SOURCE_PATH
    if not pdf_dir.exists():
        print(f"ERROR: PDF directory not found: {pdf_dir}")
        sys.exit(1)

    pdf_paths = sorted(pdf_dir.glob("*.pdf"))
    if args.limit:
        pdf_paths = pdf_paths[: args.limit]

    if not pdf_paths:
        print(f"ERROR: No PDFs found in {pdf_dir}")
        sys.exit(1)

    # Windows asyncio fix — reuse pattern from generate_explanations.py
    if sys.platform == "win32":
        asyncio.set_event_loop_policy(asyncio.WindowsSelectorEventLoopPolicy())

    print("=== AND-107 FOUNDATION-1: RAG Document Preprocessing ===")
    print(f"PDFs:        {len(pdf_paths)} ({', '.join(p.stem for p in pdf_paths)})")
    print(f"Model:       {EMBEDDING_MODEL}")
    print(f"Chunk size:  {CHUNK_SIZE_TOKENS} tokens (overlap: {OVERLAP_TOKENS})")
    print(f"ChromaDB:    {CHROMADB_PATH}/{COLLECTION_NAME}")
    print(f"Mode:        {'DRY RUN' if args.dry_run else 'FORCE' if args.force else 'RESUME (idempotent)'}")

    stats = asyncio.run(run_pipeline(pdf_paths, force=args.force, dry_run=args.dry_run))

    print("\n=== Complete ===")
    print(f"PDFs processed:   {stats['pdfs_processed']}/{stats['pdfs_total']}")
    print(f"Chunks created:   {stats['chunks_created']}")
    print(f"Embeddings OK:    {stats['embeddings_ok']}/{stats['chunks_created']}")
    print(f"ChromaDB count:   {stats['chromadb_count']}")
    print(f"Duration:         {stats.get('duration_s', 0):.1f}s")


if __name__ == "__main__":
    main()
