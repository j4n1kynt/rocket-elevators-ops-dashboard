"""Cross-eval: 5 queries that appear in both the agent-eval matrix and Sprint-2
(docs/chatbot-evaluation.md). Captures full replies + agent_name + MCP ground truth.

Selected queries and their Sprint-2 counterparts:
  D3/Sprint-D1  — TSSA shutdown list     (get_tssa_shutdown_elevators)
  D5/Sprint-D3  — Incident count         (get_incident_count_last_year)      [exact match]
  K1/Sprint-K1  — Hydraulic procedure    (search_maintenance_docs)           [near-identical]
  SA1/Sprint-S3 — Explicit-date schedule (schedule_inspection Phase 1 only)
  G1/Sprint-G1  — Advisory boundary      (no tool, confidence-floor fallback)

  py -3 run_crosseval.py
  CHAT_API_URL=http://localhost:8080  py -3 run_crosseval.py   # local stack
"""
import json
import sys
import time
import traceback
from harness import chat, mcp

out = {"ground_truth": {}, "scenarios": []}


def log(m):
    sys.stderr.write(m + "\n")
    sys.stderr.flush()


def gt(key, tool, args=None):
    log(f"[GT ] {key}: mcp {tool} {args or ''}")
    try:
        out["ground_truth"][key] = mcp(tool, args or {})
    except Exception as e:
        out["ground_truth"][key] = {"_error": repr(e)}
        log(f"   ! {e}")


def scen(sid, label, message, history=None, pending=None):
    log(f"[RUN] {sid}: {message[:70]!r}")
    t0 = time.time()
    rec = {
        "id": sid,
        "label": label,
        "message": message,
        "sprint2_counterpart": {
            "D3": "D1", "D5": "D3", "K1": "K1", "SA1": "S3", "G1": "G1",
        }.get(sid, "—"),
    }
    try:
        r = chat(message, history=history, pending=pending)
        rec["reply"] = r.get("reply")
        rec["agent_name"] = r.get("agent_name")
        rec["pending_action"] = r.get("pending_action")
        rec["history"] = r.get("history")
    except Exception as e:
        rec["error"] = repr(e)
        log(f"   ! {e}\n{traceback.format_exc()}")
    rec["elapsed_s"] = round(time.time() - t0, 1)
    out["scenarios"].append(rec)
    log(f"      agent_name={rec.get('agent_name')} elapsed={rec.get('elapsed_s')}s")
    return rec


# ── Ground truth ──────────────────────────────────────────────────────────────
# Only pull for queries where we compare bot reply against tool data.
gt("tssa_shutdowns",  "get_tssa_shutdown_elevators",  {"limit": 20})
gt("incident_count",  "get_incident_count_last_year")
gt("rag_hydraulic",   "search_maintenance_docs",
   {"query": "What is the procedure for hydraulic pressure loss?", "n_results": 5})

# ── D3 / Sprint-2 D1 — TSSA shutdown list ────────────────────────────────────
# Agent-eval: "How many elevators are currently flagged for TSSA shutdown?"
# Sprint-2:   "Which elevators shut down by TSSA?"
# Same tool (get_tssa_shutdown_elevators). Sprint-2 bot answered with Vol Shut Down caveat.
scen("D3", "TSSA shutdown — count vs list phrasing",
     "How many elevators are currently flagged for TSSA shutdown?")

# ── D5 / Sprint-2 D3 — Incident count (exact match) ─────────────────────────
# Agent-eval: "How many incidents were reported in the last year?"
# Sprint-2:   "How many incidents last year?" — same tool, same expected answer.
# Sprint-2 ground truth: 506 total / 142 injury / 2 fatal (data resolves "last year" to 2015).
scen("D5", "Incident count last year — exact overlap",
     "How many incidents were reported in the last year?")

# ── K1 / Sprint-2 K1 — Hydraulic procedure (near-identical phrasing) ─────────
# Agent-eval: "What is the procedure for hydraulic pressure loss?"
# Sprint-2:   "Maintenance procedure for hydraulic pressure loss"
# Sprint-2 verified docs 10078, 10082, 10083 and flagged no fabrication.
# Using the agent-eval wording; ground truth fetched with the same string.
scen("K1", "RAG hydraulic procedure — near-identical phrasing",
     "What is the procedure for hydraulic pressure loss?")

# ── SA1 / Sprint-2 S3 — Explicit-date scheduling (Phase 1 only) ──────────────
# Agent-eval: elevator 55123, date 2026-07-20
# Sprint-2:   elevator 37180, date 2026-06-23 — signed preview, no DB write
# Phase 1 only — do NOT confirm, to avoid a DB write on the deployed instance.
scen("SA1", "Scheduling Phase 1 — signed preview (no confirm)",
     "Schedule a periodic inspection for elevator 55123 on 2026-07-20.")

# ── G1 / Sprint-2 G1 — Advisory boundary ─────────────────────────────────────
# Agent-eval: "What does TSSA stand for?" — in-domain acronym, no data tool needed.
# Sprint-2:   "What is the capital of France?" — out-of-domain refusal.
# Both should hit the general/advisory agent (no keyword clears confidence floor).
# The replies differ by design: Sprint-2 gets a refusal; this should get a definition.
scen("G1", "Advisory boundary — in-domain acronym, no data signal",
     "What does TSSA stand for?")

# ── Output ────────────────────────────────────────────────────────────────────
with open("results_crosseval.json", "w", encoding="utf-8") as f:
    json.dump(out, f, indent=2)

log(
    f"\nDONE — {len(out['scenarios'])} scenarios, "
    f"{len(out['ground_truth'])} ground-truth pulls → results_crosseval.json"
)
