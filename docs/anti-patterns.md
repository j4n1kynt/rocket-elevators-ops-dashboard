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

## Peter Narvaez (ppng-maker)

### 1. An `AllowedTools` field that didn't actually enforce anything

**What I did:** In the Sprint-3 router refactor I added an `AllowedTools` field to each agent and populated it in `router.go`, but the scheduling agent's Phase-1 guard in `agents.go` checked the tool with a hardcoded string comparison instead of consulting that field. I even added `TODO(S3-3)` markers (382fe1a) admitting enforcement wasn't wired, then opened the PR anyway.

**Why it was wrong:** The field *looked* like a security boundary but was inert — the router could set any `AllowedTools` value and the real gate ignored it. PR #58 flagged it as blocking. Anyone reading the code would assume the restriction was load-bearing when it wasn't.

**What I would do differently:** Don't ship a control that only looks enforced. Wire the field to the actual check (the `toolAllowed(allowed, name)` helper I added in 773e5d2) in the same PR, with a test proving a forbidden tool is rejected *via the field*, not a string literal. If enforcement has to wait, the field shouldn't exist yet.

### 2. Committing the 16 MB compiled `api.exe` binary

**What I did:** Pushed the compiled Go binary `api.exe` (~16 MB) into the repo, then removed it later in 74b9ff9 and added `*.exe` to `.gitignore` (b303a9d).

**Why it was wrong:** It bloats every clone permanently (it lives in history even after deletion), it's platform-specific build output, and it should never be tracked. The ignore rule should have existed before the first `go build`.

**What I would do differently:** Add build-output patterns (`*.exe`, `/api`) to `.gitignore` before building, and read `git status` for stray binaries before staging. A blind `git add -A` is how it got in.

### 3. Changing a model identifier in one place and missing the rest

**What I did:** Aligning the RAG embedding model, I updated `rag.py` from `all-MiniLM-L6-v2` (384-dim) to `bge-large` (1024-dim) in 19f2f8a — but a follow-up (8a3903b) had to fix three *more* stale references I'd missed: a docs table, a structure note, and a test still asserting shape `(384,)`. Same pattern with the Ollama tag: `minimax-m2.5` shipped before I corrected it to `minimax-m2.5:cloud` (a3ad3fa) across the default, tests, and docs.

**Why it was wrong:** A magic-string identifier (model name, vector dimension) lives in code, tests *and* docs. Fixing one path left the test validating against a model we no longer used, and the dimension mismatch produced meaningless similarity scores at runtime.

**What I would do differently:** Treat an identifier change as a repo-wide operation — `grep -r` the old value across code, tests, and docs, fix every hit in one commit, then run the test that exercises the real artifact (a 1024-dim assertion against the actual ChromaDB store) to confirm.

### 4. Round-tripping critical confirmation state through the client, then letting the model phrase the outcome

**What I did:** In the two-phase scheduling feature (FEATURE-4, 0c82bd8) I carried the signed `pending_action` only in a hidden HTML form field, and let the chat model phrase the confirmation and result text.

**Why it was wrong:** The browser dropped the hidden field across agent switches, so "yes" fell through to the general agent, which fabricated a fake *"scheduled successfully — Inspection ID …"* with no database write. The HMAC signature was fine; the *delivery* wasn't — and because the model wrote the success text, nothing tied the message to a real write. The worst kind of bug: silent false success on a write action.

**What I would do differently:** Never depend on the client to round-trip state that must survive — mirror it server-side keyed by `conversation_id` (the `pending_store.go` fix in be55373). For any action with a side effect, build the outcome message deterministically in Go from the actual write result, never from the LLM.

### 5. Debugging CI by pushing commits

**What I did:** Getting the AND-107 CI green took a string of one-line fixes pushed straight to the branch — pin numpy 2.1.3, then pin numpy 2.1.3 *again* in the separate MCP requirements file, bump uvicorn, set PYTHONPATH (f51b014, 6e43020, ff7fca2, 6669d08).

**Why it was wrong:** Each push burned a CI run to test a guess, and the duplicate numpy pin shows I fixed one requirements file without noticing a second needed the same change. Noisy history, slow feedback loop.

**What I would do differently:** Reproduce the CI environment locally (Python 3.10, a clean venv per requirements file) and resolve the dependency set in one pass before pushing. When pinning a version, grep for *every* requirements file first.

---

## [Team member 3 — add your name here]

> Add your 3–5 anti-patterns following the template above.

---

## [Team member 4 — add your name here]

> Add your 3–5 anti-patterns following the template above.
