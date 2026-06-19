# Chatbot Evaluation — Claude as Test Harness (AND-107)

**Date:** 2026-06-19
**Target:** Deployed chatbot — Go API `POST /api/chat`
**Model under test:** `google/gemma-4-31b-it:free` (OpenRouter, per `docs/openrouter-chat-test-report.md`)
**Harness / judge:** Claude (Claude Code), a stronger model than the one the chatbot runs on.
**Endpoints exercised:**
- Chat API — `https://rocket-elevators-ops-dashboard.onrender.com/api/chat`
- MCP server — `https://rocket-elevators-ops-dashboard-2.onrender.com/mcp`

---

## TL;DR

The chatbot is **strong on data grounding and citation discipline** — it answers the five
mandatory queries accurately, cites its source on every data-backed answer, corrects false
premises, and refuses out-of-scope questions cleanly. The inspection-scheduling
confirmation gate works exactly as designed (signed preview, no write on cancel).

**Two gaps were found. (A previously reported "critical fabrication" was retracted — see below.)**

| Severity | Finding |
|---|---|
| ⬛ **RETRACTED** | **The earlier "fabricated citation (K2)" finding was a false positive in the harness, not a chatbot bug.** The ground truth was fetched with a *different* query string than the bot used; ChromaDB returns a different top-5 for a different query embedding, so the two result sets diverged and the mismatch looked like fabrication. Re-tested with the **exact same query string** for both sides: the bot's cited incident IDs match the ground truth **5/5, across 3/3 runs**. No fabrication. See K2 below. |
| 🟠 **GAP** | **Risk explanations are empty.** `risk_explanation` is `NULL` in the `predictions` table, so "Why is elevator X high-risk?" cannot return the specific risk factors the spec requires. The bot behaves correctly (it says no explanation is available rather than inventing one), but the **product requirement is unmet upstream** in the data pipeline. |
| 🟡 **GAP** | **Invalid / out-of-fleet IDs give a vague reply.** Asking about elevator `999999999` returns a generic "are you asking in general…?" clarifying question instead of "that elevator isn't in the fleet / that ID is invalid." |
| 🟠 **INTERMITTENT** *(fixed)* | **RAG questions sometimes answer "no information" even though the data exists in ChromaDB.** Root cause: embedding-model cold start on the idle-spun-down MCP server exceeds the 25 s MCP timeout → empty data → advisory fallback. Fixed by warming the embedding model at MCP startup (`warm_rag`). See finding below. |

---

## Methodology

Claude is used as an **adversarial judge**, not as the answer source. For every scenario the
harness does two things:

1. **Pull ground truth** by calling the MCP tool directly (the same tool the Go API calls
   internally) — e.g. `get_inspection_history`, `search_incident_narratives`.
2. **Ask the deployed chatbot** the natural-language question via `POST /api/chat`.

Claude then compares the chatbot's reply against the ground truth. The chatbot's answer is
**never trusted on its own** — a claim only passes if the underlying MCP data supports it.

This matters because the chatbot runs on a small free-tier model; the value of the harness is
catching the cases where that model *sounds* right but isn't grounded.

> ⚠️ **Methodology caveat (learned the hard way).** For semantic-search (RAG) tools, the ground
> truth **must be fetched with the exact same query string the bot's pipeline sends to the tool**,
> not a paraphrase. ChromaDB returns a different top-5 for a different query embedding, and the
> flood-narrative corpus has several near-duplicate entries with tied similarity scores — so a
> mismatched query produces a different-but-equally-valid result set. Comparing the bot's reply
> against a mismatched ground truth produced a **false "fabrication" signal** in the first pass of
> this report (see K2). Always match the query string for nondeterministic retrieval.

### Scoring rubric (0–3 per dimension)

| Dimension | 3 | 2 | 1 | 0 |
|---|---|---|---|---|
| **Accurate** | Matches ground truth exactly | Mostly right, minor omission | Contains an error | Wrong / fabricated |
| **Grounded** | Every claim traces to retrieved data | Mostly grounded, slight over-generalization | Partly ungrounded | Invented |
| **Cited** | Correct, existing source named | Source named but vague | Weak / partial citation | Citation missing or **fake** |

---

## Results Summary

| ID | Category | Question | Acc | Grnd | Cite | Verdict |
|---|---|---|:--:|:--:|:--:|---|
| D1 | Data (mandatory) | Which elevators shut down by TSSA? | 3 | 3 | 3 | ✅ PASS (exemplary) |
| D2 | Data (mandatory) | Inspection history for elevator 60503 | 3 | 3 | 3 | ✅ PASS |
| D3 | Data (mandatory) | How many incidents last year? | 3 | 3 | 3 | ✅ PASS |
| D4 | Data (mandatory) | Incidents for elevator 60503 | 3 | 3 | 3 | ✅ PASS |
| D5 | Data (mandatory) | Elevators with 'Follow up' on last inspection | 3 | 2 | 3 | ✅ PASS (no "more exist" caveat) |
| R1 | Risk | Why is elevator 37180 high-risk? | 3 | 3 | 3 | ✅ PASS — but exposes data gap |
| R2 | Risk | Why is elevator 60503 high-risk? | 3 | 3 | 3 | ✅ PASS (corrects false premise) |
| K1 | RAG (manuals) | Maintenance procedure for hydraulic pressure loss | 3 | 3 | 3 | ✅ PASS (exemplary) |
| K2 | RAG (incidents) | Have we seen flooding incidents? | 3 | 3 | 3 | ✅ PASS (earlier FAIL retracted — harness artifact) |
| K3 | RAG (paraphrase) | What do I do when hydraulic pressure drops? | 3 | 3 | 3 | ✅ PASS (paraphrase-robust) |
| G1 | Boundary | What is the capital of France? | 3 | — | — | ✅ PASS (clean refusal) |
| G2 | Edge | Inspection history for elevator 999999999 | 1 | — | — | 🟡 WEAK (vague, no "not found") |
| R3 | Edge | Why is elevator 999999999 high-risk? | 1 | — | — | 🟡 WEAK (vague, no "invalid") |
| S1 | Action | Schedule … *next Tuesday* (relative date) | 2 | — | — | 🟡 PARTIAL (asks instead of resolving) |
| S3 | Action | Schedule … *on 2026-06-23* (explicit date) | 3 | 3 | 3 | ✅ PASS (signed preview) |
| S4 | Action | "No, cancel that" | 3 | — | — | ✅ PASS (no DB write) |

Response times ranged from ~1.5 s (cached/advisory) to ~23 s (RAG with embedding). No errors,
no timeouts.

---

## Detailed Findings

### ✅ Data queries (D1–D5) — accurate and well-cited

All five mandatory queries returned correct answers, each prefixed with a source citation
("According to the inspections table", "Based on the predictions table").

**D1 is exemplary.** Ground truth: the `get_tssa_shutdown_elevators` tool carries the note
*"No explicit shutdown flag exists in the database."* The bot **honored that caveat** —
instead of falsely labeling 20 elevators as "TSSA shutdowns," it answered:

> "there are no elevators listed as having been shut down by the TSSA. The data does show two
> devices that were voluntarily shut down (Vol Shut Down) … elevator 81102 in Niagara Falls and
> elevator 1883 in Cardinal."

Cross-check: the only two `Vol Shut Down` outcomes in the result set are indeed 81102 and 1883.
This is exactly the honesty the requirement asks for.

**D3** matched ground truth to the number: 506 total / 142 injury / 2 fatal, and it correctly
surfaced that the data's "last year" resolves to **2015** rather than implying 2025.

**D4** honestly reported "no incidents" for elevator 60503 (ground truth: 0 rows) rather than
padding the answer.

**D5 minor note:** the bot listed all 20 returned rows but presented them as the complete set
("all 20 elevators listed in the provided data"). The tool was called with `limit=20`; the fleet
almost certainly has more follow-ups. The answer would be safer with a "showing the first 20"
caveat. *Grounded = 2.*

### ✅ K2 — Incident-narrative citations are faithful (earlier FAIL retracted)

Question: *"Have we seen flooding incidents in elevators?"*

**What the first pass reported (now retracted):** the bot listed incident `#1148032 / elevator
12529`, which was absent from the ground truth I had fetched — so I flagged it as a fabricated
citation, "reproduced 3/3."

**What actually happened:** the ground truth was fetched with the query string
`"flooding incidents in elevators"`, while the Go API forwards the user's *full* message
`"Have we seen flooding incidents in elevators?"` to `search_incident_narratives`. Those are two
different query embeddings, and the flood corpus has several near-duplicate "water damage flood"
narratives with tied similarity scores — so each query returns a different-but-valid top-5. I was
comparing the bot's answer against the wrong sample.

**Re-test with matched queries** (same string for the bot and the ground-truth call), 3 runs:

| Run | Ground-truth incident IDs | Bot's cited incident IDs | Fabricated |
|---|---|---|---|
| 1 | 1148032, 1148086, 1148116, 1148204, 1970196 | 1148032, 1148086, 1148116, 1148204, 1970196 | none |
| 2 | (same) | (same) | none |
| 3 | (same) | (same) | none |

Every cited incident is real and matches the retrieval **5/5, 3/3 runs**. The bot was faithful all
along. The original reply's IDs also match this matched-query ground truth exactly — confirming
the earlier "fabrication" was entirely a harness artifact, not chatbot behavior.

**Lesson:** verify nondeterministic RAG with a query-matched ground truth (see the Methodology
caveat above). This was my error, and it is the most important correction in this report.

### 🟠 R1 — Risk explanations are empty (data-pipeline gap)

Question: *"Why is elevator 37180 flagged as high-risk?"*

Ground truth: `risk_score = 1.0`, `risk_level = HIGH`, **`risk_explanation = null`**.

The bot answered correctly and **did not hallucinate factors**:

> "elevator 37180 has a risk level of HIGH with a risk score of 1.0. However, there is no specific
> explanation provided for why this device was flagged. … model version v4.1."

This is the **right behavior for the bot** (no fabrication), but the spec requirement —
*"The response should include the specific risk factors from the prediction data, not just a label"*
— is **unmet**, because the factors are missing from the database. `generate_explanations.py`
populates `risk_explanation` for HIGH-risk elevators; on the deployed DB that column is `NULL`
for 37180. **Action:** run/verify `generate_explanations.py` against the deployed database, then
re-test R1 — the bot should then surface the real factors.

### ✅ R2 — Corrects a false premise

Question presupposed elevator 60503 is "high-risk." Ground truth: it's **MEDIUM** (0.4693). The
bot pushed back: *"elevator 60503 is actually classified as MEDIUM risk, not high-risk…"* — it
trusted the data over the user's framing. Excellent.

> **Coverage gap (honest disclosure):** I could not test the "valid elevator that has **no**
> prediction row → say so" path. Both tested IDs (37180, 60503) have predictions, and the
> `predictions` table on the deployed DB appears to cover far more than the ~500 elevators the
> business doc describes (60503 is a MEDIUM-risk row). A true "no prediction" test needs an
> elevator ID known to be absent from the table. Recommend the team add one such case.

### ✅ K1 / K3 — RAG on manuals is faithful and paraphrase-robust

K1 (*"maintenance procedure for hydraulic pressure loss"*) and K3 (*"what do I do when hydraulic
pressure drops?"*) are semantically different phrasings of the same need. Both retrieved the same
documents (10078, 10082, 10083) and produced faithful, well-structured answers citing each
document by number. Spot-checks against the retrieved chunks confirmed:
- static-pressure test → gradual drop = internal leak at cylinder seals / control valve ✓
- running pressure below rated = worn pump / valve bypass / low fluid ✓
- active-failure response: shut off pump, contain oil, contact KONE, report TSSA ✓

This satisfies the spec's paraphrase-robustness requirement and grounds every claim in a real
document. One mild caveat (consistent with `openrouter-chat-test-report.md`): the model is faithful
to *content* but occasionally broadens *context*, merging routine-monitoring and emergency-response
material under one heading. No fabrication.

### 🟠 RAG reliability — intermittent "no information" on cold start (fixed)

Reported from real use: incident/RAG questions whose answers **do** exist in ChromaDB
*sometimes* come back as "I don't have that information." This is a **timing** failure, not a
data failure. Two distinct mechanisms produce the same symptom:

**1. Cold-start timeout (the intermittent one — the "sometimes").**
When a question routes to a ChromaDB tool (`search_maintenance_docs` / `search_incident_narratives`),
the Go API gives that MCP call a 25 s budget (`chat.go`). If it overruns:

```go
if result, err := CallMCPTool(...); err != nil {
    log.Printf("mcp tool %s failed: %v — falling back to advisory", ...)
    // dataContext stays EMPTY → the LLM has no retrieved data → "no information"
}
```

On Render's free tier the MCP instance spins down when idle. The first query after wake must
both start the container **and** lazily load the ~125 MB `bge-small-en-v1.5` embedding model
(`rag.py:_get_model`). That cold path can exceed 25 s → timeout → empty `dataContext` → "no
information." The **next** query, with the model warm, succeeds — hence the intermittency.

*Evidence:* in this eval, K3 took **23.4 s end-to-end while warm-ish** — already grazing the 25 s
ceiling. A cold instance clears it easily. (This is also why the team had already raised the
timeout 10 s → 25 s in commit `ee123ee`; 25 s still isn't enough on a cold start.)

**Fix applied:** `platform/mcp/rag.py` now exposes `warm_rag()` (loads the ChromaDB client and the
embedding model, plus a trivial `encode`), and `platform/mcp/server.py` calls it from the server
lifespan at startup (off the event loop, non-fatal if ChromaDB isn't populated; skip with
`MCP_SKIP_RAG_WARMUP`). The first **real** query no longer pays the cold-start cost. Both
collections share one embedding model, so warming it once covers maintenance docs *and* incident
narratives. Unit tests: `platform/mcp/test_rag.py`.

**2. Routing brittleness (a deterministic cousin — worth a follow-up).**
`intent.go` only routes incident questions to the ChromaDB narrative search when an *experiential*
cue is present (`"have we seen"`, `"happened before"`, …). A plainly phrased "are there any
flooding incidents?" matches the DATA_QUERY keyword `"incident"` (1.0) instead and is routed to
`get_incident_count_last_year` (a Postgres aggregate) — it **never touches ChromaDB**, so the
flooding narratives are never retrieved. This is deterministic, not intermittent, and is a
separate improvement (broaden narrative routing) not covered by the warm-up fix.

### 🟡 G2 / R3 — Invalid and out-of-fleet IDs

- `999999999` is rejected by the MCP tool's own validation (`elevator_id must not exceed 9,999,999`).
- For both the inspection-history (G2) and risk (R3) questions about this ID, the bot replied with a
  vague *"Are you asking how … work in general, or looking up a specific elevator?"* clarifying
  question — it never told the user the ID is invalid or not in the fleet.

Not a safety issue (it didn't invent data), but a **UX gap**: a user who typo'd an ID gets no
useful signal. Recommend a clear "I couldn't find elevator X in the fleet" / "that ID isn't valid"
response.

### ✅ G1 — Out-of-scope boundary

*"What is the capital of France?"* → clean refusal with a redirect to elevator topics. No leakage.

### ✅ S1–S4 — Inspection scheduling & the confirmation gate

| Step | Behavior | Verdict |
|---|---|---|
| **S1** — "…next Tuesday" (relative date) | Bot asks for an exact calendar date instead of resolving it. Safe (won't invent a date) but the business-doc example expects it to resolve "next Tuesday" → June 23. | 🟡 PARTIAL |
| **S3** — "…on 2026-06-23" (explicit date) | Returns a **signed** `pending_action` (HMAC + 10-min expiry), shows the **real DB location** (75 Waterloo St, Stratford), and asks yes/no. | ✅ PASS |
| **S4** — "No, cancel that" | "No inspection was booked and nothing was written to the database." Pending action cleared, **no DB write**. | ✅ PASS |

The write gate works as designed: a write is only previewed, never executed without an explicit
"yes," and the pending action is cryptographically signed so a tampered client payload is rejected.

> **Not tested on purpose:** the Phase-2 *confirmed write* ("yes") was **not** executed, to avoid
> writing a test inspection into the deployed database. The cancel path proves the gate holds. A
> confirmed-write test should be run against a disposable/staging DB, or with an agreed cleanup step.

---

## Recommendations (priority order)

1. ✅ **RAG cold-start "no information"** — **fixed** via `warm_rag()` at MCP startup
   (`platform/mcp/server.py`, `platform/mcp/rag.py`, tests in `platform/mcp/test_rag.py`).
   Verify in production that the first RAG query after an idle period now succeeds.
2. 🟠 **Populate `risk_explanation`** on the deployed DB (`generate_explanations.py`) so risk
   answers can include real factors, then re-test R1.
3. 🟡 **Broaden incident-narrative routing** — a plainly phrased "are there flooding incidents?"
   currently routes to a Postgres count, never to ChromaDB (see RAG finding §2).
4. 🟡 **Handle invalid / out-of-fleet IDs explicitly** — report "not found" instead of a vague
   clarifying question (G2, R3).
5. 🟡 **Resolve relative dates** ("next Tuesday") for scheduling, or keep asking — but align the
   behavior with the business-doc example either way (S1).
6. ⚪ Add a "showing first N of many" caveat to capped list answers (D5).
7. ⚪ Add a true "elevator with no prediction row" test case once an absent ID is known (R2 gap).

---

## Reproducing this evaluation

The harness is two small Python scripts (no third-party deps) that speak the chat API and the MCP
JSON-RPC/SSE protocol directly. Core shape:

```python
# chat(message) -> POST {API}/api/chat  ->  {reply, history, pending_action}
# mcp(tool, args) -> initialize -> notifications/initialized -> tools/call  (Streamable HTTP + SSE)
```

For each scenario: call `mcp(...)` for ground truth, call `chat(...)` for the answer, compare.
The MCP transport opens a fresh session per call (`mcp-session-id` header) and parses the first
`data:` line of the SSE response.

The runnable harness lives in **`tests/chatbot/`** (`harness.py`, `run_eval.py`, `run_eval2.py`,
plus a `README.md`). It is stdlib-only (no deps) and defaults to the Render deployment; override
`CHAT_API_URL` / `MCP_SERVER_URL` to point at a local stack:

```bash
cd tests/chatbot
py -3 run_eval.py     # full battery   -> results.json
py -3 run_eval2.py    # edge cases     -> results2.json
```

**Note on determinism:** the chatbot is a free-tier LLM, so wording varies between runs — score on
substance, not phrasing. For RAG tools, retrieval is also nondeterministic across near-duplicate
chunks, so always fetch ground truth with the **same query string** the bot uses before judging a
citation as wrong (see the Methodology caveat and K2).
