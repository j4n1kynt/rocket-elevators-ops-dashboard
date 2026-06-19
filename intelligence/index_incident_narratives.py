#!/usr/bin/env python3
"""
AND-107 FOUNDATION-1b: Index incident narratives into ChromaDB.

Reads incident narratives from PostgreSQL, embeds each one, and upserts it into a
dedicated ChromaDB collection ("incident_narratives", separate from the
maintenance_documents collection). Semantic embeddings let the
search_incident_narratives MCP tool retrieve incidents even when the query
phrasing does not match the narrative wording — something PostgreSQL FTS could
not do.

One vector per incident (narratives are short and atomic — no chunking).
ChromaDB id = str(incident_id) so re-runs upsert rather than duplicate (idempotent).

Usage:
  py -3 intelligence/index_incident_narratives.py            # index all narratives
  py -3 intelligence/index_incident_narratives.py --force    # delete collection and rebuild
  py -3 intelligence/index_incident_narratives.py --dry-run  # embed but skip ChromaDB write

Environment variables:
  DATABASE_URL / DB_*   PostgreSQL connection (same pattern as etl_to_database.py)
  RAG_CHROMADB_PATH     ChromaDB persistent store path (default: data/chromadb)

Constants below MUST match platform/mcp/rag.py and intelligence/rag_preprocessing.py.
"""

import argparse
import os
import sys
import time
from datetime import datetime, timezone

import psycopg2
from dotenv import load_dotenv

load_dotenv()

# ── Config (pinned contract — MUST match platform/mcp/rag.py) ───────────────────
CHROMADB_PATH   = os.environ.get("RAG_CHROMADB_PATH", "data/chromadb")
COLLECTION_NAME = "incident_narratives"
EMBEDDING_MODEL = "BAAI/bge-small-en-v1.5"
MODEL_VERSION   = "bge-small-en-v1.5-v1"
BATCH_SIZE      = 64

_NARRATIVE_QUERY = """
    SELECT incident_id, elevator_id, date_of_occurrence, category,
           incident_summary, narrative
    FROM incidents
    WHERE narrative IS NOT NULL
    ORDER BY incident_id
"""


# ── Connection (same pattern as etl_to_database.py) ─────────────────────────────

def get_connection():
    url = os.environ.get("DATABASE_URL")
    if url:
        return psycopg2.connect(url)
    return psycopg2.connect(
        host=os.environ["DB_HOST"],
        port=os.environ.get("DB_PORT", "5432"),
        user=os.environ["DB_USER"],
        password=os.environ["DB_PASSWORD"],
        dbname=os.environ["DB_NAME"],
    )


# ── Metadata builder (pure — unit tested) ───────────────────────────────────────

def build_metadata(row: dict) -> dict:
    """
    Build the per-vector ChromaDB metadata for one incident row.

    ChromaDB rejects None metadata values, so missing fields are coerced to "".
    date_of_occurrence is stored as a string per the pinned contract.
    """
    date_val = row.get("date_of_occurrence")
    date_str = date_val.isoformat() if hasattr(date_val, "isoformat") else (
        str(date_val) if date_val is not None else ""
    )

    def _str_or_empty(v):
        return "" if v is None else v

    return {
        "incident_id":        row["incident_id"],
        "elevator_id":        _str_or_empty(row.get("elevator_id")),
        "date_of_occurrence": date_str,
        "category":           _str_or_empty(row.get("category")),
        "incident_summary":   _str_or_empty(row.get("incident_summary")),
        "source_type":        "incident",
        "model_version":      MODEL_VERSION,
    }


# ── Pipeline ────────────────────────────────────────────────────────────────────

def fetch_narratives() -> list:
    """Read all incidents with a non-null narrative from PostgreSQL."""
    conn = get_connection()
    try:
        with conn.cursor() as cur:
            cur.execute(_NARRATIVE_QUERY)
            cols = [c[0] for c in cur.description]
            return [dict(zip(cols, r)) for r in cur.fetchall()]
    finally:
        conn.close()


def embed_narratives(rows: list, model) -> list:
    """Embed each narrative. Returns rows enriched with an 'embedding' key."""
    texts = [r["narrative"] for r in rows]
    vectors = model.encode(
        texts,
        batch_size=BATCH_SIZE,
        show_progress_bar=False,
        normalize_embeddings=False,
    )
    return [{**row, "embedding": vec.tolist()} for row, vec in zip(rows, vectors)]


def store_in_chromadb(enriched: list, collection) -> int:
    """Upsert embedded narratives into ChromaDB in batches. Idempotent."""
    inserted = 0
    for start in range(0, len(enriched), BATCH_SIZE):
        batch = enriched[start:start + BATCH_SIZE]
        collection.upsert(
            ids        = [str(r["incident_id"]) for r in batch],
            documents  = [r["narrative"] for r in batch],
            embeddings = [r["embedding"] for r in batch],
            metadatas  = [build_metadata(r) for r in batch],
        )
        inserted += len(batch)
    return inserted


def run(force: bool = False, dry_run: bool = False) -> dict:
    t_start = time.time()
    stats = {
        "run_date":       datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC"),
        "rows_read":      0,
        "embedded":       0,
        "chromadb_count": 0,
    }

    print("\n[1/3] Reading incident narratives from PostgreSQL...")
    rows = fetch_narratives()
    stats["rows_read"] = len(rows)
    print(f"  read: {len(rows)} narratives")

    if not rows:
        print("  No narratives to index — aborting.")
        stats["duration_s"] = time.time() - t_start
        return stats

    print(f"\n[2/3] Embedding ({EMBEDDING_MODEL})...")
    from sentence_transformers import SentenceTransformer  # lazy — heavy import
    model = SentenceTransformer(EMBEDDING_MODEL)
    enriched = embed_narratives(rows, model)
    stats["embedded"] = len(enriched)
    print(f"  embedded: {len(enriched)} narratives")

    if dry_run:
        print("\n  [DRY RUN] Skipping ChromaDB write.")
        stats["duration_s"] = time.time() - t_start
        return stats

    print(f"\n[3/3] Storing in ChromaDB (collection: {COLLECTION_NAME})...")
    import chromadb  # lazy — heavy import
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

    before = collection.count()
    inserted = store_in_chromadb(enriched, collection)
    after = collection.count()
    stats["chromadb_count"] = after
    print(f"  upserted: {inserted}  |  net new: {after - before}  |  collection total: {after}")

    stats["duration_s"] = time.time() - t_start
    return stats


# ── Entry point ─────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="AND-107 FOUNDATION-1b: index incident narratives into ChromaDB"
    )
    parser.add_argument("--force",   action="store_true", help="Delete existing collection and rebuild")
    parser.add_argument("--dry-run", action="store_true", help="Embed without writing to ChromaDB")
    args = parser.parse_args()

    print("=== AND-107 FOUNDATION-1b: Incident Narrative Indexing ===")
    print(f"Model:      {EMBEDDING_MODEL}")
    print(f"ChromaDB:   {CHROMADB_PATH}/{COLLECTION_NAME}")
    print(f"Mode:       {'DRY RUN' if args.dry_run else 'FORCE' if args.force else 'UPSERT (idempotent)'}")

    try:
        stats = run(force=args.force, dry_run=args.dry_run)
    except Exception as exc:
        print(f"ERROR: {exc}")
        sys.exit(1)

    print("\n=== Complete ===")
    print(f"Rows read:       {stats['rows_read']}")
    print(f"Embedded:        {stats['embedded']}")
    print(f"ChromaDB count:  {stats['chromadb_count']}")
    print(f"Duration:        {stats.get('duration_s', 0):.1f}s")


if __name__ == "__main__":
    main()
