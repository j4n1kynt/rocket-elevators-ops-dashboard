# Agent Evaluation

This document captures the classification signal analysis and the test query set for the four-agent chatbot introduced in Sprint 3. It is meant to be read alongside `docs/multi-agent-design.md`.

---

## Part 1 — Classification Signal Map

The router in `platform/api/router.go` delegates every incoming message to one of four agents. It does this by calling the existing `ClassifyIntent` function in `intent.go`, which scores the message against keyword groups and returns a `Classification` with an intent and a confidence score. Any intent that scores below the 0.6 confidence floor is downgraded to `advisory` and lands with the general agent. The router adds no classification logic of its own — it only maps the output of `ClassifyIntent` to an agent name.

### Data Agent

The data agent is selected when a message contains a numeric elevator ID (five to eight digits, such as `48210` or `9876543`) or one of these keywords: `inspection history`, `risk`, `incident`, `fleet`, `shutdown`, or `stats`. Each of these signals maps directly to one of the seven data tools the Go handler is allowed to call. Elevator IDs are the only entity `intent.go` extracts — the `E`-prefix format (`E12345`) is not recognized, so only bare numeric IDs trigger entity extraction.

### Scheduling Agent

The scheduling agent has two entry paths, which makes it the only agent with out-of-band routing. The first path is keyword-based: messages containing `schedule`, `book`, `arrange`, `inspection request`, or `set up an inspection` route here through the normal `ClassifyIntent` flow. The second path is a pre-emption check that runs before `ClassifyIntent` is called at all: if the request carries a non-empty `pending_action` field, the router sends the message straight to the scheduling agent regardless of its content. This is what allows Phase 2 confirmations (`yes, confirm`) and cancellations (`cancel that`) to reach the right agent even though those phrases carry no scheduling-specific keywords.

### Knowledge Agent

The knowledge agent is selected when a message contains `how`, `procedure`, `manual`, `maintenance`, `regulation`, `TSSA requirement`, `what causes`, or `past incidents`. These signals indicate that the user is asking about how something works or what a rule says, rather than asking about a specific elevator's live record. The practical distinction from the data agent is directional: knowledge queries ask about processes and regulations, data queries ask about a specific device's current state or history.

### General Agent

The general agent is the fallback. It has no positive keyword signals — it receives everything that does not reach the confidence floor for any other intent. Greetings, definitions, terminology questions, and anything ambiguous land here. This behavior is entirely inherited from `intent.go`'s `ConfidenceFloor` logic; the router contributes nothing new for this path.

### A note on signal conflicts

Two agents share overlapping vocabulary. The word `shutdown` appears as a data keyword (it maps to `get_tssa_shutdown_elevators`) but also appears naturally in regulatory questions ("what are the TSSA requirements for a shutdown elevator?") that belong to the knowledge agent. Similarly, `how` is a knowledge keyword but appears in data-flavored questions like "how often does elevator 92341 get inspected?". The presence of a numeric elevator ID is currently the strongest single signal — it tends to push a message toward the data agent even when a knowledge keyword is also present. Whether the scoring handles these conflicts correctly is the main thing the stress-test queries in Part 2 are designed to reveal.

---

## Part 2 — Test Query Set

The queries below are designed to exercise all four agent domains and to surface the edge cases most likely to cause misrouting. Each entry states the expected agent and the specific routing signal that should drive that decision. For borderline queries, a note explains what goes wrong if the scorer gets it backwards.

### Data Agent

**"What is the risk level of elevator 48210?"**
Expected agent: data. Routing signal: numeric elevator ID (`48210`, six digits) plus the keyword `risk`. Both fire independently, so this should reach a high confidence score for the `data_query` intent. The handler calls `get_elevator_risk`.

**"Show me the inspection history for elevator 92341."**
Expected agent: data. Routing signal: the phrase `inspection history`, which is an explicit data keyword, reinforced by the numeric ID `92341`. The handler calls `get_inspection_history`.

**"How many elevators are currently flagged for TSSA shutdown?"**
Expected agent: data. Routing signal: the keyword `shutdown`, which maps to `get_tssa_shutdown_elevators`. There is no numeric ID in this query, so the score comes entirely from the keyword — a useful check that fleet-level data queries route correctly without an entity present.

**"Which elevators need a followup inspection?"**
Expected agent: data. Routing signal: the broader `data_query` intent pattern in `intent.go`. The word `followup` is not itself a listed data keyword, so this query depends on the pattern matcher recognising it as a fleet-level lookup. Worth verifying that `followup` lands in the right keyword group rather than falling below the confidence floor. The handler calls `get_elevators_needing_followup`.

**"How many incidents were reported in the last year?"**
Expected agent: data. Routing signal: the keyword `incident`. The time framing ("last year") is not a routing signal — it is passed to `get_incident_count_last_year` as context after routing has already happened.

---

### Scheduling Agent

**"Schedule a periodic inspection for elevator 55123 on 2026-07-20."**
Expected agent: scheduling. Routing signal: keyword match on `schedule`. This is the happy-path input — it contains the keyword, a numeric elevator ID, a date, and an inspection type, giving the handler everything it needs to call `schedule_inspection` with `confirmed=false` and present a preview.

**"Book a followup inspection for elevator 78432 next Monday."**
Expected agent: scheduling. Routing signal: keyword match on `book`. The date `next Monday` is a relative reference and the Go handler must resolve it to an absolute date before calling the tool.

**"I need to arrange a followup — elevator 44210 failed last week."**
Expected agent: scheduling. Routing signal: keyword match on `arrange`. The date is missing, so the agent should return `[ACTION NEEDS MORE INFO]` and ask only for it. This query also contains `failed` and `followup`, which could weakly score toward the data intent, so the confidence margin is worth checking to confirm scheduling wins clearly.

**"Yes, confirm."** *(with pending_action set)*
Expected agent: scheduling. Routing signal: the `pending_action` pre-emption check, which runs before `ClassifyIntent` is called. The message content is irrelevant — the router short-circuits on the non-empty field and sends directly to the scheduling agent. If `pending_action` is absent, this message has no scheduling keyword and falls to `advisory`, which is the correct graceful-degradation outcome.

**"Cancel that."** *(with pending_action set)*
Expected agent: scheduling. Routing signal: same pre-emption path as above. The scheduling agent interprets the message as a non-confirmation, abandons the pending state, and confirms cancellation without writing to the database.

---

### Knowledge Agent

**"What is the procedure for hydraulic pressure loss?"**
Expected agent: knowledge. Routing signal: the keyword `procedure`. The handler calls `search_maintenance_docs` and the model synthesises the retrieved chunks into a step-by-step answer.

**"What maintenance is required after a cable replacement?"**
Expected agent: knowledge. Routing signal: the keyword `maintenance`. No competing signals — no numeric ID, no scheduling verb.

**"When is an inspection considered overdue under TSSA regulations?"**
Expected agent: knowledge. Routing signal: the keyword `regulation` (and `TSSA requirement` is also present). There is no numeric ID, so there is no data-agent competition. The handler calls `search_maintenance_docs`.

**"What patterns appear in past incident narratives for traction elevators?"**
Expected agent: knowledge. Routing signal: the phrase `past incidents`. The handler calls `search_incident_narratives`.

---

### General Agent

**"What does TSSA stand for?"**
Expected agent: general. Routing signal: confidence floor — no keyword reaches 0.6 for any operational intent, so `intent.go` returns `advisory` and the router sends to the general agent.

**"What is a Customer Shutdown?"**
Expected agent: general. Routing signal: confidence floor. The word `shutdown` is a data keyword, but in this phrasing it appears inside a definition question with no numeric ID or fleet-level framing. Whether `intent.go` still scores this above 0.6 for `data_query` is worth checking — if it does, the data agent receives a question it cannot answer with a tool result.

**"Hello, what can you help me with?"**
Expected agent: general. Routing signal: confidence floor. A greeting with no operational content; no keyword from any group fires.

---

### Borderline and Ambiguous

**"What causes elevator 48210 to be high risk?"**
Expected agent: data (contested). The phrase `what causes` is a knowledge keyword; the numeric ID `48210` and the word `risk` are data signals. If `intent.go` weights the knowledge keyword heavily enough to push the `rag` intent above 0.6, this routes to the knowledge agent — which has no access to live risk data and will either refuse or fabricate. The correct outcome is for the numeric ID plus `risk` to outweigh the single knowledge keyword. Logging the raw confidence for both intents on this query is the fastest way to confirm the scorer weights are correct.

**"How often does elevator 92341 get inspected?"**
Expected agent: data (contested). The word `how` is a knowledge keyword, but the numeric ID `92341` should be the stronger signal and push the score toward `data_query`. If `how` alone is enough to tip the balance, this misroutes to the knowledge agent, which cannot look up inspection frequency for a specific device. This query tests whether entity presence overrides a single keyword match.

**"What are the TSSA requirements for getting a shutdown elevator back in service?"**
Expected agent: knowledge (contested). `TSSA requirement` is a knowledge keyword; `shutdown` is a data keyword. There is no numeric ID to break the tie, so the result depends entirely on relative keyword weights. If `shutdown` wins, the data agent calls `get_tssa_shutdown_elevators` and returns a list of flagged devices — not a procedural answer about re-activation. The correct outcome is for `TSSA requirement` to score higher and route to the knowledge agent.

---

## What to watch for when running these queries

The borderline queries in the last section share a common failure mode: the wrong agent receives the message and returns a plausible-sounding but wrong answer. The data agent saying "no records found" when asked a procedural question, or the knowledge agent saying "I cannot access live data" when asked about a specific elevator's risk — both are technically correct refusals, but they reveal a routing error. Logging `Classification.Intent` and `Classification.Confidence` alongside each response during testing is the most direct way to confirm that the scorer is behaving as designed and that the confidence floor is positioned correctly.

---

## Part 3 — Run Results

Evaluated on 2026-06-26 using `tests/chatbot/run_eval_agents.py` against the deployed Render endpoints. 21 scenarios were sent. The `/api/chat` response does not yet include an `agent` field, so routing was inferred from the reply content — the data agent always includes `Source: live fleet database`, the knowledge agent cites document names, the scheduling agent includes a phase 1 preview or confirmation block, and the general agent answers from training data without any attribution line.

One ground truth pull failed: `get_tssa_shutdown_elevators` timed out on the MCP server. The remaining five pulls succeeded.

### Results by scenario

**D1 — "What is the risk level of elevator 48210?"**
Routing: data agent. Reply: "Elevator 48210 was not found in the fleet database." Correct — the agent did not fabricate a risk level and returned a properly formatted data-agent response with a `Source:` attribution.

**D2 — "Show me the inspection history for elevator 92341."**
Routing: data agent. Reply: "Elevator 92341 was not found in the fleet database." Correct — same no-fabrication behaviour as D1, with data-agent attribution.

**D3 — "How many elevators are currently flagged for TSSA shutdown?"**
Routing: data agent. Reply: "There are 20 elevators flagged for TSSA shutdown." The keyword `shutdown` without a numeric ID was sufficient to reach the data intent and call `get_tssa_shutdown_elevators`. The MCP ground truth pull for this tool timed out, so the count cannot be verified against a baseline, but the response format is correct.

**D4 — "Which elevators need a followup inspection?"** ❌
Routing: general agent (misroute). Reply: the agent explained what a followup inspection is and when it is scheduled — a definition, not a list. The word `followup` is not in the data keyword list in `intent.go`, and the query contains no other data signal, so the message fell below the confidence floor and landed with the general agent. The data tool `get_elevators_needing_followup` was never called.

**D5 — "How many incidents were reported in the last year?"**
Routing: data agent. Reply: "In 2015, the fleet recorded 506 total incidents, 142 involving injuries, 2 fatal." The keyword `incident` triggered the data intent correctly. The year 2015 is what `get_incident_count_last_year` returns for this dataset — the source data predates the current year. This is a data limitation, not a routing problem.

**SA1 — "Schedule a periodic inspection for elevator 55123 on 2026-07-20."**
Routing: scheduling agent. Reply: "Elevator ID 55123 was not found in the database." Correct — the keyword `schedule` routed to the scheduling agent, which ran Phase 1 validation and reported that the elevator does not exist. No `pending_action` was issued, which is the correct behaviour when Phase 1 fails.

**SA1_cancel — "Cancel that." (pre-emption, no pending)**
Routing: scheduling agent via general agent fallback. Because SA1 did not produce a `pending_action` (Phase 1 failed), this message was sent without a `pending_action` field. The router therefore called `ClassifyIntent`, which returned `advisory`, and the general agent handled it — but the reply was still sensible: "No action taken — the request to schedule the inspection for elevator 55123 has been cancelled." The pre-emption path itself is not exercised here because there was no pending to carry forward. A separate test (SA4/SA4_confirm below) confirms the pre-emption path works when `pending_action` is set.

**SA2 — "Book a followup inspection for elevator 78432 next Monday."**
Routing: scheduling agent. Reply: the agent asked for a specific date, explaining that `next Monday` is ambiguous. Correct — the keyword `book` triggered scheduling, the handler recognised the relative date could not be resolved, and the agent prompted for a concrete date. No `pending_action` was issued.

**SA3 — "I need to arrange a followup — elevator 44210 failed last week."** ❌
Routing: general agent (misroute). Reply: "I don't have access to scheduling systems, so I can't arrange the inspection directly." The keyword `arrange` did not trigger the scheduling intent. Whether this is a keyword scoring problem or an issue with the surrounding context (the em dash, the word `failed`) is unclear without inspecting the raw confidence scores. The general agent gave a passable explanation of what a followup inspection is but did not offer to book one.

**SA4 — "Schedule a periodic inspection for elevator 37180 on 2026-08-01."**
Routing: scheduling agent. A `pending_action` was returned. The Phase 1 preview listed the elevator location (75 Waterloo St, Government of Canada Bldg, Stratford), status (ACTIVE), date (2026-08-01), and type (Periodic), and asked the user to confirm.

**SA4_confirm — "Yes, confirm." (pre-emption with pending)**
Routing: scheduling agent via pre-emption. The `pending_action` from SA4 was carried forward. The router bypassed `ClassifyIntent`, the scheduling agent verified the HMAC signature and expiry, and called `schedule_inspection(confirmed=true)`. The inspection was written to the database. The reply confirmed the booking with elevator ID, location, type, and date. This is the only scenario in this suite that produced a database write.

**K1 — "What is the procedure for hydraulic pressure loss?"**
Routing: knowledge agent. The reply stepped through static and dynamic pressure tests, referencing maintenance documentation. The keyword `procedure` triggered the knowledge intent and `search_maintenance_docs` was called.

**K2 — "What maintenance is required after a cable replacement?"**
Routing: knowledge agent. The reply described the re-activation inspection requirement under TSSA regulations, citing cable replacement as a Major Alteration category. The keyword `maintenance` triggered the knowledge intent correctly.

**K3 — "When is an inspection considered overdue under TSSA regulations?"** ❌
Routing: data agent (misroute). Reply: "Source: live fleet database — most recent inspection per elevator." The data agent returned inspection records rather than a regulatory definition. The keyword `regulation` was apparently outweighed by some other signal — possibly `inspection` or `overdue` contributing to the `data_query` score — and the message did not reach the knowledge agent.

**K4 — "What patterns appear in past incident narratives for traction elevators?"** ❌
Routing: general agent (misroute). Reply: "I do not have access to live fleet data, historical incident narratives, or specific device records." The phrase `past incidents` is listed as a knowledge keyword in the design doc but did not score high enough to reach the knowledge intent. The message fell to `advisory` and the general agent responded. Ground truth confirms five relevant incident narratives exist in the ChromaDB collection.

**G1 — "What does TSSA stand for?"**
Routing: general agent. The reply correctly expanded the acronym and described the TSSA's regulatory role. No keyword matched any operational intent; the message fell below the confidence floor as expected.

**G2 — "What is a Customer Shutdown?"**
Routing: general agent. Despite `shutdown` being a data keyword, the query did not carry a numeric ID or fleet scope, and the surrounding words ("What is a") signalled a definition request. The scorer apparently did not push the `data_query` intent above 0.6, so it fell to `advisory`. This is the desired behaviour, though it took 56 seconds — notably slower than any other general-agent response.

**G3 — "Hello, what can you help me with?"**
Routing: general agent. OpsBot introduced itself correctly. No operational keyword fired.

**B1 — "What causes elevator 48210 to be high risk?"**
Routing: data agent. Reply used the data-agent attribution format and reported the elevator was not found. The numeric ID plus the keyword `risk` outweighed the knowledge keyword `what causes`. This is the expected outcome.

**B2 — "How often does elevator 92341 get inspected?"** ❌
Routing: knowledge or general agent (misroute). Reply: "In Ontario, most elevating devices must be inspected at least once per year by a licensed TSSA inspector, as required by O. Reg. 209/01." The answer describes the regulatory frequency rather than looking up elevator 92341's specific inspection history. The keyword `how` appears to have scored toward the knowledge or advisory intent, overriding the numeric ID signal. The data tool `get_inspection_history` was never called.

**B3 — "What are the TSSA requirements for getting a shutdown elevator back in service?"** ❌
Routing: data agent (misroute). Reply: "The data shows 20 elevators with non-passing most recent inspection outcomes from 2011 requiring follow-up or voluntary shutdown actions." The keyword `shutdown` scored above `TSSA requirement` and the data agent called `get_tssa_shutdown_elevators`. The reply lists flagged devices rather than answering the procedural question about re-activation. This is the exact failure mode predicted in Part 2.

---

### Summary

| Scenario | Expected agent | Routed correctly |
|---|---|---|
| D1 | Data | ✅ |
| D2 | Data | ✅ |
| D3 | Data | ✅ |
| D4 | Data | ❌ `followup` not in data keyword list |
| D5 | Data | ✅ |
| SA1 | Scheduling | ✅ |
| SA1_cancel | Scheduling (pre-emption) | — no pending issued (SA1 Phase 1 failed) |
| SA2 | Scheduling | ✅ |
| SA3 | Scheduling | ❌ `arrange` keyword not matched |
| SA4 | Scheduling | ✅ |
| SA4_confirm | Scheduling (pre-emption) | ✅ DB write confirmed |
| K1 | Knowledge | ✅ |
| K2 | Knowledge | ✅ |
| K3 | Knowledge | ❌ `regulation` outweighed by data signal |
| K4 | Knowledge | ❌ `past incidents` phrase not matched |
| G1 | General | ✅ |
| G2 | General | ✅ |
| G3 | General | ✅ |
| B1 | Data | ✅ numeric ID + `risk` beat `what causes` |
| B2 | Data | ❌ `how` beat numeric ID |
| B3 | Knowledge | ❌ `shutdown` beat `TSSA requirement` |

15 correct, 6 misroutes. The misroutes cluster around four gaps in `intent.go`'s keyword coverage: `followup` is not a data keyword, `arrange` is not a recognised scheduling keyword, `past incidents` did not score for the knowledge intent, and `regulation` alone is not strong enough to beat a competing data signal when no numeric ID is present.

### Recommended fixes for `intent.go`

Based on these results, the following additions and weight adjustments should be considered:

- Add `followup` to the data keyword list so "which elevators need a followup" routes to the data agent.
- Add `arrange` to the scheduling keyword list alongside `schedule`, `book`, and `book a`.
- Verify that `past incidents` is matched as a phrase rather than split tokens; if it is split, add `narratives` or `incident narrative` as a knowledge keyword.
- Increase the weight of `TSSA requirement` or `regulation` relative to `shutdown` when no numeric ID is present, so regulatory-framing questions route to the knowledge agent rather than the data agent.
- Investigate why `how` overrides a numeric elevator ID in B2 — entity signals should generally outweigh a single keyword match when the query contains a specific device reference.
