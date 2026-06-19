# Sprint Retrospective — Chatbot (CHAT / EVAL / DESIGN)

**Date:** 2026-06-12

## What Went Well

- **Spec-driven discipline held.** The OpsBot prompt was defined and stress-tested (EVAL-1) before a single line of UI was written — the evaluation log became the decision record that resolved every later design dispute cleanly.
- **Widget shipped in full.** The CHAT-2 feature landed with the FAB overlay, suggested chips, HTMX-driven history, and a warm-up goroutine in one PR cycle. No deferred polish items.
- **Prompt evaluation caught real gaps early.** EVAL-1 surfaced two boundary failures (identity override, out-of-scope leakage) against the chosen model before any user ever touched the widget — knowing the limits upfront kept expectations honest.

## What Didn't Go Well

- **Three implementation iterations for one feature.** The chatbot was built once with a data-grounded approach, then reworked as a hybrid, then reverted to strict advisory-only. The direction change cost a full extra day and left residual complexity in git history.
- **Ollama model instability blocked live testing.** The GGML crash on the dev machine meant the widget was never validated end-to-end in a running environment before the PR merged — the gap between "code compiles" and "chatbot actually responds" was wider than expected.

## Action Item for Next Sprint

**Connect the local AI model to live elevator data.** Re-introduce the intent detection and DB context-fetch layer in `chat.go` so OpsBot can answer questions about real fleet state — current risk counts, specific elevator status, overdue inspections — pulled directly from PostgreSQL at query time. This requires a new eval pass against `docs/chat-design/chatbot_design_doc.md` to validate that grounded responses stay accurate and that live-data boundaries (what the model says vs. what the DB actually contains) hold under stress.

---

# Sprint 2 Retrospective — Chatbot Data & Knowledge (AND-107)

**Date:** 2026-06-19

## What Went Well

- **We built the data layer in a clean way.** Instead of putting the database logic straight into the chat code, we built a separate tool server (MCP) and connected the Go API to it. The chatbot reads live data through these tools, and we can use the same tools while we develop.
- **Many features shipped and work end to end.** The chatbot answers the five required questions about elevators, inspections, and incidents. It also looks up risk levels, searches the maintenance manuals and incident history, and can schedule an inspection after the user confirms.
- **Answers point to their source.** Every answer that uses real data says where it comes from. Our tests showed the data answers were correct, and the chatbot does not invent facts when the data is missing.
- **We deployed and tested it live.** We replaced the local model with a free online provider, and the chatbot now runs on the public dashboard. In Sprint 1 we could not test it live; this time we did.
- **Our test tool found real problems.** We used Claude as a judge to check the answers. It found a real bug and even found a mistake in our own test setup, which we then fixed.

## What Didn't Go Well

- **Some data was missing in the live database.** When a user asks "why is this elevator high-risk?", the chatbot cannot give the real reasons because that information was not loaded into the deployed database. The chatbot acts correctly and does not guess, but the answer is still incomplete. We only noticed this during testing.
- **The free server was slow to wake up.** When the server had been idle, the first search was too slow and timed out, so the chatbot wrongly said it had no information. This problem only showed up sometimes, so it was hard to catch early.
- **The chatbot sometimes routes questions to the wrong place.** Some questions go to the wrong data source, so the chatbot misses information that actually exists. Also, when an elevator ID is wrong, the chatbot gives a vague reply instead of saying the ID was not found.

## Sprint 1 Follow-Through

- **The Sprint 1 action item was:** connect the chatbot to live elevator data, so it can answer about real fleet state, and check it with a new evaluation pass.
- **What we did about it this sprint:** We did this, but in a stronger way than first planned. Instead of adding the data logic directly into the chat code, we built a tool server and a router that sends each question to the right place. The chatbot now reads live data from the database and the document search. We then ran the planned evaluation, using Claude as a judge.
- **What changed:** The chatbot now answers about real, current data — shutdowns, inspections, incidents, and risk — and it shows its source. It is deployed and tested end to end, not only on a laptop. The evaluation helped: it found the slow-server bug and made the missing-data problem visible early. So yes, the follow-through helped, and it gave us a cleaner, reusable design.

## Action Item for Next Sprint

**Make the chatbot answers clearer and the input more flexible.** First, the chatbot should reply in a more organized format (for example, clear sections, short lists, or steps) so the answers are easier to read. Second, the chatbot should accept more general input instead of forcing one exact format. For example, today if the user does not write the date in the exact `YYYY-MM-DD` form, the chatbot will not schedule the inspection; it should understand normal date wording (like "next Tuesday") and still work.
