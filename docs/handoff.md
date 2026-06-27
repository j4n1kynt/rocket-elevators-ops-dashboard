# Handoff Guide — Rocket Elevators Operations Dashboard

This document lets a new developer set up, run, and extend the project without help from the original team.

---

## Prerequisites

| Tool | Version | Notes |
|---|---|---|
| Python | 3.10+ (3.12 recommended) | `py -3 --version` |
| Go | 1.22+ | `go version` |
| Docker + Docker Compose | any recent | for local PostgreSQL + MCP |
| Git | any | — |

---

## Local setup (step by step)

### 1. Clone and install Python dependencies

```bash
git clone https://github.com/j4n1kynt/rocket-elevators-ops-dashboard.git
cd rocket-elevators-ops-dashboard
pip install -r requirements.txt
```

### 2. Create the environment file

The Go API reads credentials from `platform/api/.env` (gitignored). Create it:

```
DATABASE_URL=postgresql://api_user:api_password@localhost:5432/rocket_elevators
MCP_SERVER_URL=http://localhost:8765
OPENROUTER_API_KEY=<your key from openrouter.ai>
OPENROUTER_MODEL=google/gemma-4-31b-it:free
```

For Ollama local dev, set `OPENROUTER_API_KEY` to your Ollama key and point `OPENROUTER_MODEL` to your local model name.

The Flask server reads `GO_API` from the shell:

```bash
export GO_API=http://localhost:8080   # default — only needed if you change the port
```

### 3. Start PostgreSQL and the MCP server via Docker

```bash
docker-compose up
```

This starts:
- PostgreSQL 16 on port `5432`
- MCP server on port `8765` (first build is slow — downloads the embedding model)

> Port `8080` is used by the Go API. Port `8081` is blocked by the company firewall — do not use it.

### 4. Load data into PostgreSQL

Run these scripts from the project root. They are idempotent.

```bash
# Populate all tables from source datasets
py -3 intelligence/etl_to_database.py

# Generate AI risk explanations for HIGH-risk elevators (needs the DB running)
py -3 intelligence/generate_explanations.py
```

### 5. Start the Go API

In a second terminal:

```bash
cd platform/api
go run .
```

The API starts on `http://localhost:8080`. Verify: `curl http://localhost:8080/health`

### 6. Start the dashboard

In a third terminal:

```bash
py -3 platform/server.py
```

Open `http://localhost:5000`.

### 7. Verify the chatbot

Open the chat widget (bottom-right bubble on any page) and ask:

- "How many elevators are TSSA shut down?" → data agent
- "How do I lubricate the guide rails?" → knowledge agent
- "Schedule an inspection for elevator 12345 on 2026-07-01" → scheduling agent
- "What is the inspection pass rate?" → general agent

---

## Running tests

```bash
# Self-contained suite (no live services needed)
make test

# Or individually:
make test-go       # Go build + vet + test
make test-python   # isolated Python suites (mocked DB/LLM/MCP)

# Integration tests (requires DATABASE_URL and running PostgreSQL):
make test-integration
```

---

## Deployment

The three services deploy independently on Render. Each has its own Dockerfile or start command configured in the Render dashboard.

| Service | Build/start | Key env vars to set |
|---|---|---|
| Dashboard | `py -3 platform/server.py` | `GO_API` |
| Go API | `platform/api/Dockerfile` | `DATABASE_URL`, `MCP_SERVER_URL`, `OPENROUTER_API_KEY`, `OPENROUTER_MODEL` |
| MCP server | `platform/mcp/Dockerfile` | `DATABASE_URL` |

PostgreSQL is hosted on [Neon](https://neon.tech). The connection string is in the Render environment variables for the Go API and the MCP server.

**Cold-start workaround**: the free tier spins down after 15 min idle. To wake all three services before a demo, open these URLs in order and wait for a response from each:
1. MCP server health: `<mcp_url>/health`
2. Go API: `<api_url>/health`
3. Dashboard: `<dashboard_url>/`

---

## Repository structure

```
.
├── data/                   Source datasets (read-only — never edit in place)
│   ├── merged_elevator_data.csv
│   ├── incident.json
│   ├── predictions.csv
│   └── chromadb/           ChromaDB vector store (committed, ≈3 MB)
├── platform/
│   ├── server.py           Flask app (dashboard + chat routes)
│   ├── layout.html         Shell layout (nav, top bar, chat widget)
│   ├── _*.html             Jinja2 partial templates
│   └── api/                Go REST API
│       ├── main.go         HTTP server + route registration
│       ├── router.go       Multi-agent router + context-carry
│       ├── intent.go       Deterministic intent classifier
│       ├── agents.go       Four agent implementations
│       ├── chat.go         POST /api/chat handler + LLM client
│       ├── data_format.go  Go-side deterministic data formatters
│       ├── mcp_client.go   MCP Streamable HTTP client
│       ├── handlers.go     Fleet/elevator REST handlers
│       ├── db.go           PostgreSQL pool init
│       ├── migrations/     SQL migration files
│       └── prompts/        System prompt text files per agent
│   └── mcp/
│       ├── server.py       FastMCP server entry point
│       ├── db.py           Async PostgreSQL pool
│       ├── rag.py          ChromaDB client + embedding
│       └── tools/          One file per tool group
├── intelligence/
│   ├── etl_to_database.py  Load CSV/JSON into PostgreSQL
│   ├── generate_predictions.py  ML risk model inference
│   ├── generate_explanations.py AI risk explanation generation
│   ├── rag_preprocessing.py    Build ChromaDB collections from PDFs
│   └── rag_documents/      Source PDFs (gitignored — place manually)
├── docs/                   Specs and project documentation
├── docker-compose.yml      Local dev stack (PostgreSQL + MCP)
├── Makefile                Common dev commands
└── requirements.txt        Root Python dependencies (Flask, mistune, pandas…)
```

---

## Branching strategy

```
main ← dev ← feat/<card-id>-<description>-<initials>
                    fix/<description>-<initials>
                    docs/<description>-<initials>
```

- `main` — production-ready. Only receives merges from `dev` at sprint end via PR.
- `dev` — integration branch. All feature/fix PRs target here.
- Feature branches — one task per branch, one branch per PR. 1 reviewer minimum before merge.

---

## What works well

- **Zero-JavaScript dashboard**: HTMX handles all partial-page updates and form submissions without any custom JS. Adding a new page requires only a Flask route, a Jinja2 template, and HTMX attributes — no JavaScript bundler, no state management.
- **Deterministic intent classifier**: the keyword-based router (`intent.go`) is fast, testable, and fully traceable via the `Signals` field. Adding a new keyword is a one-line change with a clear test to match.
- **Hybrid data formatting**: Go formats the exact data block; the LLM writes only a one-sentence intro. This eliminates hallucinated numbers and inconsistent markdown in data responses.
- **2-phase scheduling confirmation**: the HMAC-signed `PendingAction` pattern prevents accidental writes. Expiry (10 min TTL) keeps the confirmation safe without requiring a session or database state.
- **Committed ChromaDB**: the vector store ships with the repo (≈3 MB) so the MCP server does not need to re-index on every Render deploy.

---

## Known issues

- **Render free-tier cold start**: all three services spin down after 15 min idle. The MCP server is slowest to wake (loads the embedding model — 30–60 s). The chatbot will time out on the first request after a cold start. Mitigation: pre-warm by hitting `/health` on each service before use.
- **LLM rate limits (free tier)**: OpenRouter's free `google/gemma-4-31b-it:free` model is rate-limited. Under load, users may see "The model is currently rate-limited. Please try again." Mitigation: upgrade to a paid model, or add retry logic in `chat.go`.
- **Risk badges regex over full HTML**: `_render_reply()` in `server.py` applies the risk-badge substitution regex over the already-rendered HTML string, which can mis-fire if "HIGH", "MEDIUM", or "LOW" appear inside an HTML attribute. Not user-visible yet, but a cleaner fix would be a custom mistune renderer that applies badges at the AST node level.
- **Incident count and alteration count from CSV**: `elevator_detail()` in `server.py` still reads incident and alteration counts from the original CSV files, not from PostgreSQL. These two fields are not exposed by the Go API, so the Flask server loads and queries DataFrames on every detail panel request.
- **No authentication**: the dashboard is publicly accessible with no login. Any user can view all fleet data and schedule inspections.
- **No pagination on conversation history**: loading a conversation with many messages returns all messages in one request. Long conversations may be slow.

---

## What to build next

- **Authentication**: add a login page (Flask-Login or session-based) so only authorized operations staff can access the dashboard and schedule inspections.
- **Incident and alteration counts moved to PostgreSQL**: remove the CSV dependency in `elevator_detail()` by adding counts to the Go API `GET /api/elevators/{id}` response.
- **Streaming LLM responses**: stream the LLM reply token-by-token to the browser so users see output immediately instead of waiting for the full response. HTMX SSE extension supports this without custom JS.
- **Intent classifier improvements**: add anchor phrases for anchor-less procedural questions (`"what causes"`, `"keeps"`, `"what do I do"`). See `docs/improvements.md` for the full proposal.
- **Paid Render plan**: eliminate cold starts. Each service costs ~$7/month on the starter plan.
- **RAG re-indexing pipeline**: automate `rag_preprocessing.py` in CI so new maintenance PDFs are indexed on merge to `main` without a manual local run.
