# S3-2 Router Refactor — Work Log

This document tracks what was built, what decisions were made, and what issues came up during the S3-2 router refactor. It is written for developers picking up this work or reviewing it.

---

## What S3-2 is about

The goal of S3-2 is to break the monolithic `chat.go` handler into a proper multi-agent architecture. Before this sprint, all routing, tool calls, and LLM calls lived inline in a single `PostChat` function. S3-2 introduces the infrastructure that the four specialized agents (data, knowledge, scheduling, general) will plug into in S3-3 through S3-6.

The design source of truth is `docs/multi-agent-design.md`. All decisions here trace back to that document.

---

## What we built

### 1. Shared interface types in `models.go`

Three new types were added to `platform/api/models.go`. These are the contracts that let the router and all agents talk to each other without knowing each other's internals.

**`AgentRequest`** — what goes into any agent. It carries the raw user message, the conversation history, any pending scheduling action (used for the two-phase inspection confirmation flow), and the list of MCP tools that agent is allowed to call.

**`AgentResponse`** — what comes back from any agent. It carries the reply text, the name of the agent that handled the request, any new pending action (set only after a Phase 1 scheduling preview), and the updated conversation history with this turn appended.

**`AgentFunc`** — a Go function type that formalizes what "an agent" is: a function that takes a context and an `AgentRequest` and returns an `AgentResponse`. The router holds a map of these, so it can call any agent through a single consistent interface without branching on agent names.

These three types live together in `models.go` because they form a single cohesive interface. Keeping them in one place makes it easy for any developer working on an agent card (S3-3 through S3-6) to find the full contract in one file.

A `context` import was added to `models.go` to support the `AgentFunc` signature.

**Decision made:** `AgentFunc` was initially written inside `router.go`. We moved it to `models.go` during review because it is a shared type — it belongs alongside `AgentRequest` and `AgentResponse`, not buried in the file that happens to use it first.

---

### 2. The router in `router.go`

A new file `platform/api/router.go` was created. It has one job: receive a request, decide which agent should handle it, call that agent, and return the response.

**How it dispatches:**

The router uses a map (`agents`) that links each routing target string to its `AgentFunc`. The routing target strings (`"advisory"`, `"mcp_data_tool"`, `"rag_search"`, `"action_executor"`) come directly from the existing `routeIntent` function in `intent.go` — the router does not reimplement any classification logic, it just wraps what was already there.

**The entry point is `Route(ctx, req)`:**

1. If the request has a `PendingAction` (a non-nil pending scheduling confirmation), the message goes straight to the scheduling agent. The intent classifier is skipped entirely. This is the pre-emption rule from the design doc (§2.2) — it ensures that a user's "yes, confirm" reply after a Phase 1 scheduling preview always reaches the scheduling agent, regardless of what words they used.

2. Otherwise, `ClassifyIntent` and `routeIntent` from `intent.go` are called. The result is looked up in the `agents` map. If for any reason the target is not found (which should not happen in practice), the general agent is used as a safe fallback.

**Logging:**

Every request is logged with a `[router]` prefix so it is easy to grep in server output. Three cases are covered:
- Pre-emption path: logs that a `pending_action` was present and the message went directly to scheduling.
- Normal path: logs the classified intent, the confidence score, and the selected routing target.
- Fallback path: logs the unknown target string if `routeIntent` ever returns something the map does not recognize.

**Error contract:**

`Route` uses a named return value and a `defer/recover` block. If any agent panics, the recover catches it, logs it with a `[router]` prefix, and sets the named return to an `AgentResponse` with a plain-language message. The caller always gets a response — the server never crashes and never returns a Go error or an empty reply. This matches the error contract in the design doc (§4.3).

**What is not in the router:**

The router does not call any MCP tools, does not call the LLM, and does not answer the user. It classifies and dispatches — nothing more.

### 3. Provider migration — OpenRouter → Ollama cloud (`chat.go` + `chat_test.go`)

The LLM client was fully migrated from OpenRouter (OpenAI-compatible) to Ollama cloud (native format). Here is everything that changed:

**Request/response structs:**
- `openAIChatReq` renamed to `ollamaChatReq` — same fields (`model`, `messages`, `stream`), new name to reflect the actual provider
- `openAIChatResp` replaced by `ollamaChatResp` — the Ollama native response carries the reply in `message.content` and a `done` boolean, not in `choices[0].message.content` like the OpenAI format did

**Endpoint and base URL:**
- Endpoint changed from `/chat/completions` to `/chat`
- Default base URL changed from `https://openrouter.ai/api/v1` to `https://ollama.com/api`

**Environment variables:**
- `OPENROUTER_BASE_URL` → `OLLAMA_BASE_URL`
- `OPENROUTER_API_KEY` → `OLLAMA_API_KEY`
- `OPENROUTER_MODEL` → `OLLAMA_MODEL`
- Default model changed from `google/gemma-4-31b-it:free` to `minimax-m2.5` (selected in S3-1 model testing — fastest accurate free-tier model at 2.7s on the T1 no-tool benchmark)

**Error messages in `PostChat`:** all three provider-specific error strings updated to reference Ollama instead of OpenRouter.

**`chat_test.go` updates:**
- `TestCallLLMSuccess`: updated struct type from `openAIChatReq` to `ollamaChatReq`, changed mock response to Ollama shape, updated path assertion from `/chat/completions` to `/chat`, updated model name
- `TestCallLLMMissingKey`: updated expected error string to `"OLLAMA_API_KEY is not set"`
- `TestCallLLMNoChoices` renamed to `TestCallLLMEmptyMessage` — the `choices[]` concept does not exist in the Ollama format; the equivalent failure is an empty `message.content`
- `TestCallLLMEmptyContent` renamed to `TestCallLLMWhitespaceContent` — both tests now use the Ollama response shape

**Known compile errors (expected until Task 4):**

The `agents` map references `generalAgent`, `dataAgent`, `knowledgeAgent`, and `schedulingAgent`. These functions do not exist yet — they will be added as stubs in the next task. The file will not compile until those stubs are in place.

---

**Warm-up and timeouts:**

Two cold-start mitigations from Sprint 2 are preserved:

- The 330s deadline in `PostChat` (`context.WithTimeout(r.Context(), 330*time.Second)`) is unchanged. The Go HTTP server's `WriteTimeout` is disabled for the `/api/chat` route so this context governs the full LLM round-trip.
- A `WarmUpLLM()` function was added to `chat.go`. It fires a single `"ping"` message to Ollama at startup with a 300s timeout, so the model is loaded before the first real user request arrives. It runs in a goroutine (`go WarmUpLLM()` in `main.go`) — it never blocks server startup and logs and discards any error silently. The two timeouts (warm-up and real request) are independent contexts and do not interfere with each other.

---

### 4. Agent handlers in `agents.go`

A new file `platform/api/agents.go` contains the four agent functions. Each matches the `AgentFunc` signature (`func(ctx context.Context, req AgentRequest) AgentResponse`) and reproduces the behavior that previously lived inline in `PostChat`.

**`generalAgent`** — handles advisory intent and owns the FEATURE-3 RAG fallback:
- Calls `shouldTryRagFallback` on the message. This gate passes for any real question (contains `?` or has 4+ words) and blocks trivial chatter (greetings, "thanks") to avoid paying embedding latency on non-questions.
- If the gate passes, calls `search_maintenance_docs` with a 25s timeout. The similarity threshold on the MCP server (0.5 floor) decides relevance — only confident matches are injected as context. If nothing clears the threshold, `hasConfidentResults` returns false and the agent stays purely advisory.
- This fallback exists because the keyword classifier in `intent.go` only routes to RAG when a procedural anchor word is present. Natural-language questions like "what do I do when hydraulic pressure drops?" fall through to advisory and would never reach the manuals without it.

**`dataAgent`** — re-classifies the message to get entities, calls `buildMCPArgs` to select the right data tool, runs it with a 25s timeout (matching the original combined data/RAG branch in `PostChat`), and injects the result. Falls back to advisory on MCP error or error payload.

**`knowledgeAgent`** — same pattern as `dataAgent` with a 25s timeout. Handles `search_maintenance_docs` and `search_incident_narratives`.

**`schedulingAgent`** — handles three paths:

- **Phase 2 — confirmation flow (`req.PendingAction != nil`, user replied yes/no):**
  - `detectConfirmation` classifies the reply as confirm, cancel, or ambiguous. An ambiguous reply (both confirm and cancel words present) defaults to cancel — never write on ambiguity.
  - On confirm: `verifyPendingAction` checks the HMAC signature — rejects anything not produced by a genuine Phase 1 on this server. Then checks `ExpiresAt` — rejects expired confirmations so a stale request cannot be replayed. If both pass, calls `schedule_inspection` with `confirmed=true` to write the inspection.
  - On cancel: sets `[ACTION CANCELLED]` context so the LLM confirms no write occurred.

- **Phase 1 — new scheduling request (`req.PendingAction == nil`):**
  - Re-classifies to extract entities (elevator ID, date, inspection type).
  - If either the elevator ID or the date is missing, sets `[ACTION NEEDS MORE INFO]` context instead of calling the tool.
  - If both are present, calls `schedule_inspection` with `confirmed=false`, which returns a preview without writing. Builds a `PendingAction` from the result, sets its `ExpiresAt` TTL, and signs it with HMAC. The signed `PendingAction` is returned on the response so the client holds it until the user confirms.

- **Abandoned confirmation (`req.PendingAction != nil`, user replied with neither yes nor no):**
  - Clears the pending state (no database write).
  - Re-classifies the message and inlines the full classify→MCP dispatch from the original `PostChat` — all four intent branches with their original timeouts and RAG fallback. Exactly reproduces the original "falls through to classifier" behavior.

**Two shared helpers** in `agents.go` eliminate repetition:
- `buildReply` — assembles the system prompt + data context + history into a message list and calls `callLLM`. Returns a plain-language error string on LLM failure (§4.3 — no Go error returned).
- `appendHistory` — appends the current turn to the history slice and returns the updated slice.

**`PostChat` is now a thin HTTP adapter** (~30 lines). It decodes the request, validates the message, caps history at 20 messages, wraps the request in a 330s context, calls `Route`, and writes the `ChatResponse`. All routing, MCP dispatch, and LLM calls have moved into agents. LLM errors are returned as plain-language text in the reply rather than HTTP 503 responses.

---

## What is still pending in S3-2

S3-2 is complete. The following cards are now unblocked:

- **S3-3** (data agent) — replace the `dataAgent` stub with the full implementation using the Data Agent system prompt from `docs/multi-agent-design.md` §3.2
- **S3-4** (knowledge agent) — replace the `knowledgeAgent` stub with the full implementation using §3.3
- **S3-5** (scheduling agent) — replace the `schedulingAgent` stub with the full implementation using §3.4
- **S3-6** (general agent) — replace the `generalAgent` stub with the full implementation using §3.1
- **S3-7** (logging) — independent, can start now

---

## Notes for future agents working on S3-3 through S3-6

- Your agent function signature must match `AgentFunc`: `func(ctx context.Context, req AgentRequest) AgentResponse`.
- The `AllowedTools` field on `AgentRequest` is set by the router. Your handler should only call the tools listed there.
- If your agent hits an unrecoverable error (MCP unreachable, LLM failure), return an `AgentResponse` with a plain-language message in `Reply` and a nil `PendingAction`. Do not return a Go error — the router always expects a response.
- The scheduling agent is the only one that should ever set a non-nil `PendingAction` on the response, and only after a Phase 1 preview (confirmed=false).
