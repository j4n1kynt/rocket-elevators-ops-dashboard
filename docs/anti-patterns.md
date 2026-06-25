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

## [Team member 2 — add your name here]

> Add your 3–5 anti-patterns following the template above.

---

## [Team member 3 — add your name here]

> Add your 3–5 anti-patterns following the template above.

---

## [Team member 4 — add your name here]

> Add your 3–5 anti-patterns following the template above.
