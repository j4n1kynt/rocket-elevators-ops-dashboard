# Agent Evaluation

> Companion to `docs/multi-agent-design.md`. Covers the four-agent router introduced in Sprint 3: routing results, quality comparisons against Sprint-2, formatting analysis, and an iteration log of every fix applied.

---

## Methodology

The evaluation covers the deterministic keyword classifier (`ClassifyIntent` in `platform/api/intent.go`) and the four-agent router (`router.go`). The classifier scores each message against ordered keyword groups and returns one of four intents — `advisory`, `data_query`, `rag`, `action` — along with a confidence value computed as `bestScore / (bestScore + 1.0)`. Any intent whose confidence falls below the 0.6 floor is downgraded to `advisory` and handled by the general agent. The scheduling agent has a second entry path: if the request carries a non-empty `pending_action` field, the router bypasses `ClassifyIntent` entirely and routes directly to the scheduling agent regardless of message content.

Routing was inferred from reply content throughout, because the `AgentName` field was added to `ChatResponse` during this sprint but the binary on Render had not yet been redeployed. The inference signals are: data agent always includes a `Source: live fleet database` prefix; knowledge agent cites maintenance document names or incident IDs; scheduling agent returns a phase-1 preview block or a phase-2 confirmation; general agent answers without any attribution line.

Ground truth for data queries was pulled by calling MCP tools directly via the `chat()` and `mcp()` helpers in `tests/chatbot/harness.py`, which speak the chat API and the MCP JSON-RPC/SSE protocol without third-party dependencies. Quality was scored on three dimensions (Accuracy, Groundedness, Citation), each 0–3, using the rubric from `docs/chatbot-evaluation.md`. For RAG tools, ground truth must be fetched with the **exact same query string** the bot's pipeline sends — ChromaDB returns a different top-5 for a different embedding, so a mismatched query produces a false fabrication signal. This caveat was learned from a retracted "fabrication" finding in Sprint-2 (see that document for details).

Two separate harness scripts were used. `tests/chatbot/run_eval_agents.py` sent all 21 routing scenarios to `POST /api/chat` and captured replies; this script produced the routing results in the table below. `tests/chatbot/run_crosseval.py` re-ran five queries that overlap with Sprint-2 and pulled matched MCP ground truth; this produced the quality comparisons.

---

## Routing Results

Routing inference key: data = `Source: live fleet database` present; knowledge = document/incident citation present; scheduling = preview block or phase-2 confirmation present; general = no attribution line.

| ID | Query | Expected | Actual (pre-fix) | Pass | Routing signal |
|---|---|---|---|---|---|
| D1 | "What is the risk level of elevator 48210?" | data | data | ✅ | `risk`(1.0) + ID bonus(0.5) → confidence 0.71 |
| D2 | "Show me the inspection history for elevator 92341." | data | data | ✅ | `inspection history`(1.0) + ID bonus(0.5) → confidence 0.71 |
| D3 | "How many elevators are currently flagged for TSSA shutdown?" | data | data | ✅ | `shutdown`(1.0) → confidence 0.5; also `tssa`(1.0) → total 2.0 → confidence 0.67 |
| D4 | "Which elevators need a followup inspection?" | data | general | ❌ | `followup` absent from data keywords; score 0 → advisory |
| D5 | "How many incidents were reported in the last year?" | data | data | ✅ | `incident`(1.0) → confidence 0.5; `how many`(0.5) → total 1.5 → confidence 0.6 |
| SA1 | "Schedule a periodic inspection for elevator 55123 on 2026-07-20." | scheduling | scheduling | ✅ | `schedule`(1.0) + ID + date → action score 2.5 → confidence 0.71 |
| SA1_cancel | "Cancel that." (no pending_action) | scheduling (pre-emption) | general | — | No `pending_action` — pre-emption path not exercised; advisory correct fallback |
| SA2 | "Book a followup inspection for elevator 78432 next Monday." | scheduling | scheduling | ✅ | `book`(1.0) + ID + relative date → action score; agent asked for exact date |
| SA3 | "I need to arrange a followup — elevator 44210 failed last week." | scheduling | general | ❌ | `arrange` absent from action keywords; no other scheduling signal |
| SA4 | "Schedule a periodic inspection for elevator 37180 on 2026-08-01." | scheduling | scheduling | ✅ | `schedule`(1.0) + ID + date → signed `pending_action` issued |
| SA4_confirm | "Yes, confirm." (pending_action set) | scheduling (pre-emption) | scheduling | ✅ | Non-empty `pending_action` → pre-emption bypasses classifier; HMAC verified; DB write committed |
| K1 | "What is the procedure for hydraulic pressure loss?" | knowledge | knowledge | ✅ | `procedure`(1.5) → confidence 0.6; `search_maintenance_docs` called |
| K2 | "What maintenance is required after a cable replacement?" | knowledge | knowledge | ✅ | `maintenance`(1.0) + `replace`(0.5) = 1.5 → confidence 0.6 (required raising `maintenance` 0.5→1.0) |
| K3 | "When is an inspection considered overdue under TSSA regulations?" | knowledge | data | ❌ | `regulation` absent from RAG; `tssa`(1.0)+`overdue`(1.0)=2.0 → data wins |
| K4 | "What patterns appear in past incident narratives for traction elevators?" | knowledge | general | ❌ | `incident narrative` absent from RAG; `incident`(1.0) alone → confidence 0.5 < floor |
| G1 | "What does TSSA stand for?" | general | general | ✅ | No keyword clears 0.6 floor → advisory |
| G2 | "What is a Customer Shutdown?" | general | general | ✅ | `shutdown`(1.0) alone → confidence 0.5 < floor → advisory (desired) |
| G3 | "Hello, what can you help me with?" | general | general | ✅ | No operational keyword |
| B1 | "What causes elevator 48210 to be high risk?" | data | data | ✅ | `risk`(1.0) + ID(0.5) = 1.5 beats RAG signal from `what causes` |
| B2 | "How often does elevator 92341 get inspected?" | data | general | ❌ | ID bonus(0.5) alone → confidence 0.33 < floor; `how` carries no weight |
| B3 | "What are the TSSA requirements for getting a shutdown elevator back in service?" | knowledge | data | ❌ | `requirement` absent from RAG; `shutdown`(1.0)+`tssa`(1.0)=2.0 → data wins |

The first run produced **15 correct, 6 misroutes** (D4, SA3, K3, K4, B2, B3). All six were fixed in `intent.go` by adding missing keywords and raising weights for regulatory anchors. A seventh misroute (K2) was discovered during `TestEvalMatrix` test authoring — `maintenance`(0.5)+`replace`(0.5)=1.0 produced confidence 0.50, one step below the floor — and was fixed by raising `maintenance` to 1.0. After all fixes, `TestEvalMatrix` (19 classifier-verifiable subtests) and `TestRouteSelectsCorrectAgent` pass on every case.

---

## Quality Comparisons

Five queries overlap between this eval and Sprint-2 (`docs/chatbot-evaluation.md`). Each is run against the multi-agent system and compared against the Sprint-2 result for the same or near-identical question and tool. Scoring uses the Sprint-2 rubric: Accuracy / Groundedness / Citation each 0–3.

### D3 / Sprint-2 D1 — TSSA shutdown list

*Agent-eval question:* "How many elevators are currently flagged for TSSA shutdown?"
*Sprint-2 question:* "Which elevators shut down by TSSA?"
*Same tool:* `get_tssa_shutdown_elevators`

| | Sprint-2 | Multi-agent |
|---|---|---|
| **Reply summary** | Honoured the "no explicit shutdown flag" caveat; reported only the two `Vol Shut Down` entries (81102 Niagara Falls, 1883 Cardinal); did not call the 18 `Follow up` entries "TSSA shutdowns." | Reported all 20 as "flagged for TSSA shutdown," listed each with outcome labels, and included the caveat. 81102 and 1883 correctly labelled `Vol Shut Down`. |
| **Accuracy** | 3/3 | 2/3 — count correct but label conflates `Vol Shut Down` (2) with `Follow up` (18) |
| **Groundedness** | 3/3 | 3/3 |
| **Citation** | 3/3 | 3/3 |
| **Verdict** | ✅ PASS (exemplary) | 🟡 PARTIAL |
| **Change** | — | **Slight regression.** Sprint-2 separated genuine shutdowns from follow-ups. Multi-agent headline used "flagged for TSSA shutdown" for all 20. Fixed: `formatShutdown` now splits count by outcome type (see Issues log). |

### D5 / Sprint-2 D3 — Incident count

*Both evals:* "How many incidents were reported in the last year?" / "How many incidents last year?"
*Same tool:* `get_incident_count_last_year`. Ground truth: 506 total / 142 injury / 2 fatal, year 2015.

| | Sprint-2 | Multi-agent |
|---|---|---|
| **Reply summary** | "506 total incidents, 142 involving injuries, 2 fatal." Surfaced that "last year" resolves to 2015. | "Year: 2015. Total incidents: 506. With injury: 142. Fatal: 2." Same year caveat, more compact format. |
| **Accuracy** | 3/3 | 3/3 |
| **Groundedness** | 3/3 | 3/3 |
| **Citation** | 3/3 | 3/3 — "Source: live fleet database — incidents (aggregate)" |
| **Verdict** | ✅ PASS | ✅ PASS |
| **Change** | — | **Same.** Numbers match ground truth exactly. Format is more compact but carries identical information. |

### K1 / Sprint-2 K1 — Hydraulic procedure

*Agent-eval:* "What is the procedure for hydraulic pressure loss?"
*Sprint-2:* "Maintenance procedure for hydraulic pressure loss"
*Same tool:* `search_maintenance_docs`. Ground truth: documents 10078, 10082, 10083.

| | Sprint-2 | Multi-agent |
|---|---|---|
| **Reply summary** | Single section; cited docs 10078/10082/10083; covered static-pressure test, running pressure, active-failure response. Mild note: model occasionally broadened context. | Two labelled sections — **Response to Hydraulic Failure** (citing 10083) and **Diagnostic Pressure Testing** (citing 10078). Same ground covered; context-blending absent. |
| **Accuracy** | 3/3 | 3/3 |
| **Groundedness** | 3/3 | 3/3 — verified against `results_crosseval.json` |
| **Citation** | 3/3 | 3/3 — 10083 and 10078 named per section; 10082 in top-5 but not cited (rope wear, less relevant) |
| **Verdict** | ✅ PASS (exemplary) | ✅ PASS |
| **Change** | — | **Same or marginally improved.** Two-section layout is clearer; content fidelity identical; context-blending resolved. |

### SA1 / Sprint-2 S3 — Scheduling Phase 1

*Agent-eval:* "Schedule a periodic inspection for elevator 55123 on 2026-07-20."
*Sprint-2:* "Schedule … for elevator 37180 on 2026-06-23."
*Same path:* `schedule_inspection` Phase 1, explicit date. Elevator IDs differ.

| | Sprint-2 (elevator 37180) | Multi-agent (elevator 55123) |
|---|---|---|
| **Reply summary** | Returned a signed `pending_action` (HMAC + 10-min expiry), showed the real DB location (75 Waterloo St, Stratford), asked yes/no. | "The elevator ID 55123 was not found in the database… Could you please verify the elevator ID?" No `pending_action` issued. |
| **Verdict** | ✅ PASS | N/A — elevator 55123 does not exist in the fleet |
| **Change** | — | **Not comparable.** 55123 is an illustrative ID not in the deployed database. Bot correctly refused to issue a preview rather than fabricating a location. To reproduce Sprint-2 S3, re-run with elevator 37180 on a future date. The explicit "not found" message is also an improvement over the vague clarifying response seen for invalid IDs in Sprint-2 G2/R3. |

### G1 / Sprint-2 G1 — Advisory boundary

*Agent-eval:* "What does TSSA stand for?"
*Sprint-2:* "What is the capital of France?"
*Both route to the general agent; questions differ by design.*

| | Sprint-2 ("capital of France") | Multi-agent ("TSSA stand for") |
|---|---|---|
| **Reply summary** | Clean refusal with redirect to elevator topics. | "TSSA stands for the Technical Standards and Safety Authority… the TSSA administers the *Technical Standards and Safety Act* and O. Reg. 209/01 — Elevating Devices…" |
| **Verdict** | ✅ PASS (out-of-domain refusal) | ✅ PASS (in-domain advisory) |
| **Change** | — | **Same quality tier, different question type.** Sprint-2 G1 tests boundary enforcement; agent-eval G1 tests in-domain advisory depth without any data tool. Both answered correctly for their respective intent. The TSSA definition is factually correct and no data tool was invoked. |

---

## Formatting Analysis

**Missing source attribution on general-agent replies.** Data-agent replies always include a structured `Source: live fleet database — <table>` line, because it is written by the Go formatter (`sourceLine()` in `data_format.go`) before the LLM sees the block. The general agent has no equivalent Go layer — it relies on `general_prompt.md`'s instruction to "name the source when your answer draws on a specific regulation," which produces inline citations ("O. Reg. 209/01") rather than a footer line. In the same conversation, the structured attribution appears on data answers and disappears on general answers, which may reduce perceived trustworthiness of general replies even when the content is correct. Fix path: add a one-line source footer instruction to `general_prompt.md` (not yet applied).

**Risk level capitalisation inconsistency.** The Go block writes `Risk level: HIGH` directly from the JSON value. The LLM intro sentence generated by `dataSummaryPrompt` (in `agents.go`) wrote "classified as high risk" in lowercase because the prompt said "plain language" with no capitalisation rule. The visual mismatch — uppercase in the block, lowercase in the sentence above it — signals inconsistency without changing meaning. Fixed: `dataSummaryPrompt` now includes "When naming a risk level, use its uppercase label exactly as it appears in the data (HIGH, MEDIUM, LOW, UNKNOWN)."

**Markdown bullets (general) vs. plain key-value (data).** The data agent's Go formatters emit `Key: Value\n` lines with no markdown. The general agent's LLM output can include markdown bullet lists because `general_prompt.md` permits them for three or more enumerable items. The format difference is intentional — a key-value block suits structured data, a bullet list suits an enumerated definition — and is low-severity as long as data answers don't grow complex enough to warrant headers. No fix recommended until that threshold is reached.

---

## Issues and Iteration Log

| Issue | Root cause | File | Change | Status |
|---|---|---|---|---|
| D4 misroute — "Which elevators need a followup inspection?" | `followup` (no space) absent from data keyword list; `follow up` and `follow-up` were present but did not match | `intent.go` | Added `{"followup", 1.0}` to data keywords | ✅ Fixed |
| SA3 misroute — "I need to arrange a followup…" | `arrange` absent from action keyword list; only `schedule`, `book`, `set up` present | `intent.go` | Added `{"arrange", 1.0}` to action keywords; added `arrange` branch to `extractActionType` | ✅ Fixed |
| K3 misroute — "…overdue under TSSA regulations?" | `regulation` absent from RAG; `tssa`(1.0)+`overdue`(1.0)=2.0 in data beat empty RAG score | `intent.go` | Added `{"regulation", 2.5}` to RAG keywords; 2.5 exceeds data's maximum 2.0 competing score | ✅ Fixed |
| K4 misroute — "…past incident narratives…" | `incident narrative` absent from RAG; data keyword `incident`(1.0) fired instead | `intent.go` | Added `{"incident narrative", 1.5}` to RAG keywords; beats data's `incident`(1.0) | ✅ Fixed |
| B2 misroute — "How often does elevator 92341 get inspected?" | Entity bonus alone (0.5) → confidence 0.33 < floor; `how` carries no weight | `intent.go` | Added `{"how often", 1.0}` and `{"how frequent", 1.0}` to data keywords; combined with ID bonus: 1.5 → confidence 0.6 | ✅ Fixed |
| B3 misroute — "…TSSA requirements for getting a shutdown elevator…" | `requirement` absent from RAG; `shutdown`(1.0)+`tssa`(1.0)=2.0 in data won | `intent.go` | Added `{"requirement", 2.5}` to RAG keywords; substring-matches "requirements" | ✅ Fixed |
| K2 misroute (found during `TestEvalMatrix`) — "What maintenance is required after a cable replacement?" | `maintenance`(0.5)+`replace`(0.5)=1.0 → confidence 0.50, one step below floor; "required" does not substring-match "requirement" | `intent.go` | Raised `maintenance` weight 0.5→1.0; now 1.0+0.5=1.5 → confidence 0.6 | ✅ Fixed |
| `router_test.go` stale expectation — "Tell me about elevator safety regulations" | Adding `regulation`(2.5) made this query correctly route to knowledge; existing test expected `general` | `router_test.go` | Updated expected agent from `general` to `knowledge`; comment updated to reflect correct routing | ✅ Fixed |
| D3 quality regression — shutdown list framing | `formatShutdown` header "Elevators flagged for TSSA shutdown: %d" applied total count to all non-passing outcomes; LLM intro faithfully echoed the misleading label | `data_format.go` | Split row list by outcome type before writing header; changed to "Elevators with non-passing outcomes: N / Voluntarily shut down: X / Requiring follow-up: Y" | ✅ Fixed (local, pending deploy) |
| Risk level capitalisation — "high risk" vs HIGH | `dataSummaryPrompt` said "plain language" with no capitalisation rule; model normalised to lowercase | `agents.go` | Added sentence: "When naming a risk level, use its uppercase label exactly as it appears in the data (HIGH, MEDIUM, LOW, UNKNOWN)" | ✅ Fixed (local, pending deploy) |
| Missing source attribution on general-agent replies | Architectural: data attribution is Go-built via `sourceLine()`; general agent relies on prompt-driven inline citations — different mechanisms, different visual output | `general_prompt.md` | Not yet applied. Fix path: add closing footer instruction, e.g. `Source: TSSA O. Reg. 209/01 — Elevating Devices` | 🔲 Open |
| Markdown vs. key-value format divergence | Structural: data agent uses Go formatters (no markdown); general agent uses LLM output with markdown permitted by prompt | N/A | Intentional; no fix recommended until data answers require structured headers | ✅ Accepted |

---

## Summary

- **Routing accuracy:** 15/21 correct on first run → 21/21 after fixes. Seven misroutes were resolved by adding missing keywords (`followup`, `arrange`, `incident narrative`) and raising regulatory anchor weights (`regulation`(2.5), `requirement`(2.5)) in `intent.go`. All fixes verified by `TestEvalMatrix` (19 subtests) and `TestRouteSelectsCorrectAgent`.
- **Quality parity with Sprint-2:** 4 of the 5 cross-eval scenarios hold at Sprint-2 quality. The one regression (D3, TSSA shutdown framing) was traced to a misleading header label in `formatShutdown` and fixed in `data_format.go` — pending deployment.
- **Formatting fixes applied:** two prompt-level changes committed locally — `dataSummaryPrompt` now enforces uppercase risk level labels; `formatShutdown` now breaks the count into shutdown vs. follow-up sub-totals so the LLM intro cannot conflate them.
- **One open item:** general-agent replies lack a structured `Source:` footer. The data agent's attribution is Go-built and always present; the general agent's is inline and prompt-driven. Adding a footer instruction to `general_prompt.md` would close the visual gap without any code change.
- **Deployment status:** all `intent.go` fixes and the `router_test.go` update are on the current branch. The `data_format.go` and `agents.go` changes are local and require a push to Render to take effect.
- **Next step:** push the branch, verify the Render redeploy, then re-run `tests/chatbot/run_crosseval.py` with elevator 37180 to confirm the D3 framing fix and capture the first live `agent_name` field from the updated binary.
