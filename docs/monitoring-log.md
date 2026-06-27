# Conversation Monitoring Log — S3-12

This log records what we learned from real chatbot conversations. We used the
logging from S3-7 and the analytics page from S3-8 to review live traffic.
Then we looked for problems we did not think to test, decided what to do, and
checked if the change helped.

---

## How we collected and reviewed the data

- **Source:** the live deployment at
  `https://rocket-elevators-ops-dashboard.onrender.com`.
- **How:** the team and teammates used the chatbot during testing. Every turn
  was logged to the `conversations` and `messages` tables (S3-7).
- **How we reviewed:** the analytics page (`/conversations`) plus the read
  API:
  - `GET /api/conversations/stats` — totals and agent split.
  - `GET /api/conversations` — the list of recent conversations.
  - `GET /api/conversations/{id}` — the full message thread for one chat.
- **Date of review:** 2026-06-26 (two passes: an early pass at 60 conversations,
  then a second pass at 87 conversations after a scheduling fix went live).

### Volume (meets the ≥20 requirement)

| Metric | Value |
| --- | --- |
| Conversations logged | **87** |
| Total messages | 316 |
| Assistant turns | 158 |
| Avg messages per conversation | 3.6 |

We reviewed in two passes. The first pass covered conversations 1–60. After it,
the team shipped a scheduling fix. The second pass covered the new
conversations 61–87, so we could check if the fix helped. This gave us a real
before/after, which is what the iteration part of this card needs.

### Agent distribution (assistant messages, all 87 conversations)

| Agent | Count |
| --- | --- |
| scheduling | 88 |
| general | 38 |
| data | 23 |
| knowledge | 9 |

**First read of the numbers:** scheduling has by far the most turns. Many early
turns were retries and failed attempts. After the fix, scheduling also has real
happy paths (three inspections were booked end to end). The general agent is
second, and part of that is still a routing bug (see Findings 1 and 2), not real
"general" questions. Knowledge is the smallest, which matches how few procedure
questions users asked.

---

## Findings

Each finding lists: **what we observed**, **what action we took (or why none
was needed)**, and **whether it helped**.

### Finding 1 — Scheduling flow now works end to end (FIXED — verified)

**Observed (first pass, conversations 1–60).** The scheduling flow often broke.
The agent asked "what inspection type?" The user answered with a single word
like `followup` or `periodic`, and the next turn went to the **general** agent
instead of back to scheduling. The general agent said "I do not have access …
please use the dashboard." The flow died right when the user gave the missing
detail. In conversations 1–60 there were **0 successful end-to-end bookings**.

- **First-pass count:** 13 of 118 assistant turns were a bare type/confirm reply
  the general agent answered (a misroute). Examples: CONV 5, 12, 49, 53, 54, 55.

**Root cause.** Two things combined:
1. A bare word like `followup` matched no intent keyword in `intent.go` (the
   keywords are `follow up` and `follow-up`, with a space or hyphen). So the
   classifier returned `advisory` → the general agent.
2. When Phase 1 was missing only the type, the MCP tool returned a validation
   error. The agent asked for the type but did **not** set a `pending_action`,
   so the next turn had no scheduling state and the pre-emption in `Route`
   (`router.go`) never fired.

**Action taken.** The team shipped a set of scheduling fixes:
- Re-run Phase 1 when the user gives the type during a pending confirmation
  (`agents.go` `default` branch; commit `4682685`).
- Carry the previous tool on an ID-only follow-up (commit `851d147`).
- Harden Phase 2 and align the payload with the real `write_tools.py` contract,
  plus a grounded confirmation message when the LLM is down after the write
  (commits `6b94e61`, `f574fe3`).

**Whether it helped — YES, verified in the second pass (conversations 61–87).**
The happy path now works:
- CONV 78: preview → `yes` → booked, **inspection ID 9000006**.
- CONV 83: preview → `yes` → booked, **inspection ID 9000007**.
- CONV 86: preview → `yes` → booked, **inspection ID 9000008**.
- CONV 80: `no` → "scheduling has been cancelled. No inspection was booked." —
  cancel works and writes nothing.
- The preview is now clean and grounded, with location and status (CONV 79, 85,
  86), not raw JSON.

This is a clear before/after: **0 bookings in 1–60, 3 real bookings in 61–87.**
Marked **fixed** for the main flow. The remaining edge cases are Findings 2–3.

---

### Finding 2 — The general agent still sometimes claims an inspection was scheduled (STILL OPEN)

**Observed.** When `yes` lands on the general agent (instead of scheduling), the
general agent says the inspection is booked — even though it has **no tools** and
wrote nothing.

- **Count:** 4 turns across all 87 conversations claimed a schedule/confirm that
  never happened.
- **Old examples:** CONV 48, 59.
- **New example (after the fix):** CONV 87 (2026-06-26 01:49 — one of the newest
  threads). Preview shown → user said `yes` → **general** agent replied "The
  periodic inspection for elevator 10054 has been scheduled for July 25, 2026."
  No write happened. When the user then typed "NOOOO", the general agent admitted
  "The interaction we just went through was a simulation." The same false
  confirmation repeats later in CONV 87 for elevator 22906.

**Why this is serious.** This is worse than a dead end. It tells the user a write
happened when it did not. The user may believe the job is done. This is a safety
and trust problem, not only a UX one.

**Why it still happens after the fix.** The happy path works only when the
scheduling Phase 1 sets a valid signed `pending_action` that round-trips to the
next turn. When the preview is produced as model text without a real
`pending_action` (or the action does not round-trip), the next `yes` has no
pending state. The classifier sees `yes`, finds no keyword, returns advisory, and
the general agent answers — then invents a success message.

**Action taken.** None yet in code. Logged as the top open issue.

**Recommended fix.** Two layers:
1. Make `yes`/`no` route back to scheduling even with no pending action when the
   last assistant turn showed a scheduling preview (carry intent in
   `contextCarry`/`isFollowUp`).
2. Harden `prompts/general_prompt.md`: the general agent must never say an action
   was done and must never claim it scheduled anything — it has no tools.

**Whether it helped.** Open. Needs its own `feat/` branch and PR.

---

### Finding 3 — Raw tool calls and model "thinking" leak into the reply

**Observed.** The scheduling agent sometimes prints the tool call as plain text
instead of running it, or leaks model reasoning tokens.

- **Count:** 11 assistant turns leaked raw syntax across all 87 conversations.
  This got **more frequent** in the second pass (3 in 1–60, 8 in 61–87), with
  new leak formats.
- **Examples:**
  - CONV 41, 51, 75, 77, 81: `[TOOL_CALL] {tool => "schedule_inspection", …}`.
  - CONV 36, 38, 48: a raw JSON block shown instead of a clean preview.
  - CONV 32: `<|channel>thought<channel|>` — raw thinking tokens leaked.
  - CONV 64, 65: `<FunctionCall>{'tool' => 'schedule_inspection', …}` — a new
    leak format the current guard does not catch.
  - CONV 72: `*(calling schedule_inspection with confirmed=false)*` — the model
    narrates the call instead of running it.

**Root cause.** `buildReply` in `agents.go` only drops a reply that *starts*
with `{`. These leaks start with `[TOOL_CALL]`, `<FunctionCall>`, or normal text
followed by a block, so the guard misses them. The model is describing the tool
call as text rather than letting the Go code run Phase 1. This is the same root
cause behind the false confirmations in Finding 2.

**Action taken.** None yet. We confirmed the `looksLikeJSON` guard does not catch
`[TOOL_CALL]`, `<FunctionCall>`, or an inline block. The scheduling fix made the
happy path work, but it did not remove these leaks.

**Whether it helped.** Open, and slightly worse in the new data. Recommended fix:
extend the sanity check in `buildReply` to also drop replies that contain
`[TOOL_CALL]`, `<FunctionCall`, `<|channel`, or a fenced JSON block, and
strengthen the scheduling prompt so the model never writes the tool call itself.

---

### Finding 4 — A short date like "July 15" sometimes gets the wrong year

**Observed.** When the user typed `July 15` and the model wrote the date itself
(in a leaked tool call), the year came out as **2025**, not 2026. The current
date is 2026-06-25, so July 15 should be 2026.

- **Examples:** CONV 36, 38, 48 — all show `2025-07-15`.
- **Contrast:** when the deterministic extractor in `intent.go` parsed the date
  (CONV 47, 52), the year was correct (`2026-07-15`).

**Action taken.** None directly. This bug only appears together with Finding 3:
the wrong year came from the model-generated tool call, not from the Go
`extractDates` function (which defaults a missing year to the current year).

**Whether it helped — looks fixed in the new data.** After the scheduling fix,
the grounded previews show the correct year: CONV 79, 85, 86, 87 all parse the
date to **2026-07-25** (from inputs like `25-07-2026` and `25/07/2026`). The
wrong year now only appears inside the leaked tool calls (Finding 3). So closing
Finding 3 will fully close this one too.

---

### Finding 5 — A procedure question phrased as "what do I do when…" gets a worse answer

**Observed.** Two very similar questions got very different answers:

- "What's the maintenance procedure for hydraulic pressure loss?" → **knowledge**
  agent → full, sourced answer (CONV 7, 8).
- "What do I do when hydraulic pressure drops?" → **general** agent → declined:
  "I cannot provide mechanical troubleshooting" (CONV 22).

**Root cause.** The RAG keywords in `intent.go` include `how to`, `how do i`,
and `procedure`, but not the phrase `what do i do when`. So the second question
scored as advisory and went to the general agent, which has no documents.

**Action taken.** None yet. Logged as a routing-coverage gap.

**Whether it helped.** Open. Recommended fix: add a few more procedural anchors
(for example `what do i do`, `what should i do`, `when … fails`) to the RAG
keyword group so this kind of question reaches the maintenance manuals.

---

### Finding 6 — "Why is X high-risk?" can return a score with no reason

**Observed.** The user asks **why** an elevator is high-risk, but the answer
shows only the level and score, with no explanation.

- **Examples:**
  - CONV 18: "Why is elevator 37180 flagged as high-risk?" → HIGH, score 1.00,
    **no explanation**.
  - CONV 19: 60503 → MEDIUM, 0.47, no explanation.
  - **Contrast:** CONV 3 (elevator 20718) returned a full explanation paragraph.

**Root cause.** This is expected data behavior, not a crash. `risk_explanation`
is only populated for some HIGH-risk elevators (see CLAUDE.md and
`generate_explanations.py`). When it is `NULL`, the data block has no reason to
show. The answer is **correct but not helpful** for a "why" question.

**Action taken.** None to the data. The honest answer is that no explanation was
generated for that elevator.

**Whether it helped.** Partly addressed by being honest. Recommended improvement:
when the user asks "why" and `risk_explanation` is `NULL`, add one line that says
no detailed explanation is on file for this elevator, instead of showing only a
bare score.

---

### Finding 7 — "Shut down by TSSA" mixes real shutdowns with follow-up cases

**Observed.** "Which elevators have been shut down by TSSA?" returns 20
elevators, but only 2 are real "Vol Shut Down"; the other 18 just need a
follow-up.

- **Examples:** CONV 1, 2, 13, 59.

**Why it is borderline, not wrong.** The answer always includes a note: "No
explicit shutdown flag exists in the database. Results show elevators with
non-passing most-recent inspection outcomes." So it is honest. But the word
"shutdown" can still mislead a quick reader.

**Action taken.** A small improvement already appears in later traffic: CONV 59
opens with "two are currently shut down (voluntary shutdown)," which separates
the real shutdowns from the follow-up cases.

**Whether it helped.** Yes, the clearer opening line helps. No code change is
required beyond keeping that framing. Marked as **no further action needed**.

---

### Finding 8 — One in four assistant turns failed because the LLM provider was rate-limited or unreachable

**Observed.** Many turns returned a graceful error instead of an answer:

- "The model is currently rate-limited…" — 18 turns.
- "I'm having trouble reaching the assistant…" — 21 turns.
- **Total: 39 of 158 assistant turns (25%) across all 87 conversations.**
- **Examples:** CONV 4, 11, 20, 23–31, 32, 33, 46, 48, 50, 62–64, 67–70, 73, 74.
- **Note from the second pass:** "rate-limited" (HTTP 429) dropped to 0 in the
  new conversations, but "trouble reaching" (provider unreachable) was still 9.
  So the load shifted, but the free-tier providers still drop requests.

**The good part.** The error handling from S3-9 worked. Users never saw a crash
or raw error text — only a plain message telling them to try again. Logging also
kept working: these failed turns are all recorded, which is exactly what
monitoring needs.

**Action taken.** A provider cascade was added so one provider failing does not
kill the turn: OpenRouter → Ollama `minimax-m2.5:cloud` → Ollama `gemma4:31b`
(commits `90549e6` and the `fix/llm-provider-cascade-jcj` work in `chat.go`).
A startup warm-up call was also added so the first request is not cold.

**Whether it helped.** Partly. The cascade and warm-up reduce hard failures, and
later conversations do complete (CONV 47, 59). But the free-tier providers still
rate-limit under bursts. This is a known limit of the free deployment, not a
code bug. See the deploy notes: paid tiers would remove most of these failures.

---

### Finding 9 — Users retry the same question during an outage

**Observed.** When a turn failed, users often re-sent the same message two or
three times.

- **Examples:** CONV 1 (same question 3×), CONV 10, CONV 32, CONV 33.

**Action taken.** None needed. This is normal user behavior during the provider
outages in Finding 8, not a separate bug. Fixing Finding 8 reduces it.

**Whether it helped.** Not applicable — no change made on purpose.

---

## Summary: what changed and what is still open

| # | Finding | Status |
| --- | --- | --- |
| 1 | Scheduling flow (preview → confirm → write) | **Fixed — verified** (3 bookings in 61–87) |
| 2 | General agent claims a write that did not happen | **Still open** (high priority; CONV 87) |
| 3 | Raw tool call / thinking tokens leak | **Open** (worse in new data) |
| 4 | Short date gets wrong year (2025) | **Looks fixed**, only leaks left (tied to #3) |
| 5 | "What do I do when…" misses the knowledge agent | **Open** |
| 6 | "Why high-risk?" with no explanation | **Open** (small improvement) |
| 7 | "Shut down by TSSA" framing | **No further action needed** |
| 8 | 25% of turns hit a provider failure | **Mitigated** (cascade + warm-up) |
| 9 | Users retry during outages | **No action** (symptom of #8) |

### What the second pass proved

The scheduling fix worked. The before/after is clear: **0 bookings in
conversations 1–60, 3 real bookings in 61–87** (IDs 9000006, 9000007, 9000008),
with clean previews, correct dates, and a working cancel. This is the full
iteration cycle this card asks for: observe → fix → re-test → confirm it helped.

### Top priorities for the next iteration

1. **Finding 2** — stop the false "scheduled" message. Make `yes`/`no` always
   return to the scheduling agent, and harden the general prompt so it never
   claims an action was done. This is the highest-risk open item: it still fired
   in the newest conversation (CONV 87).
2. **Finding 3 + 4 together** — stop the model from writing the tool call as
   text (`[TOOL_CALL]`, `<FunctionCall>`, JSON blocks). This removes both the
   leaked syntax and the wrong-year date.
3. **Finding 5** — add a few procedural keywords so "what do I do when…"
   questions reach the maintenance manuals.

These fixes will each get their own `feat/` branch and PR, per the project
branching rules.
