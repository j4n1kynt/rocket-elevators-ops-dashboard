"""Drive the full chatbot evaluation: capture MCP ground truth + chatbot replies.

Dumps everything to results.json for offline scoring. Each step is wrapped so one
failure does not abort the run. Progress goes to stderr.

  py -3 run_eval.py            # uses deployed endpoints (see harness.py)
"""
import json
import sys
import time
import traceback
from harness import chat, mcp

RID_HAS_PRED = 37180   # top of data/predictions.csv: HIGH, score 1.0
RID_FROM_TSSA = 60503  # appears in inspections (TSSA list)

out = {"ground_truth": {}, "scenarios": []}


def log(m):
    sys.stderr.write(m + "\n")
    sys.stderr.flush()


def gt(key, tool, args=None):
    log(f"[GT] {key}: mcp {tool} {args or ''}")
    try:
        out["ground_truth"][key] = mcp(tool, args or {})
    except Exception as e:
        out["ground_truth"][key] = {"_error": repr(e)}
        log(f"   ! {e}")


def scen(sid, label, message, history=None, pending=None):
    log(f"[CHAT] {sid}: {message[:60]!r}")
    t0 = time.time()
    rec = {"id": sid, "label": label, "message": message}
    try:
        r = chat(message, history=history, pending=pending)
        rec["reply"] = r.get("reply")
        rec["pending_action"] = r.get("pending_action")
        rec["history"] = r.get("history")
    except Exception as e:
        rec["error"] = repr(e)
        log(f"   ! {e}\n{traceback.format_exc()}")
    rec["elapsed_s"] = round(time.time() - t0, 1)
    out["scenarios"].append(rec)
    return rec


# ── Ground truth from MCP ──────────────────────────────────────────────────────
gt("tssa", "get_tssa_shutdown_elevators", {"limit": 20})
gt("inspection_history_60503", "get_inspection_history", {"elevator_id": RID_FROM_TSSA, "limit": 10})
gt("incident_count_last_year", "get_incident_count_last_year")
gt("incidents_60503", "get_elevator_incidents", {"elevator_id": RID_FROM_TSSA, "limit": 10})
gt("followup", "get_elevators_needing_followup", {"limit": 20})
gt("risk_37180", "get_elevator_risk", {"elevator_id": RID_HAS_PRED})
gt("risk_60503", "get_elevator_risk", {"elevator_id": RID_FROM_TSSA})
gt("rag_hydraulic", "search_maintenance_docs", {"query": "hydraulic pressure loss procedure", "n_results": 5})
gt("rag_hydraulic_paraphrase", "search_maintenance_docs", {"query": "what do I do when hydraulic pressure drops?", "n_results": 5})
gt("rag_flood", "search_incident_narratives", {"query": "flooding incidents in elevators", "limit": 5})

# ── Chatbot scenarios ──────────────────────────────────────────────────────────
# Data queries (5 mandatory)
scen("D1", "TSSA shutdowns", "Which elevators have been shut down by TSSA?")
scen("D2", "Inspection history", f"Show me the inspection history for elevator {RID_FROM_TSSA}")
scen("D3", "Incidents last year", "How many incidents were reported last year?")
scen("D4", "Incidents for elevator", f"What incidents have been reported for elevator {RID_FROM_TSSA}?")
scen("D5", "Follow up", "Which elevators have a 'Follow up' on their most recent inspection?")

# Risk predictions
scen("R1", "Risk w/ prediction", f"Why is elevator {RID_HAS_PRED} flagged as high-risk?")
scen("R2", "Risk no prediction", f"Why is elevator {RID_FROM_TSSA} flagged as high-risk?")

# RAG knowledge base
scen("K1", "RAG procedural", "What's the maintenance procedure for hydraulic pressure loss?")
scen("K2", "RAG incident narrative", "Have we seen flooding incidents in elevators?")
scen("K3", "RAG paraphrase robustness", "what do I do when hydraulic pressure drops?")

# Out-of-scope / hallucination guard
scen("G1", "Out of scope", "What is the capital of France?")
scen("G2", "Unknown elevator", "Show me the inspection history for elevator 999999999")

# Schedule inspection — Phase 1 preview, then CANCEL (no DB write)
s1 = scen("S1", "Schedule preview", f"Schedule an inspection for elevator {RID_HAS_PRED} next Tuesday. Suspected hydraulic issue.")
scen("S2", "Schedule cancel", "No, cancel that.", history=s1.get("history"), pending=s1.get("pending_action"))

with open("results.json", "w", encoding="utf-8") as f:
    json.dump(out, f, indent=2)
log(f"\nDONE — {len(out['scenarios'])} scenarios, {len(out['ground_truth'])} ground-truth pulls. -> results.json")
