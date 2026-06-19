# Chatbot Test Harness (AND-107)

Uses Claude (Claude Code) as an **adversarial judge** of the deployed chatbot. The
chatbot's reply is never trusted on its own — the harness pulls **ground truth** from the
MCP tools (the same tools the Go API calls) and compares.

The written-up scenarios, scores, and findings live in **`docs/chatbot-evaluation.md`**.

## Files

| File | What it does |
|---|---|
| `harness.py` | `chat()` → `POST /api/chat`; `mcp()` → MCP `tools/call`. Stdlib only, no deps. |
| `run_eval.py` | Full battery: 5 mandatory data queries, risk, RAG, boundary, scheduling. Writes `results.json`. |
| `run_eval2.py` | Edge cases: scheduling write-gate, no-prediction path, RAG-citation determinism. Writes `results2.json`. |

## Run

```bash
cd tests/chatbot

# Against the Render deployment (default):
py -3 run_eval.py
py -3 run_eval2.py

# Against a local stack instead:
CHAT_API_URL=http://localhost:8080 MCP_SERVER_URL=http://localhost:8765 py -3 run_eval.py
```

Then read `results.json` / `results2.json` and compare each `scenarios[].reply` against the
matching `ground_truth` entry. The chatbot is a free-tier LLM, so wording varies run to run;
score on substance (accurate / grounded / cited), not phrasing.

## One-off queries

```bash
py -3 harness.py chat "How many incidents were reported last year?"
py -3 harness.py mcp get_inspection_history '{"elevator_id": 60503, "limit": 10}'
```

## Notes

- The Phase-2 **confirmed write** ("yes") is intentionally **not** exercised, to avoid writing a
  test inspection into the deployed database. The cancel path (`run_eval2.py` S3→S4) proves the
  confirmation gate holds. Run a confirmed-write test only against a disposable/staging DB.
- Render free tier has a cold start; the first request can take 30 s+.
