# Error Handling & Consistent Formatting

This document records every error-handling decision made in the chat agent pipeline. It is written for developers joining the team who need to understand what gaps existed, what was fixed, and what rules to follow going forward.

---

## Background

The agent pipeline (`platform/api/`) follows an inject-and-answer pattern: each agent calls an MCP tool, injects the result into a system-message data context block, then calls the LLM once. When a tool call failed, the original code logged the error and left the data context empty. The LLM then had no signal that anything went wrong and silently answered in advisory mode — giving the user a confident-sounding generic reply when it should have explained that live data was unavailable.

A separate but related gap existed in `cleanValidationError`: its fallback branch returned the raw Go error string, which could send infrastructure-level text (e.g. `dial tcp 127.0.0.1:8765: connect: connection refused`) directly into the LLM system prompt.

---

## Changes

### 1. `dataAgent` — MCP failure now injects an unavailability notice

**File:** `platform/api/agents.go` — `dataAgent` function

**The bug:** When `CallMCPTool` returned a transport error (`err != nil`) or a tool-level error payload (`isToolError(result)`), both branches left `dataContext` empty. `buildReply` then called the LLM with no data context, causing the model to answer as though it had general knowledge rather than telling the user the service was down.

**What was changed:** Both failure branches now set `dataContext` to a structured plain-language notice instead of leaving it empty.

```go
// Before
if err != nil {
    log.Printf("[data] mcp tool %s failed: %v — falling back to advisory", toolName, err)
} else if isToolError(result) {
    log.Printf("[data] mcp tool %s returned error payload — falling back to advisory", toolName)
} else {
    dataContext = "[DATA SOURCE: PostgreSQL — live fleet data]\n" + result
}

// After
if err != nil {
    log.Printf("[data] mcp tool %s failed: %v", toolName, err)
    dataContext = "[DATA SERVICE UNAVAILABLE: the PostgreSQL data service could not be reached. " +
        "Tell the user that live fleet data is currently unavailable and you cannot answer their question with real data. " +
        "Do not speculate or answer from general knowledge.]"
} else if isToolError(result) {
    log.Printf("[data] mcp tool %s returned error payload", toolName)
    dataContext = "[DATA SERVICE ERROR: the data tool returned an error response. " +
        "Tell the user that live fleet data is currently unavailable and you cannot answer their question with real data. " +
        "Do not speculate or answer from general knowledge.]"
} else {
    dataContext = "[DATA SOURCE: PostgreSQL — live fleet data]\n" + result
}
```

**Why two separate messages:** Transport errors (`err != nil`) mean the service is unreachable. Tool error payloads (`isToolError`) mean the service responded but reported an internal error. The distinction is surfaced in the log but both produce equivalent user-facing text — the LLM is told not to speculate in either case.

---

### 2. `knowledgeAgent` — both corpora hard-failing now injects an unavailability notice

**File:** `platform/api/agents.go` — `knowledgeAgent` function

**The bug:** Same silent-fallback pattern as `dataAgent`, but with an added complexity: `knowledgeAgent` has two tool call attempts (primary corpus and fallback corpus). After both attempts, `dataContext` could be empty for two different reasons that require different responses:

| Reason | Service state | Correct response |
|---|---|---|
| Both tools failed (transport or tool error) | Service is down | Inject unavailability notice |
| Both tools responded but returned no confident results | Service is up, no relevant docs | Leave empty — LLM should say it found nothing |

The original code treated both as identical (log + empty context). A developer reading the logs would see "no confident results from either corpus — advisory only" even when the service was completely unreachable.

**What was changed:** A `hardFailed` bool was introduced for each tool call. It is set to `true` only when the call broke at the service level (transport error or tool error payload), not when the service responded but found nothing relevant. After both attempts, the final check branches on whether both flags are set.

```go
// Track hard failures separately from soft failures (no confident results)
var primaryHardFailed bool
if primaryErr != nil {
    primaryHardFailed = true
} else if isToolError(primaryResult) {
    primaryHardFailed = true
} else if hasConfidentResults(primaryResult) {
    dataContext = knowledgeSourceLabel(primaryTool) + primaryResult
}

var fallbackHardFailed bool
if dataContext == "" {
    // ... fallback call ...
    if fallbackErr != nil {
        fallbackHardFailed = true
    } else if isToolError(fallbackResult) {
        fallbackHardFailed = true
    } else if hasConfidentResults(fallbackResult) {
        dataContext = knowledgeSourceLabel(fallbackTool) + fallbackResult
    }
}

if dataContext == "" {
    if primaryHardFailed && fallbackHardFailed {
        log.Printf("[knowledge] both corpus tools unreachable — injecting unavailability notice")
        dataContext = "[DATA SERVICE UNAVAILABLE: the maintenance knowledge base could not be reached. " +
            "Tell the user that documentation search is currently unavailable and you cannot answer their question from maintenance records. " +
            "Do not speculate or answer from general knowledge.]"
    } else {
        log.Printf("[knowledge] no confident results from either corpus — advisory only")
    }
}
```

**Mixed failure cases:** If only one tool hard-failed and the other returned a real response (even with no confident results), the service is partially up. This is treated as a content miss ("no relevant docs") rather than an outage, so `dataContext` remains empty and the LLM handles it normally.

---

### 3. `cleanValidationError` — raw Go error strings leaked to the LLM

**File:** `platform/api/chat.go` — `cleanValidationError` function

**The bug:** `cleanValidationError` is designed to strip Pydantic boilerplate from MCP validation error messages. It scans lines for a `"Value error, "` prefix and returns the user-facing message. When no match was found (i.e. the error was a transport failure, not a Pydantic validation error), the function returned the raw input string unchanged. This raw string could contain Go infrastructure text such as:

```
mcp server unreachable: Post "http://127.0.0.1:8765/mcp": dial tcp 127.0.0.1:8765: connectex: No connection could be made...
```

This string was then injected into the scheduling agent's `dataContext` and passed to the LLM.

**Call sites affected:** Both Phase 1 and Phase 2 of `schedulingAgent` in `agents.go`:

```go
errMsg := cleanValidationError(err.Error())
dataContext = "[ACTION VALIDATION ERROR]\n" + errMsg + "..."
```

**What was changed:** The fallback `return raw` was replaced with a generic plain-language message.

```go
// Before
func cleanValidationError(raw string) string {
    for _, line := range strings.Split(raw, "\n") {
        line = strings.TrimSpace(line)
        if after, ok := strings.CutPrefix(line, "Value error, "); ok {
            return after
        }
    }
    return raw  // leaked Go error strings
}

// After
func cleanValidationError(raw string) string {
    for _, line := range strings.Split(raw, "\n") {
        line = strings.TrimSpace(line)
        if after, ok := strings.CutPrefix(line, "Value error, "); ok {
            return after
        }
    }
    return "The request could not be processed. Please try again."
}
```

**What is not affected:** `extractScheduleError` reads the `"error"` field from the MCP server's JSON payload. Those messages are written by the Python MCP server and are already user-friendly (e.g. `"Elevator not found"`, `"Date must be in the future"`). That function was not changed.

---

## Tests Added

**File:** `platform/api/agents_test.go`

### New helper: `capturingLLMServer`

The existing `fakeLLMServer` returns a canned reply but does not expose what was sent to the LLM. To assert that no raw error text appears in the system message, a `capturingLLMServer` was added. It records the content of every `"role": "system"` message it receives, making it available via `SystemMessages()` after the agent returns.

This is the correct level to assert at: the system message is what actually reaches the model, not the final reply string (which is just the fake server's canned response).

### `TestDataAgentMCPTransportFailureNoRawError`

Covers the `err != nil` path in `dataAgent`. The MCP server is started and immediately closed before the agent runs, producing a connection-refused transport error.

**Assertions:**
- System message contains `"DATA SERVICE UNAVAILABLE"` — confirms the notice was injected.
- System message does not contain `"connection refused"`, `"dial tcp"`, `"mcp server unreachable"`, or `"mcp status"` — confirms no raw Go error string reached the LLM.

### `TestKnowledgeAgentBothCorporaToolErrorNoRawError`

Covers the `isToolError` path (distinct from transport errors) for both corpora. The MCP server returns `isError: false` at the JSON-RPC level but with `{"error": true}` as the content payload — this triggers `isToolError()` rather than the `err != nil` branch. Both `search_maintenance_docs` and `search_incident_narratives` return this payload, so both `hardFailed` flags are set.

**Assertions:**
- System message contains `"DATA SERVICE UNAVAILABLE"`.
- System message does not contain `"tool error"`, `"connection refused"`, `"dial tcp"`, or `"internal tool error"`.

**Both tests passed on first run.**

---

### 5. Shared `## Response Format` section in all four prompt files

**Files:** `platform/api/prompts/general_prompt.md`, `data_prompt.md`, `knowledge_prompt.md`, `scheduling_prompt.md`

**The gap:** Format guidance was scattered across `## Tone` and `## Hard Limits` in each file — sometimes contradictory, always inconsistent. The 1500-token ceiling appeared in four different places in four different forms. Citation rules existed only in agents that had tool results. Bullet vs. prose guidance existed only in `general_prompt`.

**What was changed:** A `## Response Format` section was inserted before `## Hard Limits` in each file. It contains three subsections:

- **Citation style** — always name the source. `general_prompt` cites regulations by full name. `data_prompt` and `knowledge_prompt` reference their existing "Answering from…" sections for specific format. `scheduling_prompt` instructs the model to present exactly what the tool returned.
- **Lists vs. prose** — use bullets for three or more discrete enumerable items; use prose for everything else. Identical rule across all four files.
- **Answer length** — 1500-token ceiling, default to the shortest answer the question supports, no padding. Identical rule across all four files.

**Tone alignment (same session):** The `## Tone` sections were also aligned so all four files open with the same three sentences:
> Use clear, professional language. Be concise — answer the question asked, not everything adjacent to it. Never reproduce raw data structures or raw tool output.

Agent-specific additions follow: `general_prompt` keeps the jargon/technical-terms guidance; `knowledge_prompt` keeps the step-by-step vs. concise-summary rule; `scheduling_prompt` keeps "Confirmation prompts must be unambiguous." `data_prompt` needed nothing extra after the core.

Redundant format guidance was removed from existing sections:
- `general_prompt` Tone: removed "Keep answers concise — one to three paragraphs..." and "Do not use bullet lists for every response; match format to the question."
- `general_prompt` Hard Limits: removed "4. Output length: stay within 1500 tokens. Be concise."
- `data_prompt` Tone: removed "Stay within 1500 tokens."
- `knowledge_prompt` Tone: removed "Stay within 1500 tokens."
- `scheduling_prompt` Hard Limits: removed "Output: stay within 1500 tokens."

### `TestBuildReplyMalformedLLMOutput`

Table-driven sub-tests that call `buildReply` directly with a `fakeLLMServer` returning a controlled bad reply.

| Sub-test | LLM reply | Expected |
|---|---|---|
| `raw_error` | `"mcp server unreachable: dial tcp ..."` | safe fallback |
| `json_object_blob` | `{"elevators":[...]}` | safe fallback |
| `json_array_blob` | `[{"id":1,...},{"id":2,...}]` | safe fallback |
| `json_blob_with_leading_whitespace` | `"\n\n  {...}"` | safe fallback |
| `markdown_link_not_json` | `"[TSSA guidance](https://...) covers ..."` | passed through |
| `citation_not_json` | `"[1] According to the maintenance log, ..."` | passed through |
| `short_reply` | `"OK"` | `"OK"` passed through |

The `short_reply` case is an intentional pass-through assertion — it documents that short replies are logged but not discarded, so a future developer cannot accidentally change the behaviour without the test failing. The `json_blob_with_leading_whitespace`, `markdown_link_not_json`, and `citation_not_json` cases lock in the two refinements described under §4 (trim before checks; `[`-leading replies discarded only when they actually parse as JSON).

---

### 4. `buildReply` — LLM output sanity checks

**File:** `platform/api/agents.go` — `buildReply` function and two new helpers

**The gap:** `callLLM` can return a non-error string that is still not a usable natural-language reply. Three failure modes were identified:

1. **Raw error string** — the LLM echoes back infrastructure text that leaked into the prompt, producing a reply that starts with `"mcp "`, `"dial tcp"`, `"connection refused"`, `"connectex:"`, or `"llm returned status"`.
2. **Unparsed JSON blob** — the model returns structured JSON (starting with `{` or `[`) instead of a prose answer.
3. **Suspiciously short reply** — the reply is under 20 characters, which may indicate a truncated or degenerate response.

None of these were checked. The raw string was passed directly to the caller and shown to the user.

**What was changed:** Two helper functions were added and three checks were inserted in `buildReply` after the successful `callLLM` return.

```go
func looksLikeRawError(reply string) bool {
    lower := strings.ToLower(reply)
    for _, marker := range []string{"mcp ", "dial tcp", "connection refused", "connectex:", "llm returned status"} {
        if strings.HasPrefix(lower, marker) {
            return true
        }
    }
    return false
}

func looksLikeJSON(reply string) bool {
    trimmed := strings.TrimSpace(reply)
    if strings.HasPrefix(trimmed, "{") {
        return true
    }
    if strings.HasPrefix(trimmed, "[") {
        return json.Valid([]byte(trimmed))
    }
    return false
}
```

```go
// In buildReply, after callLLM returns without error:
// Trim first so leading whitespace/newlines don't cause the prefix-based
// checks below to miss a malformed reply.
reply = strings.TrimSpace(reply)
if looksLikeRawError(reply) {
    log.Printf("[agent] llm reply looks like a raw error — discarding: %.120s", reply)
    return "I'm having trouble generating a response right now. Please try again."
}
if looksLikeJSON(reply) {
    log.Printf("[agent] llm reply looks like an unparsed JSON blob — discarding: %.120s", reply)
    return "I'm having trouble generating a response right now. Please try again."
}
if len(reply) < 20 {
    log.Printf("[agent] llm reply is suspiciously short (%d chars): %s", len(reply), reply)
}
return reply
```

**Why different actions per case:** Raw error strings and JSON blobs are unambiguously wrong and are replaced with a safe fallback. Short replies are only logged and passed through — replies like `"Yes."` or `"Done."` can be legitimately short in a conversational context, so discarding them would introduce false positives.

### Two refinements applied during the dev rebase

**`buildReply` trims the reply before the sanity checks.** All three checks are prefix-based, so a reply like `"\n\n{...}"` would have slipped past `looksLikeJSON` unmodified. `strings.TrimSpace` now runs once before the checks; the trimmed value is also what is returned, so leading/trailing whitespace never reaches the user.

**`looksLikeJSON` no longer discards every reply that starts with `[`.** A leading `{` is still treated as a misfire outright, but a leading `[` is ambiguous — markdown links (`[text](url)`) and citations (`[1] Smith et al.`) also begin with `[`. The function now only discards a `[`-leading reply when the whole string actually parses as JSON (`json.Valid`), so genuine answers with bracketed links or citations pass through while true JSON-array blobs are still caught. This adds the `encoding/json` import to `agents.go`.

---

## Rules for Future Work

These rules are derived directly from the gaps found above. Apply them whenever adding or modifying agent code.

**Never leave `dataContext` empty on a hard failure.** An empty context causes the LLM to answer in advisory mode without any signal that a service failed. Always inject a plain-language notice so the model can explain the situation to the user.

**Distinguish hard failures from soft failures.** A transport error or tool error payload means the service is broken. A successful response with no relevant results means the service is working but found nothing. These require different handling — only the former should inject an unavailability notice.

**Never pass `err.Error()` into `dataContext` directly or via a helper that may fall through.** Go error strings contain infrastructure details (hostnames, ports, OS-specific wording) that are meaningless to users and reveal internal topology to the LLM. Always wrap errors in a fixed plain-language string before injecting them into any context block.

**Test the system message, not just the reply.** The final reply in tests comes from the fake LLM server and can be anything. Use `capturingLLMServer` when the assertion is about what the agent sent to the model — that is where data context, unavailability notices, and error text actually live.

**Two error layers exist in `CallMCPTool` — handle both.** Transport/HTTP errors surface as a non-nil `err`. Tool-level errors (where the MCP server responded but reported a failure) surface as `isToolError(result) == true` on the returned string. Both are hard failures and both must be handled explicitly. Do not assume that a nil error means the result is safe to use without checking `isToolError`.

**Validate LLM output before returning it.** A non-error return from `callLLM` does not guarantee the content is a natural-language reply. Always check for raw error strings, JSON blobs, and suspiciously short output before passing the reply to the caller. Use `looksLikeRawError` and `looksLikeJSON` (defined in `agents.go`) — discard and return a safe fallback for clear misfires, log-and-pass-through for ambiguous cases like short replies.

**`looksLikeRawError` matches transport fragments as substrings, not just prefixes.** The canonical Go HTTP error leads with the method verb — `Post "URL": dial tcp ...: connection refused` — so a prefix-only check misses it and leaks the raw error to the user (caught by `TestRouteErrorContract`'s scheduling row). The unambiguous fragments (`dial tcp`, `connectex:`, `connection refused`, `mcp server unreachable`, `llm returned status`) are matched anywhere in the reply; only the ambiguous `"mcp "` token stays prefix-only, because it appears legitimately mid-answer ("the MCP server exposes…"). When adding a new marker, decide deliberately: pure-infrastructure token → substring; word that can occur in a real answer → prefix-only.

**Keep response format rules in the prompt files, not scattered across sections.** Citation style, list vs. prose guidance, and the answer length ceiling all live in `## Response Format` in each prompt file. If you need to change how OpsBot formats its answers, that is the only place to edit. Do not add conflicting format guidance to `## Tone` or `## Hard Limits`.

**The `## Tone` section opens with three shared sentences, identical across all four prompt files.** If a core tone rule needs to change, update all four files. Agent-specific tone additions follow the shared core and must not contradict it.
