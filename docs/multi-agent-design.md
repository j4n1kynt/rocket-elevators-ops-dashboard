# Multi-Agent Chatbot Design

**Status:** Approved — Sprint 3  
**Blocks:** S3-2 (router), S3-3 (data agent), S3-4 (knowledge agent), S3-5 (scheduling agent), S3-6 (general agent)

---

## 1. Agents

The chatbot dispatches each incoming message to one of four specialized agents. Each agent has a focused system prompt and access only to the MCP tools it needs.

### 1.1 General Agent

| Field | Value |
|---|---|
| **Responsibility** | Terminology, definitions, greetings, general elevator/regulatory questions, and any question that does not fit another category |
| **Allowed tools** | None |
| **Forbidden tools** | All MCP tools (`get_fleet_stats`, `get_inspection_history`, `get_elevator_risk`, `get_elevator_incidents`, `get_elevators_needing_followup`, `get_tssa_shutdown_elevators`, `get_incident_count_last_year`, `search_maintenance_docs`, `search_incident_narratives`, `schedule_inspection`) |
| **Model** | `gemini3-flash` (Ollama cloud) |
| **Why this model** | No tool calls required. Gemini Flash prioritizes low latency, which is the main quality bar for a prompt that answers from its system prompt alone. |

**Example queries:**
- "What does TSSA stand for?"
- "What is a risk level?"
- "What is the difference between a periodic and a followup inspection?"
- "Hello, what can you help me with?"
- "What is a Customer Shutdown?"

---

### 1.2 Data Agent

| Field | Value |
|---|---|
| **Responsibility** | Fleet statistics, elevator lookups, inspection history, incident reports, and risk predictions |
| **Allowed tools** | `get_fleet_stats`, `get_inspection_history`, `get_elevator_risk`, `get_elevator_incidents`, `get_elevators_needing_followup`, `get_tssa_shutdown_elevators`, `get_incident_count_last_year` |
| **Forbidden tools** | `search_maintenance_docs`, `search_incident_narratives`, `schedule_inspection` |
| **Model** | `deepseek-v3` (Ollama cloud) |
| **Why this model** | Tool calling accuracy is the priority. DeepSeek-V3 has strong function-calling performance and follows structured output requirements reliably compared to faster alternatives. |

**Example queries:**
- "Show the inspection history for elevator E12345."
- "What elevators are currently flagged for TSSA shutdown?"
- "What is the risk level of elevator 9876?"
- "How many incidents happened in the last year?"
- "Which elevators need a followup inspection?"

---

### 1.3 Knowledge Agent

| Field | Value |
|---|---|
| **Responsibility** | Procedural, technical, and regulatory questions answered from the maintenance documentation and incident narrative corpus |
| **Allowed tools** | `search_maintenance_docs`, `search_incident_narratives` |
| **Forbidden tools** | All data tools (`get_fleet_stats`, `get_inspection_history`, `get_elevator_risk`, `get_elevator_incidents`, `get_elevators_needing_followup`, `get_tssa_shutdown_elevators`, `get_incident_count_last_year`), `schedule_inspection` |
| **Model** | `gemini3-flash` (Ollama cloud) |
| **Why this model** | The agent's job is to format and summarize retrieved RAG chunks, not to reason deeply. Gemini Flash provides fast responses; the quality of the answer comes from the retrieved documents, not the model's parametric knowledge. |

**Example queries:**
- "What is the procedure for hydraulic pressure loss?"
- "How do I handle an elevator that fails a periodic inspection?"
- "What maintenance is required after a cable replacement?"
- "What patterns appear in past incident narratives for traction elevators?"
- "When is an inspection considered overdue?"

---

### 1.4 Scheduling Agent

| Field | Value |
|---|---|
| **Responsibility** | Inspection scheduling requests — Phase 1 preview and Phase 2 confirmed write |
| **Allowed tools** | `schedule_inspection` (Phase 1: confirmed=false; Phase 2: confirmed=true) |
| **Forbidden tools** | All data tools, all knowledge tools |
| **Model** | `deepseek-v3` (Ollama cloud) |
| **Why this model** | The two-phase confirmation flow requires the model to extract structured parameters reliably (elevator ID, date, inspection type, reason) and to present a clear confirmation summary. DeepSeek-V3's instruction-following reduces the risk of malformed Phase 2 calls or ambiguous confirmation prompts. |

**Example queries:**
- "Schedule an inspection for elevator E12345 on 2026-07-15."
- "Book a periodic inspection for E9876 next Monday."
- "I need a followup inspection for elevator 4421 — it failed last week."
- "Cancel that" (cancellation during Phase 1)
- "Yes, confirm." (confirmation during Phase 2)

---

## 2. Routing Logic

The router lives in `platform/api/router.go`. It classifies each incoming message and dispatches it to one agent. The router classifies only — it does not answer.

### 2.1 Classification method

The router extends the existing keyword-based classifier in `intent.go`. No additional LLM call is made for routing. This keeps routing latency at zero and makes routing behavior predictable and testable.

**Mapping from existing intent to agent:**

| `intent.go` output | Agent |
|---|---|
| `action` | Scheduling agent |
| `data_query` | Data agent |
| `rag` | Knowledge agent |
| `advisory` | General agent |

### 2.2 Scheduling pre-emption

If the request carries a non-empty `pending_action` field, the router sends the message directly to the scheduling agent regardless of intent classification. This ensures Phase 2 confirmation messages are never misclassified.

### 2.3 Ambiguity fallback

If the classifier returns low confidence or the message does not match any keyword group, the router falls back to the general agent. The router never guesses or splits a message across agents.

### 2.4 Classification signals

| Signal | Routes to |
|---|---|
| Elevator ID pattern (e.g. `E12345`, numeric ID), "inspection history", "risk", "incident", "fleet", "shutdown", "stats" | Data agent |
| "schedule", "book", "arrange", "inspection request", "set up an inspection" | Scheduling agent |
| "how", "procedure", "manual", "maintenance", "regulation", "TSSA requirement", "what causes", "past incidents" (procedural phrasing) | Knowledge agent |
| "what does X stand for", "what is", "define", greetings, everything else | General agent |

---

## 3. Router-to-Agent Interface

All agents share the same input and output contracts. The router constructs an `AgentRequest` and the agent returns an `AgentResponse`.

### 3.1 Input — `AgentRequest`

```go
type AgentRequest struct {
    Message       string         // Raw user message
    History       []ChatMessage  // Conversation history (max 10 turns = 20 messages)
    PendingAction *PendingAction // Non-nil only for scheduling Phase 2
    AllowedTools  []string       // Tool names the agent may call (empty = no tools)
}
```

### 3.2 Output — `AgentResponse`

```go
type AgentResponse struct {
    Reply         string         // Plain-language assistant reply
    AgentName     string         // "general" | "data" | "knowledge" | "scheduling"
    PendingAction *PendingAction // Non-nil only after scheduling Phase 1
    UpdatedHistory []ChatMessage  // History with this turn appended
}
```

### 3.3 Error contract

If the agent encounters an unrecoverable error (MCP unreachable, LLM failure, invalid tool output), it returns an `AgentResponse` with a populated `Reply` containing a plain-language explanation and a nil `PendingAction`. It does not return a Go error to the router — the caller always gets a response.

---

## 4. Model Selection

### 4.1 Provider

All agents use Ollama cloud models accessed via the Ollama API (OpenAI-compatible endpoint). This replaces the previous OpenRouter + `google/gemma-4-31b-it:free` configuration.

### 4.2 Model assignments

| Agent | Model | Rationale |
|---|---|---|
| Router | No model — keyword classifier | Zero latency, deterministic, testable |
| General | `gemini3-flash` | Fastest path; no tools; quality comes from prompt |
| Knowledge | `gemini3-flash` | Formats RAG results; model speed matters more than reasoning depth |
| Data | `deepseek-v3` | Strong tool calling; structured parameter extraction |
| Scheduling | `deepseek-v3` | Precise instruction following for Phase 1/2 flow |

### 4.3 Tradeoffs considered

| Option | Tradeoff | Decision |
|---|---|---|
| Single model for all agents | Simpler config; one point of failure; slower general answers | Rejected — different agents have different latency/accuracy needs |
| LLM-based router | More accurate classification; adds ~1-2s per message | Rejected for MVP — keyword classifier is fast enough and deterministic |
| OpenRouter (previous) | Wide model selection; free tier unreliable (rate limits, cold starts) | Replaced — Ollama cloud models are more consistent |
| `deepseek-v3` for all agents | Consistent quality; higher cost and latency for simple answers | Rejected — general and knowledge agents do not need deep reasoning |

---

## 5. Dependency Map

```
S3-1 (this doc) — design agreed
    └── S3-2 (router) — reads section 2 and 3
            ├── S3-3 (data agent) — reads section 1.2
            ├── S3-4 (knowledge agent) — reads section 1.3
            ├── S3-5 (scheduling agent) — reads section 1.4
            └── S3-6 (general agent) — reads section 1.1
S3-7 (logging) — independent, can start after S3-2
S3-8 (analytics) — requires S3-7
```
