# Architecture — Rocket Elevators Operations Dashboard

## System overview

The platform has five runtime processes and two external services. All communication is HTTP; there is no message broker or shared memory.

```
Browser
   │
   │  HTMX partial-page swaps (HTML fragments)
   ▼
┌─────────────────────────────────────┐
│        Dashboard  (Flask / Python)   │
│  platform/server.py — port 5000     │
│  Jinja2 templates + Tailwind CSS    │
│  No custom JavaScript               │
└─────────┬────────────────┬──────────┘
          │ REST (JSON)    │ POST /api/chat
          ▼                ▼
┌──────────────────────────────────────────────────────────────────┐
│                       Go REST API  (Go / net/http)               │
│  platform/api/   — port 8080                                     │
│                                                                  │
│  Fleet endpoints:                                                │
│    GET /api/fleet/stats   GET /api/fleet/alerts                  │
│    GET /api/elevators     GET /api/elevators/{id}                │
│    GET /api/elevators/{id}/inspections                           │
│    GET /api/elevators/{id}/risk                                  │
│    GET /api/conversations  GET /api/conversations/{id}           │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐     │
│  │                  Multi-Agent Router                     │     │
│  │  POST /api/chat → router.go → ClassifyIntent            │     │
│  │                                                         │     │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────┐  │     │
│  │  │ general  │  │  data    │  │knowledge │  │sched-  │  │     │
│  │  │ agent    │  │  agent   │  │  agent   │  │uling   │  │     │
│  │  │(advisory)│  │(data_    │  │  (rag)   │  │(action)│  │     │
│  │  │ LLM only │  │ query)   │  │+ MCP RAG │  │+ MCP   │  │     │
│  │  └──────────┘  └────┬─────┘  └────┬─────┘  └───┬────┘  │     │
│  └───────────────────────────────────────────────────────┘      │
│                         │              │           │             │
└─────────────────────────┼──────────────┼───────────┼─────────────┘
                          │  MCP Streamable HTTP (JSON-RPC 2.0)
                          ▼              ▼            ▼
┌──────────────────────────────────────────────────────┐
│              MCP Server  (FastMCP / Python)           │
│  platform/mcp/server.py — port 8765                  │
│                                                      │
│  Read tools (7):                                     │
│    get_fleet_stats          get_elevator_risk        │
│    get_inspection_history   get_elevator_incidents   │
│    get_elevators_needing_followup                    │
│    get_tssa_shutdown_elevators                       │
│    get_incident_count_last_year                      │
│                                                      │
│  RAG tools (2):                                      │
│    search_maintenance_docs                           │
│    search_incident_narratives                        │
│                                                      │
│  Write tool (1):                                     │
│    schedule_inspection  (2-phase: propose → confirm) │
│                                                      │
│      │ SQL                         │ vector search   │
│      ▼                             ▼                 │
│  ┌───────────────────┐   ┌───────────────────────┐   │
│  │   PostgreSQL 16   │   │      ChromaDB          │   │
│  │  (Neon in prod /  │   │  data/chromadb/        │   │
│  │   Docker locally) │   │  maintenance_docs      │   │
│  │                   │   │  incident_narratives   │   │
│  │  elevators        │   └───────────────────────┘   │
│  │  inspections      │                               │
│  │  incidents        │                               │
│  │  predictions      │                               │
│  │  conversations    │                               │
│  │  messages         │                               │
│  └───────────────────┘                               │
└──────────────────────────────────────────────────────┘

  All agents call the same LLM endpoint:
  ┌──────────────────────────────────────────────────┐
  │  OpenRouter  (prod)  /  Ollama  (local dev)      │
  │  env: OPENROUTER_API_KEY, OPENROUTER_MODEL       │
  └──────────────────────────────────────────────────┘
```

---

## Request flow — chat message

1. Browser submits the chat form via HTMX `hx-post="/chat"`.
2. Flask `POST /chat` forwards the payload `{message, history, pending_action, conversation_id}` to `POST /api/chat` on the Go API.
3. Go API runs `Route()`:
   - If `pending_action` is set → skip classification, dispatch directly to `schedulingAgent`.
   - Otherwise → `ClassifyIntent()` scores the message against keyword tables and returns one of four intents: `advisory`, `data_query`, `rag`, `action`.
   - `routeIntent()` maps the intent to an agent.
4. Agent behaviour by type:

   | Agent | Target | MCP tools | LLM role |
   |---|---|---|---|
   | `general` | advisory | none | full answer |
   | `data` | mcp_data_tool | 1 data tool | intro sentence only |
   | `knowledge` | rag_search | up to 2 RAG tools | full answer over context |
   | `scheduling` | action_executor | `schedule_inspection` | full answer + confirmation |

5. Go API returns `{reply, history, pending_action, conversation_id, agent_name}`.
6. Flask passes the reply through `_render_reply()` (mistune markdown → HTML, label bolding, risk badges) and returns an HTML fragment.
7. HTMX swaps the fragment into the bubble area.

---

## Intent classifier

`platform/api/intent.go` — purely deterministic, no ML, no external calls.

```
message
  ├─ keyword scan → per-intent score (weighted sum)
  ├─ entity bonus: elevator ID → +0.5 data + action; date → +0.5 action
  ├─ action co-occurrence: verb + (ID or date) → +1.0 action
  ├─ confidence = score / (score + 1.0)   floor = 0.6
  └─ below floor → advisory fallback

  context-carry: if follow-up + low confidence → inherit previous non-advisory intent
```

---

## Scheduling — 2-phase confirmation

The `schedule_inspection` MCP tool uses a 2-phase write pattern to prevent accidental DB writes:

1. **Phase 1** — Go API calls `schedule_inspection` with `confirmed=false`. MCP validates (elevator exists, date valid, no duplicate) and returns a proposal. Go builds a `PendingAction` with HMAC signature and 10-minute TTL.
2. The user sees a confirmation prompt. `PendingAction` travels back to Flask as JSON in a hidden form field.
3. **Phase 2** — on the next message, Go verifies signature and TTL, then calls `schedule_inspection` with `confirmed=true`. Only then does the MCP server write to the database.

---

## ChromaDB / RAG

Two vector collections committed in `data/chromadb/` (≈3 MB, static):
- `maintenance_docs` — indexed from PDF maintenance manuals in `intelligence/rag_documents/`
- `incident_narratives` — indexed from structured incident records

Populated offline by `intelligence/rag_preprocessing.py`. Re-run only when source PDFs change. The committed directory ships with the MCP Docker image so Render does not need to re-index on deploy.

---

## Deployment

| Service | Platform | Start command |
|---|---|---|
| Dashboard | Render free | `py -3 platform/server.py` |
| Go API | Render free | binary via `platform/api/Dockerfile` |
| MCP server | Render free | `platform/mcp/Dockerfile` |
| PostgreSQL | Neon | managed, TLS |

Free-tier Render services spin down after 15 min idle. Cold start: 30–60 s (MCP is slowest — loads the embedding model on boot).

---

## Environment variables

| Variable | Consumed by | Purpose |
|---|---|---|
| `DATABASE_URL` | Go API, MCP server | PostgreSQL connection string |
| `MCP_SERVER_URL` | Go API | FastMCP base URL (default `http://localhost:8765`) |
| `OPENROUTER_API_KEY` | Go API | LLM provider auth |
| `OPENROUTER_MODEL` | Go API | Model name (default `google/gemma-4-31b-it:free`) |
| `GO_API` | Flask | Go API base URL (default `http://localhost:8080`) |
| `PORT` | Go API | HTTP listen port (default `8080`) |
| `MCP_PORT` | MCP server | HTTP listen port (default `8765`) |
