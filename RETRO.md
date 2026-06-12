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
