# MCP Development Workflow

**Card:** AND-107 — MCP Server and Chatbot
**Period:** 2026-06-16 → 2026-06-19
**Scope:** `platform/mcp/` (FastMCP server, tools, RAG layer), the Go API chat
handler that consumes it (`platform/api/`), and the deployment work that put
both on Render.

This document is the story of how the MCP server came to be: the order the work
happened in, the decisions we made and why, the things that broke along the way,
and what we had to change as a result. It is deliberately written in plain
language. Where a topic has its own focused reference doc, this file links to it
rather than repeating the detail.

> **Companion docs** (the "how it works today" references):
> - `platform/mcp/mcp docs/scripts_explanations.md` — what every file in the server does
> - `platform/mcp/mcp docs/asyncpg_pool_implementation.md` — the connection pool
> - `platform/mcp/mcp docs/input_models.md` — the Pydantic validation models
> - `platform/mcp/mcp docs/rag_search_tool.md` — the maintenance-doc search tool
> - `platform/mcp/mcp docs/incident_date_anchor_fix.md` — the "last year" date fix
> - `docs/schedule_inspection.md` — the write action, end to end
> - `docs/mcp-security-test.md` — the security audit

---

## 1. What we set out to build

Before AND-107, the project already had a chatbot widget in the dashboard (built
under CHAT-1, CHAT-2, DESIGN-1 and EVAL-1). It was a conversational assistant —
"OpsBot" / the Fleet Assistant — but it could only talk. It had no access to the
live fleet data sitting in PostgreSQL, and no access to the maintenance manuals.
Ask it "how many incidents did we have last year?" and it would either decline or
invent an answer.

AND-107 was the work that gave the chatbot hands. The plan was an **MCP (Model
Context Protocol) server**: a separate process that exposes a fixed catalogue of
*tools* — `get_fleet_stats`, `get_elevator_risk`, `search_maintenance_docs`,
`schedule_inspection`, and so on — that the chatbot's language model can call.
The model decides *which* tool to call from the user's question; the MCP server is
the only thing that actually touches the database or the document index, and it
validates every input before it does.

The architecture we landed on:

```
Browser (HTMX chat widget)
  │  POST /chat
  ▼
Flask  platform/server.py            ← thin proxy, holds no session
  │  forwards JSON
  ▼
Go API  platform/api/chat.go         ← classifies intent, calls the LLM,
  │                                     decides which MCP tool to invoke
  │  MCP JSON-RPC over HTTP (mcp_client.go)
  ▼
MCP server  platform/mcp/server.py   ← FastMCP, 10 validated tools
  │  asyncpg (parameterized SQL)        ChromaDB (semantic search)
  ▼
PostgreSQL + ChromaDB
```

Two design rules drove almost every decision:

1. **The MCP server is the trust boundary.** Tool inputs arrive from a language
   model, which is effectively untrusted user input. Every tool validates and
   sanitises before touching a data store — exactly as a public web API would.
2. **The model picks the tool; the server owns the data.** The LLM never writes
   SQL and never sees raw credentials. It chooses a tool and supplies arguments;
   the server does the rest.

---

## 2. How the work was sequenced

The work broke naturally into three layers, and we built them bottom-up:
**FOUNDATION** (the data and the server skeleton), then **FEATURE** (wiring the
chatbot to actually use the tools), and **DEPLOY** (getting it onto Render). They
overlapped in practice, but the dependency order held.

| Phase | What it delivered | PRs |
|---|---|---|
| FOUNDATION-1 | RAG preprocessing — chunk and embed maintenance docs into ChromaDB | #27, #28 |
| FastMCP skeleton | Server structure, env-based Postgres config | #29 |
| asyncpg migration | Replace psycopg2 with an async pool + startup health check | #30 |
| Data query tools | The five inspection/incident read tools | #31 |
| FOUNDATION-3 | `search_maintenance_docs` — similarity filtering, source attribution, tests | #32, #33, #34 |
| FOUNDATION-4 | Intent classifier + routing in the Go API | #35 |
| FEATURE-1 | MCP HTTP client in Go, wire routing into the chat handler | #36, #37, #38 |
| DEPLOY-1 | Replace Ollama with OpenRouter; containerize the MCP server | #40, #41 |
| FEATURE-2 | Risk-assessment phrasing + prompt hardening | #42 |
| DEPLOY-2 | Split dashboard vs. pipeline requirements | #43 |
| FOUNDATION-1b | Semantic search over incident narratives | #44 |
| FEATURE-4 | `schedule_inspection` — the two-phase write action + audit trail | #47 |
| Timeout / fallback fixes | Raise MCP timeout to 25s; advisory RAG fallback | #48 |

The rest of this document follows that arc, but groups it by *theme* rather than
strictly by PR, because several of the most interesting lessons cut across
multiple PRs.

---

## 3. FOUNDATION: the data layer and the server skeleton

### 3.1 Getting documents into ChromaDB (FOUNDATION-1)

The first foundation task was preprocessing: take the maintenance documents,
split them into overlapping chunks, embed each chunk into a vector, and store the
vectors in a ChromaDB collection (`maintenance_documents`) so the chatbot can do
semantic search later. The chunking settled at **500 tokens with 50-token
overlap**, cosine distance, and a single embedding model used on both sides
(write and query).

A subtle but important detail surfaced here and stayed with us: **the embedding
model and its settings must be identical at index time and at query time.** If
preprocessing embeds with one model and the query tool embeds with another, the
vectors live in different spaces and the similarity scores are meaningless. This
came back to bite us later (see §6.3), so it's worth flagging early.

> **Source-format note.** The pipeline was designed around `.txt` source files,
> not PDFs. As a result `MAINTENANCE_SOURCE_TYPE` in `rag_tools.py` is left as
> `None` (no metadata filter — it searches all indexed docs) until the `.txt`
> ingestion path writes a `source_type` value to filter on. This is tracked in
> the memory note on RAG source format.

### 3.2 The FastMCP skeleton (PR #29)

Next came the server itself: a `FastMCP` app in `server.py` that registers each
tool by passing the function to `mcp.tool()`, plus environment-variable-based
PostgreSQL configuration (`DATABASE_URL` first, falling back to individual `DB_*`
vars — the same precedence the ETL script and the Go API already used).

### 3.3 The asyncpg migration — the first real architectural correction (PR #30)

The skeleton initially used **psycopg2** with a `ThreadedConnectionPool`. It
worked in isolation, but it was the wrong driver for this server. FastMCP runs on
**uvicorn, a single-threaded async event loop.** psycopg2 is synchronous, so
every database call blocked that loop — meaning that even though the server
*looked* concurrent, tool calls actually queued up one behind another, each one
just waiting on the network while holding the whole loop hostage.

We rewrote `db.py` around **asyncpg**, a native-async PostgreSQL driver that
integrates with the event loop directly. With it, one tool can be waiting on its
query while another is already processing results — real concurrency on a single
thread. The full rationale is in `asyncpg_pool_implementation.md`; the
highlights:

- The pool is created **once at startup** via FastMCP's `lifespan` hook, not lazily
  on first use. So the old thread-safe double-checked-locking dance went away — by
  the time any tool runs, the pool already exists.
- `init_pool()` runs a `SELECT 1` **startup health check.** If PostgreSQL is
  unreachable, the server refuses to start with a clear error, instead of dying
  silently on the first tool call thirty seconds later. Fail fast, fail loud.
- Converting the tools was mechanical but total: `def` → `async def`, `with` →
  `async with`, psycopg2 cursors → asyncpg's `fetch`/`fetchrow`/`fetchval`/
  `execute`, and `%(name)s` placeholders → positional `$1, $2`.

This migration also flushed out four pre-existing problems that had nothing to do
with asyncpg — they're documented in §6.1 because they're a good lesson in
"the documented command had never actually been run."

### 3.4 Input validation: from a helper module to Pydantic models

Validation went through two designs. The first was `tools/_validators.py` — a
handful of pure functions (`validate_elevator_id`, `validate_limit`, …) that every
tool called inline before touching a data store. It worked, and the security audit
was written against it.

We then **replaced it with per-tool Pydantic models** (`tools/models.py`,
`ConfigDict(strict=True)`). The reasons:

- **Strict mode kills type coercion.** This matters most for the `confirmed`
  boolean on the write tool — in strict mode, `"yes"` and `1` are rejected at the
  type layer and never reach the function body. The model *is* the gate.
- **One model per tool, with named `@field_validator` methods.** The first cut of
  the models leaned on `Field(gt=0, le=9_999_999)` constraints, but those produce
  generic Pydantic error messages. We moved every rule into named validators that
  raise `ValueError` with a precise, human-readable message — which matters
  because those messages get surfaced to the user through the chat handler.

One Python footgun is called out explicitly in both the models and the audit:
`isinstance(True, int)` is `True`, because `bool` subclasses `int`. So elevator-ID
validation checks `isinstance(v, bool)` *first* and rejects it — otherwise `True`
would sail through as elevator ID 1. Details in `input_models.md`.

---

## 4. The data query tools

PR #31 added the five core read tools (`get_tssa_shutdown_elevators`,
`get_inspection_history`, `get_elevators_needing_followup`, `get_elevator_risk`,
`get_fleet_stats`) plus the two incident tools. Two decisions from this batch are
worth recording.

### 4.1 Every tool returns a JSON error object — it never crashes the call

We wrapped every tool body in `try/except` that returns
`{"error": true, "message": ...}` instead of letting an exception propagate out of
the MCP call. A tool that throws is a tool the chatbot can't reason about; a tool
that returns a structured error is one the handler can detect and route around.

There's one deliberate exception to "catch everything": **`ValidationError` is
re-raised before the broad `except Exception`.** If the broad handler swallowed it,
invalid input would quietly come back as an error dict instead of failing
validation — which would defeat the whole point of the validation layer and also
made the validation tests pass when they shouldn't. This exact bug bit us twice
(once in FOUNDATION-3, once in FEATURE-2) — see §6.4.

### 4.2 The "last year" tool that always returned zero (the date anchor fix)

`get_incident_count_last_year` originally computed its window relative to
`CURRENT_DATE`. The trouble: **the incident dataset only spans 2011–2016.** Asking
for "last year" relative to today (2026) produced an empty window — so the tool
returned a perfectly valid JSON structure with every count at zero. It looked like
it worked. It was wrong on every call.

The fix anchors the window to a hardcoded date, `2016-11-22` (the latest
`date_of_occurrence` in the table), so "last year" resolves to **2015** — the most
recent *complete* calendar year of data. After the fix it returns 506 incidents,
2 fatal, 142 injury. The full reasoning, and the note that the anchor is hardcoded
and will need updating if new data ever lands, is in `incident_date_anchor_fix.md`.
The lesson: **a tool returning a well-formed empty result is not the same as a
tool working.** We only caught this by checking the actual numbers, not the shape.

---

## 5. RAG search and connecting the chatbot

### 5.1 `search_maintenance_docs` (FOUNDATION-3)

This is the semantic-search tool. It embeds the query, asks ChromaDB for the
nearest chunks, converts each cosine distance to a similarity score
(`similarity = 1 - distance`), and **drops anything below a 0.5 threshold.** That
threshold is the relevance arbiter — it's what stops the chatbot grounding an
answer in a chunk that merely happens to be the *least irrelevant* of a bad batch.
The tool returns one consistent response shape across all branches (results,
no-confident-matches, no-results) so the consumer never hits a `KeyError`. Full
contract in `rag_search_tool.md`.

A couple of decisions here:

- **Filtering lives in the tool, not in `rag_query`.** `rag_query` is a
  general-purpose helper; the 0.5 threshold is a policy specific to this tool and
  might differ for a future one.
- **Tests mock `rag_query`.** The suite controls distance values directly, so it's
  fast, deterministic, and doesn't need a populated ChromaDB in CI. What's under
  test is the filtering and messaging logic, not ChromaDB itself.

### 5.2 Intent classification and routing (FOUNDATION-4 → FEATURE-1)

With tools on the server, the Go API needed to decide *when* to call them. We chose
a **deterministic, rule-based classifier** (`platform/api/intent.go`) — regex and
keyword matching, no LLM — so the same input always routes the same way and entity
extraction (elevator IDs, dates, inspection types) is reproducible.

FEATURE-1 then built the plumbing:

- **`mcp_client.go`** — a Go HTTP client that opens a fresh MCP session
  (`initialize` → `notifications/initialized` → `tools/call`) per request and
  handles both `application/json` and `text/event-stream` responses.
- **Wiring in `chat.go`** — classify the message, build tool arguments from the
  classification and extracted entities, call the tool, and **inject the result
  into the system prompt as a "Live Data Context" block** so the LLM answers from
  real data. On any MCP error, it falls back gracefully to advisory mode.

That wiring needed hardening almost immediately (PR #37, `harden MCP routing`),
which is its own lesson — see §6.5.

---

## 6. What went wrong, and what we changed

This is the part worth keeping. Most of these were not "bugs in the feature" —
they were assumptions that turned out to be false, and the fixes are the kind of
thing that's easy to forget once it works.

### 6.1 The documented run command had never actually worked

When we first tried `py -3 -m platform.mcp.server`, it failed — and it turned out
the command in the README had *never* run. Three layered problems:

1. **No `platform/__init__.py`.** Python found the standard-library `platform`
   module before the local `platform/` directory, so the package import failed
   with `'platform' is not a package`.
2. **Adding an empty `platform/__init__.py` broke the opposite way.** Now the local
   package *shadowed* the stdlib `platform` module, and third-party code deep in
   FastMCP's dependency tree (e.g. `attrs` calling `platform.python_implementation()`)
   blew up with `AttributeError`. The fix was a `__getattr__` hook in
   `platform/__init__.py` that lazily loads the *real* stdlib module (searching
   `sys.path` while skipping the current directory) and proxies attribute lookups
   to it.
3. **`pip install` and `py -3 -m pip install` targeted different interpreters.**
   The first install went into a different Python environment than `py -3`
   resolves to, so the packages "weren't there" at runtime. Re-running with
   `py -3 -m pip install` fixed it.

**Lesson:** a command in the docs is a hypothesis until someone runs it on a clean
machine.

### 6.2 Python 3.14 had no wheels for the heavy packages

The only Python on the dev machine was 3.14, released recently enough that
`chromadb` and `sentence-transformers` had no prebuilt wheels for it. `rag.py` had
**top-level imports** of both, so the *entire server* failed to start — even for a
DB-only tool call that never touched RAG. We moved those imports *inside* the
functions that use them (`_get_client()`, `_get_model()`). Now the server starts
cleanly without those packages, and a RAG tool only errors if it's actually called
without them installed — which is the correct behaviour.

**Lesson:** a heavy, optional dependency should be imported lazily, not at module
top level, so it can't take down unrelated functionality.

### 6.3 The embedding models didn't match (and that's silent)

At one point `rag.py` queried with `all-MiniLM-L6-v2` (384-dim) while
`rag_preprocessing.py` had indexed with `BAAI/bge-large-en-v1.5` (1024-dim). A
dimension mismatch *sometimes* errors outright — but a same-dimension,
different-model mismatch just returns **plausible-looking nonsense similarity
scores.** We aligned both sides on `BAAI/bge-small-en-v1.5`. (The downgrade to
`small` was itself forced by deployment — see §7.) We also had to match
`normalize_embeddings=False` on both sides, for the same reason.

**Lesson:** the embedding model is a contract between two files. Document it,
pin it, and treat any change as a breaking change to the index.

### 6.4 `ValidationError` kept getting swallowed by the broad `except`

This one recurred. The pattern "wrap the tool in `try/except Exception` and return
an error dict" is right for *runtime* errors — but it also catches
`pydantic.ValidationError`, which turns an invalid-input *rejection* into a quiet
error dict. The result: validation tests failed (FOUNDATION-3), and later CI
stopped catching invalid input at all (FEATURE-2, commit `307fdea`,
"re-raise ValidationError in MCP tools so CI catches invalid input"). The fix is
always the same — an explicit `except ValidationError: raise` placed **before** the
broad handler.

**Lesson:** "catch everything" and "validate strictly" are in direct tension. The
narrow re-raise has to come first, and it's easy to drop during a merge.

### 6.5 The first chat wiring routed too bluntly

As soon as the chat handler could call tools, PR #37 had to harden it:

- It was injecting tool **error envelopes** (`{"error": true, ...}`) into the
  system prompt as if they were live data. Added `isToolError()` to skip them.
- "Status of elevator 12345" was routing to fleet-wide `get_fleet_stats` instead of
  `get_elevator_risk`. The default for a data query now calls the per-elevator tool
  when an ID is present.
- An incomplete scheduling request (missing ID or date) silently degraded to
  advisory chatter. It now injects an `[ACTION NEEDS MORE INFO]` block so the
  assistant *asks* for the missing field instead of guessing.

**Lesson:** routing is where "it works on the happy path" and "it works" diverge.
Every branch — error, partial input, ambiguous entity — needs an explicit answer.

### 6.6 Natural-language questions never reached the manuals (the RAG fallback)

The keyword classifier only routed to RAG when a procedural anchor word
("procedure", "how to", "troubleshoot") was present. So a perfectly natural
question — *"what do I do when hydraulic pressure drops?"* — matched no keyword,
fell through to advisory, and the LLM answered from general knowledge and
**invented a procedure.** That violated the spec requirement that the same content
come back regardless of phrasing.

The fix (PR #48, commit `b5e3702`) adds an **advisory RAG fallback** in `PostChat`:
when a message is classified advisory but is substantive (a question, or ≥ 4 words —
deliberately content-agnostic so it doesn't reintroduce keyword brittleness), run a
semantic search anyway and let the **0.5 similarity threshold** decide. Inject the
docs only on a confident match; otherwise stay advisory, so greetings and
out-of-scope chatter are unaffected. The classifier and retriever were left
untouched — the relevance *score* became the arbiter, which is what the spec's
expected flow always implied.

**Lesson:** a keyword gate in front of a semantic search throws away the very
signal (the similarity score) that's better at the gate's job.

### 6.7 RAG queries were timing out

Embedding a query with a sentence-transformer on a constrained (no-GPU) CPU takes
roughly **8 seconds warm, and longer cold.** The MCP-call budget was 10s — and that
10s cap actually lived in *two* layers (`chat.go`'s data/RAG branch and
`CallMCPTool`'s own internal ceiling), so the effective deadline was
`min(10s, 10s)`. RAG lost the race, the call timed out, and chat fell back to
advisory with no data — producing "no fleet data" answers even though the documents
were sitting right there in the index. We raised **both** layers to 25s (PR #48,
commit `ee123ee`). The handler's outer 330s request deadline still bounds
everything, and the fast DB tools finish far under the new ceiling.

**Lesson:** a timeout has to be budgeted against the *slowest* legitimate operation
on the *target* hardware, and a duplicated limit in two layers is a deadline of
the smaller one.

---

## 7. DEPLOY: getting it onto Render

Deployment forced several changes that the local dev setup never needed.

- **Ollama → OpenRouter (PR #40).** Locally the chatbot talked to an Ollama server.
  The Render free tier can't run Ollama, so we swapped in an OpenRouter client
  (OpenAI-compatible, Bearer auth, reply read from `choices[0].message.content`).
  This also removed the Ollama warm-up step — a hosted API has no cold start. A
  follow-up fix replaced the hardcoded "Make sure Ollama is running" error with the
  *real* provider error (rate limits, 401s), since the Ollama message was now
  actively misleading.

- **Containerizing the MCP server (PR #41).** The `Dockerfile` bakes the embedding
  model into the image so there's no cold-start download, and **commits the
  ChromaDB index** (`data/chromadb`, ~3MB) so it ships with the image — the
  free-tier disk is ephemeral and the source documents aren't in the repo, so we
  can't rebuild the index at runtime.

- **CPU-only torch (PR #41).** On Linux the default `torch` pulls the entire NVIDIA
  CUDA stack — several gigabytes that Render can't use (no GPU) and that bloated
  both the image and the build. Installing torch from the PyTorch **CPU index**
  first means `sentence-transformers` reuses the CPU build.

- **Downgrading the embedding model to `bge-small` (commit `82fe47b`).** The
  `large` model was too heavy for the free tier. We dropped to
  `BAAI/bge-small-en-v1.5` (384-dim) — and, per §6.3, had to re-align *both* the
  preprocessing and query sides onto it.

- **Splitting requirements (DEPLOY-2, PR #43).** The dashboard, the data pipeline,
  and the MCP server have different dependency needs. Keeping
  `platform/mcp/requirements.txt` separate from the root file produces a leaner
  install and a Docker layer that doesn't rebuild every time a Flask dependency
  changes.

**Lesson:** "runs on my machine" and "runs on a free-tier no-GPU container" are
different programs. GPU assumptions, ephemeral disks, missing source files, and
package weight all surface only at deploy time.

---

## 8. CI: making the test suite deterministic

The CI workflow (`.github/workflows/ci.yml`, PR #29 onward) runs against a real
`postgres:16` service and had its own crop of fixes:

- **Pinning for Python 3.10.** CI runs 3.10 (not the dev machine's 3.14), so `numpy`
  was pinned to `2.1.3`, `uvicorn` bumped to `0.35.0` to satisfy `fastmcp 3.4.2`,
  and `pytest-asyncio` added.
- **Seeding a RAG fixture before pytest** (commit `a7b328f`) to warm the
  HuggingFace model cache, plus an `ALLOW_EMPTY_CHROMADB` flag so `rag_query`
  doesn't hard-fail when the collection is empty in CI.
- **`MCP_SKIP_CONFIRMATION` set globally in CI** so the write-tool tests don't need
  to drive the two-phase flow — with the important caveat that the two *preview*
  tests use a helper to remove that variable for the duration of the call, so the
  two-phase path is still exercised deterministically. (This flag is **test-only**
  and must never be set anywhere that serves real users — it removes the human
  approval gate.)
- **Catching `InsufficientPrivilegeError`** on the sequence DDL in `init_pool` so a
  restricted CI database role doesn't fail startup over a `CREATE SEQUENCE` it
  isn't allowed to run.

---

## 9. The write action: `schedule_inspection` (FEATURE-4)

The only tool that *writes* got the most safety machinery, because a language model
deciding to insert a database row is exactly the situation that warrants a hard
gate. The full design is in `docs/schedule_inspection.md`; the essence:

- **Two-phase confirmation.** Phase 1 (`confirmed=false`) validates, checks the
  elevator exists, and returns a human-readable summary — **no write.** Phase 2
  (`confirmed=true`) re-validates and inserts. The model must show the user the
  Phase 1 summary and get an explicit "yes" before Phase 2.
- **An HMAC signature** binds the four execution fields of the pending action to a
  server secret. Because the browser holds no session and replays the pending
  action between turns, the signature is what makes Phase 1 a *real* gate — Phase 2
  refuses any pending action it didn't itself sign, so a tampered or forged one
  can't trigger a write.
- **A duplicate guard** (a partial unique index scoped to chatbot-created rows)
  makes a double-submit or replay idempotent rather than inserting a second
  identical pending inspection.
- **An audit trail** (`scheduling_audit_log`) records every *confirmed* attempt,
  success or failure, in the same transaction as the insert, written best-effort so
  an audit failure can never mask the original error.
- **A dedicated ID sequence** starting at 9,000,000 — far above the real
  source-data range (~143k) — so MCP-scheduled inspections never collide with
  imported records.

**Lesson:** the safety of a stateless two-phase flow lives in the signature. Without
it, "Phase 1 first" is advice, not enforcement.

---

## 10. Security

A dedicated audit (`docs/mcp-security-test.md`) covered seven categories: SQL
injection, input validation, ChromaDB query safety, write-operation safety,
environment-variable handling, connection-pool safety, and singleton
initialization. All checks passed. The headline points:

- **Zero SQL injection surface.** Every query uses parameterized placeholders
  (asyncpg `$1, $2`); there are no f-strings, no concatenation, no `%`-formatting
  in any SQL string. The two parameterless tools (`get_fleet_stats`,
  `get_incident_count_last_year`) have no injection surface at all.
- **Incident narrative search uses `plainto_tsquery`,** which treats the query as
  plain text and is therefore immune to `tsquery`-syntax injection.
- **One accepted risk worth naming:** there's no authentication at the MCP layer —
  any process that can reach the port can call the tools. That's accepted *because*
  the server is deployed behind the network boundary and is not publicly exposed.
  If that ever changes, this is the first thing to revisit.

> Note: the audit was originally written against `_validators.py`; the validation
> layer is now the Pydantic models in `models.py`. The guarantees are the same (and
> stricter, thanks to `strict=True`), but if you're cross-referencing, the
> function names in the audit's validation section describe the older module.

---

## 11. Where it ended up

The MCP server starts in about two seconds, passes its PostgreSQL health check,
creates its sequence and audit DDL, and serves **10 tools**:

**Read:** `get_tssa_shutdown_elevators`, `get_inspection_history`,
`get_elevators_needing_followup`, `get_elevator_risk`, `get_fleet_stats`,
`get_incident_count_last_year`, `get_elevator_incidents`,
`search_maintenance_docs`, `search_incident_narratives`.
**Write:** `schedule_inspection`.

The chatbot now answers fleet questions from live PostgreSQL data, retrieves real
maintenance procedures from the document index regardless of how the question is
phrased, and can schedule an inspection through a confirmed, audited, signature-
gated write. The whole thing runs on a free-tier, no-GPU container.

If there's a single thread running through this whole effort, it's this: **most of
the hard problems weren't in the feature code — they were in the assumptions.** The
run command that had never run, the embedding models that silently disagreed, the
empty-but-valid result, the timeout budgeted for the wrong hardware, the keyword
gate in front of a smarter classifier. The code that fixed each one was usually
small. Finding it was the work.
