"""Agent routing evaluation — covers all 20 queries from docs/agent-evaluation.md.

Each scenario maps to one query in the evaluation doc. Ground truth is pulled via MCP
for data and knowledge queries where the tool result is needed to judge the bot reply.

NOTE: SA5_confirm writes a real inspection record to the database (Phase 2 confirm path).
It uses elevator 37180 on 2026-08-01. Run against a dev/staging environment, or comment
it out if you do not want a DB write.

  py -3 run_eval_agents.py            # deployed endpoints (see harness.py)
  CHAT_API_URL=http://localhost:8080  py -3 run_eval_agents.py   # local stack
"""
import json
import sys
import time
import traceback
from harness import chat, mcp

# Known-good elevator IDs from the existing evaluation harness.
# The eval doc uses illustrative IDs (48210, 92341, etc.) — those are kept verbatim
# in the scenario messages so the router sees the exact query text from the doc.
# Ground truth is only pulled for tools/IDs that will return actual data.
RID_HAS_PRED = 37180   # HIGH risk, score 1.0 — used for Phase 2 confirm test
RID_FROM_TSSA = 60503  # appears in inspections + TSSA shutdown list

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
    log(f"[RUN] {sid}: {message[:65]!r}")
    t0 = time.time()
    rec = {"id": sid, "label": label, "message": message}
    try:
        r = chat(message, history=history, pending=pending)
        rec["reply"] = r.get("reply")
        rec["pending_action"] = r.get("pending_action")
        rec["history"] = r.get("history")
        rec["agent_name"] = r.get("agent_name")
    except Exception as e:
        rec["error"] = repr(e)
        log(f"   ! {e}\n{traceback.format_exc()}")
    rec["elapsed_s"] = round(time.time() - t0, 1)
    out["scenarios"].append(rec)
    log(f"      agent_name={rec.get('agent_name')} elapsed={rec.get('elapsed_s')}s")
    return rec


# ── Ground truth ──────────────────────────────────────────────────────────────
# Data tools — fleet-level queries (no specific elevator ID needed)
gt("tssa_shutdowns",      "get_tssa_shutdown_elevators",   {"limit": 20})
gt("followup_list",       "get_elevators_needing_followup", {"limit": 20})
gt("incident_count",      "get_incident_count_last_year")

# Knowledge tools — exact query strings the bot's pipeline will embed
# (paraphrasing would change the embedding and produce a different-but-valid top-5,
#  which would look like a fabricated citation — same caveat as run_eval.py K2 note)
gt("rag_cable",     "search_maintenance_docs",
   {"query": "What maintenance is required after a cable replacement?", "n_results": 5})
gt("rag_overdue",   "search_maintenance_docs",
   {"query": "When is an inspection considered overdue under TSSA regulations?", "n_results": 5})
gt("rag_traction",  "search_incident_narratives",
   {"query": "What patterns appear in past incident narratives for traction elevators?", "limit": 5})

# ── Data agent scenarios (D1–D5) ──────────────────────────────────────────────
# Routing signals: numeric elevator ID and/or data keywords
# (risk, inspection history, shutdown, fleet, incident, stats)

scen("D1", "Risk lookup — illustrative ID",
     "What is the risk level of elevator 48210?")
# Signal: numeric ID (48210) + keyword "risk" → data agent → get_elevator_risk.
# Elevator may not exist; agent should say "not found", not fabricate a risk level.

scen("D2", "Inspection history — illustrative ID",
     "Show me the inspection history for elevator 92341.")
# Signal: "inspection history" + numeric ID → data agent → get_inspection_history.

scen("D3", "TSSA shutdown fleet list",
     "How many elevators are currently flagged for TSSA shutdown?")
# Signal: keyword "shutdown" (no numeric ID) → data agent → get_tssa_shutdown_elevators.
# Compare reply against ground truth key "tssa_shutdowns".

scen("D4", "Follow-up needed list",
     "Which elevators need a followup inspection?")
# Signal: data_query intent pattern (followup) → data agent → get_elevators_needing_followup.
# "followup" is not a listed keyword — this tests the broader pattern matcher.

scen("D5", "Incident count last year",
     "How many incidents were reported in the last year?")
# Signal: keyword "incident" → data agent → get_incident_count_last_year.
# Compare reply count against ground truth key "incident_count".

# ── Scheduling agent scenarios (SA1–SA5) ──────────────────────────────────────
# Two routing paths: keyword match and pending_action pre-emption (bypasses ClassifyIntent).

sa1 = scen("SA1", "Schedule preview — full input",
           "Schedule a periodic inspection for elevator 55123 on 2026-07-20.")
# Signal: keyword "schedule" → scheduling agent → schedule_inspection(confirmed=false).
# Happy path: all fields present; agent should present a preview and ask for confirmation.

sa1_cancel = scen("SA1_cancel", "Cancel via pre-emption",
                  "Cancel that.",
                  history=sa1.get("history"), pending=sa1.get("pending_action"))
# Signal: non-empty pending_action → pre-emption path (ClassifyIntent NOT called).
# Agent should abandon pending state and confirm cancellation. No DB write.

scen("SA2", "Schedule preview — relative date",
     "Book a followup inspection for elevator 78432 next Monday.")
# Signal: keyword "book" → scheduling agent.
# "next Monday" is a relative date; handler must resolve to an absolute date before calling tool.

scen("SA3", "Schedule — missing date (needs more info)",
     "I need to arrange a followup — elevator 44210 failed last week.")
# Signal: keyword "arrange" → scheduling agent.
# Date is missing. Agent must return [ACTION NEEDS MORE INFO] and ask only for the date.
# "failed" and "followup" could weakly score toward data intent — margin worth checking.

# SA4/SA4_confirm: pre-emption path for "Yes, confirm." (Phase 2 write).
# Uses RID_HAS_PRED (37180) — a known-good elevator — so Phase 1 is likely to succeed
# and issue a real signed pending_action that the router can pre-empt on SA4_confirm.
# WARNING: SA4_confirm writes a real inspection record to the database.
sa4 = scen("SA4", "Schedule preview — for confirm chain",
           f"Schedule a periodic inspection for elevator {RID_HAS_PRED} on 2026-08-01.")
# Signal: keyword "schedule" → scheduling agent → schedule_inspection(confirmed=false).

scen("SA4_confirm", "Confirm via pre-emption (DB WRITE)",
     "Yes, confirm.",
     history=sa4.get("history"), pending=sa4.get("pending_action"))
# Signal: non-empty pending_action → pre-emption path.
# Agent verifies HMAC signature + expiry, then calls schedule_inspection(confirmed=true).
# This is the only scenario in this suite that writes to the database.

# ── Knowledge agent scenarios (K1–K4) ─────────────────────────────────────────
# Routing signals: procedure, maintenance, regulation, TSSA requirement, past incidents

scen("K1", "RAG procedural — hydraulic",
     "What is the procedure for hydraulic pressure loss?")
# Signal: keyword "procedure" → knowledge agent → search_maintenance_docs.

scen("K2", "RAG procedural — cable replacement",
     "What maintenance is required after a cable replacement?")
# Signal: keyword "maintenance" → knowledge agent → search_maintenance_docs.
# Compare citations against ground truth key "rag_cable".

scen("K3", "RAG regulatory — overdue inspections",
     "When is an inspection considered overdue under TSSA regulations?")
# Signal: keyword "regulation" (also matches "TSSA requirement") → knowledge agent.
# Compare citations against ground truth key "rag_overdue".

scen("K4", "RAG narrative — traction elevator incidents",
     "What patterns appear in past incident narratives for traction elevators?")
# Signal: phrase "past incidents" → knowledge agent → search_incident_narratives.
# Compare citations against ground truth key "rag_traction".

# ── General agent scenarios (G1–G3) ───────────────────────────────────────────
# Routing signal: confidence floor — no keyword reaches 0.6, intent.go returns advisory.

scen("G1", "Definition — TSSA acronym",
     "What does TSSA stand for?")
# No keyword from any group fires. Falls to advisory → general agent.

scen("G2", "Definition — Customer Shutdown",
     "What is a Customer Shutdown?")
# "shutdown" is a data keyword, but the phrasing is a definition question with no
# numeric ID or fleet scope. Check whether the data intent still crosses 0.6.

scen("G3", "Greeting",
     "Hello, what can you help me with?")
# No operational content. Should score well below the confidence floor.

# ── Borderline / ambiguous scenarios (B1–B3) ──────────────────────────────────
# These are the stress-test cases from the evaluation doc. Each has competing signals
# from two different agent domains. The expected routing and the risk of misrouting
# are documented inline.

scen("B1", "Borderline: knowledge vs data (what causes + numeric ID + risk)",
     "What causes elevator 48210 to be high risk?")
# Expected: data agent. "what causes" → knowledge signal; numeric ID + "risk" → data signals.
# If knowledge wins: agent has no live data, will refuse or fabricate.
# If data wins (correct): agent calls get_elevator_risk, reports the score and explanation.

scen("B2", "Borderline: knowledge vs data (how + numeric ID)",
     "How often does elevator 92341 get inspected?")
# Expected: data agent. "how" → knowledge signal; numeric ID → data signal.
# If knowledge wins: agent searches maintenance docs, finds no per-device answer.
# If data wins (correct): agent calls get_inspection_history and reports the frequency.

scen("B3", "Borderline: knowledge vs data (TSSA requirement + shutdown, no ID)",
     "What are the TSSA requirements for getting a shutdown elevator back in service?")
# Expected: knowledge agent. "TSSA requirement" → knowledge; "shutdown" → data keyword.
# No numeric ID, so neither side has the entity-signal advantage.
# If data wins: agent calls get_tssa_shutdown_elevators, returns a list of flagged devices.
# If knowledge wins (correct): agent searches for re-activation procedures.

# ── Output ────────────────────────────────────────────────────────────────────
with open("results_agent_eval.json", "w", encoding="utf-8") as f:
    json.dump(out, f, indent=2)

log(
    f"\nDONE — {len(out['scenarios'])} scenarios, "
    f"{len(out['ground_truth'])} ground-truth pulls → results_agent_eval.json"
)
