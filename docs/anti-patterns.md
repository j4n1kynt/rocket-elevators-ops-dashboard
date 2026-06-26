# Anti-Patterns — Rocket Elevators Operations Dashboard

**Instructions for each team member:**
Each developer must add their own section below following the template. Write 3–5 anti-patterns you introduced in your own work, explain what went wrong, and describe what you would do differently. Be specific — name the file and the decision, not a generic rule.

Template:
```
## [Your name]

### 1. [Short name for the anti-pattern]
**What I did:** …
**Why it was wrong:** …
**What I would do differently:** …
```

---

## Juan Camilo Janica (jcj)

### 1. Tailwind CDN without typography plugin — using `prose` classes that generated no CSS

**What I did:** I added `prose prose-sm max-w-none` to the assistant chat bubble divs, expecting the Tailwind typography plugin to style lists, headings, and paragraphs inside markdown-rendered HTML.

**Why it was wrong:** The project uses the Tailwind CDN build (`<script src="https://cdn.tailwindcss.com">`). The CDN build does not include the optional typography plugin. The `prose` classes compile to zero CSS rules, so all styling was silently ignored. Tailwind Preflight also stripped the browser's default list bullets, heading sizes, and paragraph margins, leaving the HTML unstyled.

**What I would do differently:** Check whether the Tailwind build includes the plugins the classes require before using them. When the typography plugin is not available, write scoped CSS rules directly (`.chat-md ul`, `.chat-md ol`, etc.) in the `<style>` block of `layout.html`, which is what we ultimately did.

---

### 2. MCP timeout asymmetry — scheduling agent at 10 s while every other agent used 25 s

**What I did:** When I implemented the scheduling agent, I set its MCP call timeout to 10 seconds (`10*time.Second`) while the data and knowledge agents already used 25 seconds.

**Why it was wrong:** The MCP server on Render free tier takes 30–60 seconds to cold-start. A 10 s timeout guaranteed failure on any first scheduling request after the server had been idle, producing "Failed to reach the assistant" errors that looked like code bugs instead of infrastructure behaviour. The asymmetry also made the system unpredictable — the same server was allowed 25 s for RAG queries but only 10 s for scheduling.

**What I would do differently:** Define a single constant for the MCP call timeout (25 s) at the top of `agents.go` and reference it everywhere, so no agent accidentally gets a different budget. Also document the Render cold-start constraint in a comment alongside the constant so the next developer understands why the number is generous.

---

### 3. Machine-specific scratch files in the shared `.gitignore`

**What I did:** I added developer-specific files (`test_stress.py`, `api_out.txt`, `api_err.txt`, `audit-and-107.txt`, etc.) to the project's shared `.gitignore`.

**Why it was wrong:** The shared `.gitignore` is team policy — adding personal scratch files there is noise for every other developer. More importantly, ignoring `test_stress.py` by pattern in the shared file means any teammate who later writes a legitimate test file with a similar name would have it silently excluded from git. Personal ignores belong in the global `~/.gitignore_global` so they never affect other contributors.

**What I would do differently:** Use `git config --global core.excludesFile ~/.gitignore_global` once, and keep all personal scratch patterns there.

---

### 4. `raise_for_status()` without prior 500 handling

**What I did:** In `platform/server.py`, the Flask `/chat` route called `api_resp.raise_for_status()` directly after receiving the Go API response. When the Go API returned a 500 with a structured JSON error body, `raise_for_status()` raised a `requests.HTTPError`, which fell through to the generic `except Exception` handler and showed the user "Failed to reach the assistant" — discarding the actual error detail.

**Why it was wrong:** A Go-level panic or validation failure returns HTTP 500 with `{"error": "..."}`. That detail is useful to the user ("invalid date", "elevator not found") but was thrown away by the exception path. The user saw a generic message instead of actionable information.

**What I would do differently:** Always inspect 500/503 status codes before calling `raise_for_status()`. Extract the `error` field from the JSON body and surface it directly when present, so the user sees the real reason rather than a generic fallback.

---

### 5. Applying `_render_reply()` inconsistently across three templates

**What I did:** When I introduced the `_render_reply()` function (markdown → HTML conversion), I applied it in `_chat_reply.html` and `_conversation_reply.html` but missed `_conversation_thread.html`, which renders the same assistant messages from the stored history.

**Why it was wrong:** Historical messages (viewed when opening a past conversation) were displayed as raw markdown text, while new messages in the same conversation were rendered as HTML. The same reply had two different visual representations depending on which code path rendered it.

**What I would do differently:** Any time a display function is responsible for rendering a type of content, locate every template that renders that same content type before shipping the change. A grep for `m._html` or `reply_html` would have immediately found all three call sites.

---

## Emmanuel Rendon (erg)

### 1. Committing a temporary diagnostic log to a shared branch

**What I did:** In `platform/server.py` I added a `print()` debug log to the `/chat` route to see the raw `pending_action` the browser sent. I committed it as its own commit (`dd5c4ab`) with the note "temporary — to be removed once the root cause is fixed."

**Why it was wrong:** Debug print code does not belong in a commit on a shared branch. It adds noise to the file and to the git history. If I forgot to remove it, it would print user data on every chat turn in production. A "remove later" promise in a commit message is easy to forget.

**What I would do differently:** Keep debug logging only on my own machine and never commit it. If I really need logs in the code, use the `logging` module at a debug level that is off by default. Find the root cause first, then commit only the real fix.

---

### 2. The same feature worked in one chat path but was missing in the other

**What I did:** The chat widget (`/chat`) round-trips `pending_action`, so two-phase scheduling works there. When I built the conversations-page thread chat (`/conversations/<id>/message`) for S3-8, I did not add the same `pending_action` round-trip.

**Why it was wrong:** The thread chat sent the user's "yes" to the API without `pending_action`. The router then read "yes" as advisory, so Phase 2 never ran and no inspection was saved. Worse, the advisory agent sometimes made up a false "scheduled" reply. The same feature worked in one path and broke silently in the other.

**What I would do differently:** When a feature depends on a request field (here `pending_action`), list every endpoint that handles that field before shipping. A quick grep for `pending_action` would have shown that only one of the two chat paths handled it.

---

### 3. A prompt that told the model to "call" the tool

**What I did:** In `platform/api/prompts/scheduling_prompt.md` I told the model to "call" the `schedule_inspection` tool. But in our design, Go calls the tools and injects the result into the prompt. The model only narrates the result.

**Why it was wrong:** The prompt did not match how the system really runs tools. The model tried to "call" the tool by writing raw `[TOOL_CALL]` markup, and that markup leaked into the user reply. My first fix also added code in `agents.go` to strip the markup, which treats the symptom, not the cause.

**What I would do differently:** Write the prompt to match the real execution model — narrate the injected result, never call a tool. Keep the markup-stripping only as a small safety net, not as the main fix.

---

### 4. The date parser only handled one format

**What I did:** The scheduling flow parsed only ISO dates (`YYYY-MM-DD`). Dates like `25-07-2026` (DD-MM-YYYY) were not parsed, so Phase 1 never built a `pending_action`.

**Why it was wrong:** Real users type dates in many formats. When the date did not parse, the next "yes" fell through to the general agent and nothing was scheduled. This is a happy-path assumption for an input that gates a database write.

**What I would do differently:** Support the common date formats up front (DD-MM-YYYY, MM-DD-YYYY, and slash variants), and add a test for each one. For an action that writes to the database, I should plan for messy input from the start.

---

### 5. A keyword cue that was too broad

**What I did:** In the RAG routing I added a bare `"incident"` cue to `incidentNarrativeCues`. The goal was to send incident questions to the incident-narrative corpus.

**Why it was wrong:** The cue was too broad. A procedural question that only mentions an incident (for example, "how do I report an incident") was routed to the narrative corpus instead of the maintenance manuals. A reviewer caught this in the PR (`2816f93`).

**What I would do differently:** Pick cues that really separate the two corpora, not common words. Add a negative test case — a procedure question that mentions "incident" — before shipping, so a too-broad cue fails the test.

---

## [Team member 3 — add your name here]

> Add your 3–5 anti-patterns following the template above.

---

## [Team member 4 — add your name here]

> Add your 3–5 anti-patterns following the template above.
