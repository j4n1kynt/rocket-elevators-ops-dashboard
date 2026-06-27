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

---

# Sprint 3 Retrospective — Multi-Agent Chatbot (AND-108 / S3)

**Date:** 2026-06-26

## What Went Well

- **All PBIs delivered.** The sprint closes with no carry-over. Every feature defined for Sprint 3 — four agents, 2-phase scheduling, conversation logging, analytics page, error handling, automated test suite, agent evaluation, and conversation monitoring — shipped and is tested.
- **Scheduling works end to end.** Three real inspections were booked in production (IDs 9000006, 9000007, 9000008) in conversations 61–87. In conversations 1–60, before the fixes, zero bookings succeeded. The before/after is clear and verified.
- **CI workflows on every PR.** GitHub Actions (Go build, vet, test + Python deps check) ran on every branch. It gave the team a shared signal before review, made the review process faster, and prevented regressions from landing silently on `dev`.
- **Communication stayed clean across 79 PRs.** Trello kept work visible, issues were raised in PR threads before becoming blockers, and every fix had a regression test and at least one reviewer before merge. No ambiguity surfaced late.
- **Error handling held.** Users never saw raw Go error strings, MCP JSON, or crash traces across 87 logged conversations. Every failure path — LLM timeout, MCP server down, bad scheduling input — returned plain language.

## What Didn't Go Well

- **The scheduling agent needed three separate fix PRs before it was stable.** Type routing, Phase 2 resilience, and LLM rate-limiting were all found after the feature was considered done. Each required its own branch, test, and review cycle.
- **LLM rate limiting hit 25% of turns at peak.** The free-tier OpenRouter model dropped roughly one in four requests during team testing. A 3-tier cascade (OpenRouter → Ollama `minimax-m2.5:cloud` → Ollama `gemma4:31b`) reduced hard failures, but free-tier limits remain a constraint in production.
- **The general agent still sometimes claims a scheduling write that never happened.** When a "yes" lands on the general agent instead of the scheduling agent, the general agent occasionally confirms the inspection as booked — even though it has no tools and wrote nothing. This fired again in the newest conversation (CONV 87) after the fixes. It is the highest-priority open item.
- **Tool call leaks got worse after the scheduling fix.** The model sometimes prints `[TOOL_CALL]` or `<FunctionCall>` as plain text instead of letting Go run the tool. This happened 3 times in conversations 1–60 and 8 times in 61–87. The leak increased because the scheduling fix brought more scheduling traffic, and the prompt still tells the model to "call" the tool in a way that does not match the actual execution model.

## Sprint 2 Follow-Through

- **The Sprint 2 action item was:** make chatbot answers clearer and the input more flexible — specifically, support natural date wording instead of requiring `YYYY-MM-DD`, and improve reply formatting with clear sections and short lists.
- **What we did about it this sprint:** We addressed both parts. On input flexibility, the date parser in `intent.go` was extended to handle `DD-MM-YYYY`, `DD/MM/YYYY`, slash variants, and natural phrases. The scheduling agent now also accepts the inspection type by name mid-conversation, not only as part of the original message. On formatting, Go-side deterministic formatters (`data_format.go`) produce structured key-value blocks for every data-agent response, so the LLM writes only a one-sentence intro over grounded data rather than generating free-form numbers. The agent evaluation (PR #78) also fixed a formatShutdown regression that conflated voluntary shutdowns with follow-up cases, and enforced uppercase risk level labels.
- **What changed:** Date input is now flexible enough that users can type dates in common formats and the scheduling flow proceeds. The conversation monitoring confirmed this: the wrong-year bug (`2025` instead of `2026`) only appears inside leaked tool calls now, not in normal Go-parsed paths. Data answers are consistently structured and grounded. So yes, the follow-through delivered what Sprint 2 defined, and it surfaced the deeper issue (leaked tool calls) that is now the next open item.

## Program Reflection

The thing that surprised us most across the three sprints is how much the **quality of what we gave the AI determined the quality of what it produced** — and how long it took us to fully internalize that.

In Sprint 1 we expected the AI to figure things out. It did, but it diverged in ways we had to undo. In Sprint 2 we started writing specs before prompting, and the quality gap between prompted-with-spec and prompted-without became obvious: the AI stopped hallucinating design decisions and started following the path we laid out. By Sprint 3 we were using separate sessions for implementation, testing, PR review, and auditing — and each one was more focused and faster than anything we did in Sprint 1.

The other surprise was where the human role actually sits. We expected to spend most of our time writing code. Instead we spent it writing specifications, testing running behavior the AI could not observe, and formulating the right question when the AI was stuck. The pending-action bug was only found because a developer ran both chat paths manually and told the AI exactly what to compare — at which point the AI found the root cause in seconds. The AI is fast at reasoning; the developer is the one who can observe the system at runtime. That division of labor was not obvious before we lived it.
