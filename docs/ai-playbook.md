# AI Playbook — Rocket Elevators Operations Dashboard

This document summarizes what the team learned using AI tools (Claude Code, OpenRouter, GitHub Copilot) throughout the project. It is a 1–2 page team reference for future sprints and for developers who take over the project.

**Instructions for each team member:**
Each developer must add their own section under "Individual contributions" following the template. Describe 2–3 specific ways AI tools helped you and 1–2 pitfalls you ran into. Be concrete — name the task, not a generic statement.

---

## What worked well (team-level)

### Using AI as a root-cause analyst, not just a code writer
The most effective use of Claude Code in this project was asking it to diagnose bugs before writing any fixes. For every chatbot issue (formatting, timeouts, routing failures), we first asked for a root-cause analysis with file paths and line numbers. This produced accurate fixes on the first try and avoided the usual pattern of applying a fix that treated a symptom instead of the cause.

### Generating deterministic code with AI, then verifying with tests
The intent classifier (`intent.go`) and the data formatters (`data_format.go`) were both designed with AI help but kept entirely deterministic — no ML, no randomness. This made the AI-generated code easy to verify: we wrote unit tests for every keyword case and every formatter, and the tests became the ground truth. AI wrote the test scaffolding; we added the specific cases; the tests caught several edge cases the AI had missed.

### Pre-PR audits as a structured review step
Before opening any PR, we asked Claude Code to audit the change against a checklist (security, Tailwind compatibility, import hygiene, gitignore policy). This caught issues like the `prose` class problem and the duplicate `import re` before they reached code review. Treating AI audit as a required step before human review reduced back-and-forth on the PR significantly.

### AI for infrastructure and boilerplate
Docker Compose configuration, GitHub Actions CI workflow, Go Dockerfile, database migration SQL — AI produced correct first drafts of all of these. The team reviewed and adjusted, but the scaffolding was solid enough to use without a full rewrite.

---

## Pitfalls we ran into (team-level)

### AI is confident even when it is wrong about the environment
Claude Code suggested using `prose prose-sm` Tailwind classes without checking whether the typography plugin was available. The suggestion was technically correct in a full Tailwind setup but wrong for the CDN build we were using. **Lesson:** always verify environment assumptions (package versions, build tool plugins, platform constraints) before accepting AI suggestions that depend on them.

### Long context windows cause drift
In long sessions, Claude Code sometimes applied a fix to a file it had read 30 turns earlier without re-reading the current state. This led to a case where an import was added to a stale version of `server.py`. **Lesson:** for multi-file changes across a long session, ask the AI to re-read the target file immediately before editing it.

### AI tends to add error handling that is never reachable
Several AI-generated code paths included `if err != nil` guards for errors that could not actually happen given the surrounding logic. This added noise without adding safety. **Lesson:** after accepting AI-generated error handling, ask "can this branch actually be reached?" If not, remove it.

---

## Individual contributions

### Juan Camilo Janica (jcj)

**What AI helped with:**

- **Debugging the chatbot formatting and timeout issues**: I described the two symptoms (markdown not rendered, scheduling agent failing) and Claude Code produced a root-cause analysis naming the exact files, lines, and mechanisms — `_render_reply()` missing mistune, the 10 s vs 25 s timeout, and the missing 500 handler before `raise_for_status()`. All three fixes were accurate on the first attempt.
- **Writing the MCP Streamable HTTP client (`mcp_client.go`)**: the JSON-RPC 2.0 handshake (initialize → notifications/initialized → tools/call) and SSE response parsing are protocol-specific and tedious to write from scratch. Claude Code produced a correct implementation that handled both `application/json` and `text/event-stream` response types, which I then verified against the MCP spec.
- **Structuring the scoped `.chat-md` CSS**: after the `prose` class problem was identified, Claude Code proposed the scoped CSS approach with the exact rule set needed to restore Tailwind-Preflight-stripped defaults without affecting other parts of the page.

**Pitfalls I ran into:**

- **Accepting a CSS class suggestion without checking the build**: I added `prose prose-sm` based on Claude Code's suggestion before verifying that the typography plugin was in our CDN build. This caused a silent visual regression that only appeared after deployment. **Fix**: check the Tailwind CDN URL or plugin list before using typography classes.
- **Letting AI write personal files into the shared `.gitignore`**: Claude Code added my local scratch file names to the team's `.gitignore` without me noticing. This is correct from its perspective (keep the working directory clean) but wrong from a team policy perspective. **Fix**: review `.gitignore` changes carefully and redirect personal ignores to `~/.gitignore_global`.

---

## [Team member 2 — add your name here]

> Add your individual AI playbook section following the template above.
> Include: 2–3 things AI helped you with, and 1–2 pitfalls you hit.

---

## [Team member 3 — add your name here]

> Add your individual AI playbook section following the template above.
> Include: 2–3 things AI helped you with, and 1–2 pitfalls you hit.

---

## [Team member 4 — add your name here]

> Add your individual AI playbook section following the template above.
> Include: 2–3 things AI helped you with, and 1–2 pitfalls you hit.
