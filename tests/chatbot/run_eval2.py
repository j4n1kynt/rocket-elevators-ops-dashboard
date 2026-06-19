"""Targeted follow-up tests: scheduling write-gate, no-prediction path, K2 determinism.

Run after run_eval.py to drill into the edge cases. Dumps to results2.json.

  py -3 run_eval2.py
"""
import json
import sys
import time
from harness import chat, mcp

out = {"ground_truth": {}, "scenarios": []}


def log(m):
    sys.stderr.write(m + "\n"); sys.stderr.flush()


def gt(key, tool, args=None):
    log(f"[GT] {key}")
    try:
        out["ground_truth"][key] = mcp(tool, args or {})
    except Exception as e:
        out["ground_truth"][key] = {"_error": repr(e)}


def scen(sid, message, history=None, pending=None):
    log(f"[CHAT] {sid}: {message[:55]!r}")
    t0 = time.time()
    rec = {"id": sid, "message": message}
    try:
        r = chat(message, history=history, pending=pending)
        rec.update(reply=r.get("reply"), pending_action=r.get("pending_action"), history=r.get("history"))
    except Exception as e:
        rec["error"] = repr(e)
    rec["elapsed_s"] = round(time.time() - t0, 1)
    out["scenarios"].append(rec)
    return rec


# Ground truth: risk for an out-of-range id; re-pull the flood narrative set
gt("risk_fake", "get_elevator_risk", {"elevator_id": 999999999})
# Same query string the bot's pipeline uses (see run_eval.py note + K2 retraction).
gt("rag_flood_again", "search_incident_narratives", {"query": "Have we seen flooding incidents in elevators?", "limit": 5})

# R3: risk for a non-existent/invalid elevator — must say "no data"/"invalid", not guess
scen("R3", "Why is elevator 999999999 flagged as high-risk?")

# K2b / K2c: re-run flooding narrative twice to check determinism of the citation set
scen("K2b", "Have we seen flooding incidents in elevators?")
scen("K2c", "Have we seen flooding incidents in elevators?")

# S3 -> S4: schedule with an EXPLICIT date so a real (signed) pending_action is issued
s3 = scen("S3", "Schedule an inspection for elevator 37180 on 2026-06-23. Suspected hydraulic issue.")
log("PENDING_ACTION present: " + str(bool(s3.get("pending_action"))))
# S4: cancel the real pending action (verifies the confirmation gate WITHOUT a DB write)
scen("S4", "No, cancel that.", history=s3.get("history"), pending=s3.get("pending_action"))

with open("results2.json", "w", encoding="utf-8") as f:
    json.dump(out, f, indent=2)
log(f"\nDONE — {len(out['scenarios'])} scenarios. -> results2.json")
