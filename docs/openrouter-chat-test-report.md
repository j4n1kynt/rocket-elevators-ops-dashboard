# OpenRouter Chat — Test Report

**Date:** 2026-06-18
**Scope:** End-to-end smoke test of the chat after migrating the LLM provider from Ollama to OpenRouter.

---

## TL;DR

- The chat pipeline works end-to-end: **UI → Flask (`server.py`, :5000) → Go API (:8080) → MCP server (:8765) → PostgreSQL (cloud) / ChromaDB**.
- The configured free model `qwen/qwen3-next-80b-a3b-instruct:free` was **unusable** (HTTP 429, upstream provider saturated). Switched to **`google/gemma-4-31b-it:free`**, which works.
- Data queries, incident queries, and RAG (PDF) queries all returned **grounded** answers with the required attribution: *"According to the live fleet database..."*.

---

## Why gemma and not qwen

The original model `qwen/qwen3-next-80b-a3b-instruct:free` returned **HTTP 429** on the very first request — even on a brand-new API key (`usage_daily: 0`, "Last Used: Never").

The raw 429 body explained the real cause (this is **not** an account quota issue):

```
"qwen/qwen3-next-80b-a3b-instruct:free is temporarily rate-limited upstream"
provider_name: "Venice"   retry_after_seconds: 27
```

The `:free` model is hosted by a third-party upstream provider (**Venice**) and shared across all OpenRouter users. When that shared pool saturates, it returns 429 to everyone, regardless of your own quota. There are **two independent limits** that stack:

1. **Account free-tier limit** — ~50 requests/day (account had 0 credits purchased), ~20 requests/minute.
2. **Upstream provider saturation** — the 429 we hit. Independent of #1.

### Empirical model probe (2026-06-18)

We probed candidate free models directly against `https://openrouter.ai/api/v1/chat/completions`:

| Model | Status | Latency | Notes |
|-------|--------|---------|-------|
| `qwen/qwen3-next-80b-a3b-instruct:free` | 429 | — | Saturated (Venice) |
| `meta-llama/llama-3.3-70b-instruct:free` | 429 | — | Also saturated |
| **`google/gemma-4-31b-it:free`** | **200** | **2.1s** | ✅ Clean, follows instructions |
| `nvidia/nemotron-3-super-120b-a12b:free` | 200 | 14.9s | Slow + verbose |
| `openai/gpt-oss-120b:free` | 200 | 1.8s | OK (backup) |
| `openai/gpt-oss-20b:free` | 200 | 1.2s | Returned `content: null` (reasoning-channel risk) |

**Decision:** `google/gemma-4-31b-it:free` — on a different upstream provider (Google) than the saturated Venice pool, fast (~2s), and it follows the system prompt (key for the attribution requirement). `openai/gpt-oss-120b:free` is the backup.

Set in `.env`:

```
OPENROUTER_MODEL=google/gemma-4-31b-it:free
```

> Note: the `gpt-oss` family sometimes routes output through a "reasoning" channel and returns `content: null`. The Go client reads `choices[0].message.content`, so a null `content` produces an empty reply. Avoid `gpt-oss-20b` for this reason.

---

## Verified queries

All confirmed via the Go API (`POST /api/chat`). Each reply began with *"According to the live fleet database..."*.

### Data / incident queries

| # | Query | Result |
|---|-------|--------|
| 1 | `Which elevators are shut down by TSSA?` | No TSSA shutdowns; reported follow-up / voluntary shutdowns instead |
| 2 | `Show inspection history for elevator 347` | Elevator **347** has 2 inspections |
| 3 | `What incidents were reported for elevator 22202?` | Elevator **22202** has 2 incidents (2011 near-miss) |
| 4 | `Which elevators need follow-up?` | Fleet-wide (no ID needed) |
| 5 | `How many incidents last year?` | Fleet-wide (no ID needed) |

**Verified elevator IDs** (have real data):
- Inspection history: `347` (also `88366`, `348`, `14540`, `14544` — all HIGH risk with follow-up history).
- Incidents: `22202` (others from `data/incident.json`: `22203`, `87727`, `17594`, `81630`, `28532`, `27865`, `20495`).
- Elevators with `latest_inspection_date = null` have **no** history — not useful for query #2.

### RAG / PDF queries (ChromaDB)

The indexed PDFs (docs `10078`–`10083`, 99 chunks in `data/chromadb`) are Rocket Elevators **Hydraulic Elevator Maintenance Procedures**.

| Query | Result |
|-------|--------|
| `How do you check for hydraulic fluid contamination in the maintenance procedure?` | Grounded; cited Document 10078 |
| `How to perform the monthly maintenance procedure on a hydraulic elevator?` | (suggested) Monthly checks |
| `What does the maintenance manual say about holed versus holeless hydraulic procedures?` | (suggested) Holed/holeless configs |

**Grounding verification:** the fluid-contamination answer reproduced exact source values that cannot be hallucinated — water `>0.1%` concern / `>0.5%` immediate, ISO 4406 cleanliness `18/16/13`, additive depletion `<50%`, TAN, viscosity. All traced back to chunks of doc `10078`.

**Nuance observed:** the model merged two distinct document sections (semi-annual lab analysis + the in-ground-cylinder "Monitoring Indicators") into one answer and broadened the scope — the source scopes those monitoring indicators to in-ground (holed) cylinders specifically. Every fact was faithful to the document (no hallucination), but the **framing/context** was generalized. Worth tracking in any RAG evaluation: the model is faithful to *content* but can loosen the *context*.

---

## RAG gotchas

1. **Routing needs 2+ RAG keywords.** `intent.go` computes `confidence = score / (score + 1.0)` and rejects below `ConfidenceFloor = 0.60`, i.e. the RAG score must be **≥ 1.5**. Keyword weights: `how to`=1.0, `how do you`=1.0, `procedure`=1.0, `manual`=0.5, `maintenance`=0.5. A single keyword (e.g. just "how do you" = 1.0 → confidence 0.50) falls back to advisory and **never queries ChromaDB**.

2. **First RAG query times out (cold start).** `PostChat` gives the MCP tool call a 10s context, but `rag.py` lazily loads the embedding model (`BAAI/bge-small-en-v1.5`, ~125 MB) on first use, exceeding 10s → `read SSE: context deadline exceeded` → advisory fallback. The **second** query works (model warm). Workaround: fire one throwaway RAG query to warm the model.

---

## How to run (Python 3.12)

The stack requires **Python 3.12** — `py -3` may resolve to 3.14, which has no prebuilt wheels for `torch`/`pandas` and fails to install. Use `py -3.12` for all Python.

```bash
# 1. Data already in cloud Postgres (ETL + explanations skipped)
# 2. RAG index (ChromaDB, local)
py -3.12 intelligence/rag_preprocessing.py --force      # PDFs in intelligence/rag_documents/ (gitignored)
# 3. MCP server (loads .env, connects to cloud DB)
py -3.12 -m platform.mcp.server                         # :8765
# 4. Go API (loads .env via env.go)
go run ./platform/api                                   # :8080
# 5. Dashboard UI (proxies /api/chat to the Go API)
py -3.12 platform/server.py                             # http://127.0.0.1:5000
```

Required in `.env`: `DATABASE_URL`, `OPENROUTER_API_KEY`, `OPENROUTER_MODEL=google/gemma-4-31b-it:free`.

---

## Follow-ups / improvements

- Warm the embedding model at MCP startup (like the removed Ollama warm-up), or raise the per-call MCP timeout for RAG, so the first RAG query never times out.
- The error log line `log.Printf("llm call failed: %v", err)` in `chat.go` was added to surface the raw OpenRouter error body — keep it.
