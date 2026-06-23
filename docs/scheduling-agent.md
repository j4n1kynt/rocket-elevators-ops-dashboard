# Scheduling Agent — S3-5

## Task 1: AllowedTools enforcement in router.go

### What was done

`router.go` now sets `req.AllowedTools = []string{"schedule_inspection"}` in both paths that invoke the scheduling agent: the `pending_action` pre-emption path (Phase 2 confirmations) and the normal `action_executor` routing path (Phase 1 requests). The TODO comment left from S3-3 was removed and replaced with an explanatory comment scoped to the scheduling agent.

### Decision

Only the scheduling agent is explicitly listed in the router's `AllowedTools` gate. The other agents (data, knowledge, general) scope themselves internally — the data agent calls only data tools via `buildMCPArgs`, the knowledge agent hardcodes its two search tools, and the general agent calls no tools. Adding explicit `AllowedTools` for those agents is a future hardening step, not a requirement for S3-5.

### Potential issues

The `AllowedTools` field is populated by the router but enforced inside `schedulingAgent` by comparing the tool name directly against the string `"schedule_inspection"` — it does not read `req.AllowedTools`. The two layers are complementary: the router declares intent, the agent enforces it. `AllowedTools` would become directly consumed if a shared enforcement utility were introduced across agents in the future.

---

## Task 1 (continued): AllowedTools guard in agents.go

### What was done

In `schedulingAgent`'s Phase 1 path in `agents.go`, a guard was added immediately after `buildMCPArgs` returns. If the tool name is anything other than `schedule_inspection`, the agent logs the blocked call, sets an `[ACTION VALIDATION ERROR]` data context, and skips the MCP call entirely. The normal `else if` / `else` chain that calls MCP and processes the result only runs when the tool name is `schedule_inspection`.

### Decision

The guard checks the tool name returned by `buildMCPArgs` rather than reading `req.AllowedTools`. `buildMCPArgs` is the only code path that produces the tool name in Phase 1, so checking its output directly is simpler and does not require passing the allowed list through additional function signatures. The two checks (router setting `AllowedTools`, agent guarding on tool name) are complementary: the router declares intent, the agent enforces it.

### Bug caught during implementation

The initial edit dropped the `mcpCtx, mcpCancel := context.WithTimeout(...)` and `defer mcpCancel()` lines that were part of the replaced block. This left `mcpCtx` undefined and broke compilation. The lines were restored immediately in a follow-up edit, placed before the guard so the context is available to the `else if` MCP call branch and the cancel is deferred regardless of which branch runs.

---

## Task 2: Scheduling agent tests

### Test 1 — Phase 1 preview (`TestSchedulingAgentPhase1Preview`)

**What was done**

Added `TestSchedulingAgentPhase1Preview` to `agents_test.go`. It sends the message "schedule an inspection for elevator 12345 on 2026-07-01" to `schedulingAgent` with no `PendingAction` set and a mock MCP server that returns `{"pending_confirmation": true, "summary": "..."}` for `schedule_inspection`. The test asserts:
- `AgentName` is `"scheduling"`
- Exactly one MCP call was made and it was to `schedule_inspection`
- The call carried `confirmed=false` (Phase 1 — no write)
- `PendingAction` is non-nil (preview returned)
- `PendingAction.Signature` is set (required for Phase 2 verification)

**Decision: extend `trackingMCPServer` to capture call arguments**

The existing `trackingMCPServer` only recorded tool names. To assert `confirmed=false`, it was extended with a `callArgs []map[string]any` field, an updated handler that parses `params.arguments` from each `tools/call` request, and a new `CallArgs()` method. This is backward compatible — existing tests never call `CallArgs()`.

**Decision: message format**

The message uses a numeric elevator ID (12345) and an ISO date (2026-07-01). The design doc explicitly states the `E`-prefix format (e.g. `E12345`) is not recognized by `extractElevatorIDs`. The numeric-only format is required for the classifier to extract an ID.

**Why this message classifies as IntentAction**

`ClassifyIntent` scores the message as follows: "schedule" keyword → 1.0; elevator ID present → +0.5 (IntentAction) and +0.5 (IntentDataQuery); date present → +0.5 (IntentAction); action verb + concrete target → +1.0. Final IntentAction score: 3.0, confidence: 0.75, well above the 0.6 floor.

### Test 2 — Phase 2 confirmed write (`TestSchedulingAgentPhase2WritesAfterConfirmation`)

**What was done**

Added `TestSchedulingAgentPhase2WritesAfterConfirmation` to `agents_test.go`. It builds a `PendingAction` with a future `ExpiresAt` and a valid HMAC signature computed via `signPendingAction` (same package), then calls `schedulingAgent` with message `"yes"` and that action attached. The mock MCP server returns `{"success":true,"inspection_id":42,...}` for `schedule_inspection`. The test asserts:
- `AgentName` is `"scheduling"`
- Exactly one MCP call was made to `schedule_inspection`
- The call carried `confirmed=true` (Phase 2 — write authorised)
- `PendingAction` in the response is nil (pending state cleared after write)
- `Reply` is non-empty (outcome reported to the user)

**Decision: use `signPendingAction` directly**

Since the test is in `package main`, `signPendingAction` is accessible without exporting it. This keeps the signing logic as the single source of truth — the test produces exactly the signature the agent will verify, with no duplication of the HMAC logic.

**Decision: add `"time"` import**

`time.Now().Add(10 * time.Minute)` is needed to produce a non-expired `ExpiresAt`. The import was added to the test file.

### Test 3 — Cancellation (`TestSchedulingAgentCancelWritesNothing`)

**What was done**

Added `TestSchedulingAgentCancelWritesNothing` to `agents_test.go`. It builds a valid `PendingAction` (same pattern as Test 2), then calls `schedulingAgent` with message `"cancel"`. The test asserts:
- Zero MCP tool calls were made
- `schedule_inspection` with `confirmed=true` was specifically never called (belt-and-suspenders over the zero-calls check)
- `PendingAction` in the response is nil
- `Reply` is non-empty

**Decision: assert zero calls, not just "no confirmed=true"**

The cancel path in `agents.go` sets the `[ACTION CANCELLED]` data context directly and returns without touching MCP at all. Asserting `len(calls) == 0` is the stronger and more honest assertion — it catches any accidental MCP call, not just a confirmed write. The secondary per-call check for `confirmed=true` is kept to directly match the acceptance criterion wording.

### Test 4 — Tool restriction (`TestSchedulingAgentNeverCallsForbiddenTools`)

**What was done**

Added `TestSchedulingAgentNeverCallsForbiddenTools` to `agents_test.go`. It sends the message "what is the risk for elevator 12345 on 2026-07-01" directly to `schedulingAgent` and asserts that none of the nine forbidden tools (seven data tools and two knowledge search tools) appear in the MCP call log.

**Why this message**

The message contains a numeric elevator ID and an ISO date (so it passes the missing-info check and reaches `buildMCPArgs`), but leads with the data keyword `"risk"`. The scoring is: `"risk"` → IntentDataQuery +1.0; elevator ID → IntentDataQuery +0.5, IntentAction +0.5; date → IntentAction +0.5; no scheduling verb → no +1.0 action bonus. Final: IntentDataQuery 1.5 vs IntentAction 1.0 — IntentDataQuery wins. `buildMCPArgs` returns `get_elevator_risk`. The guard fires and blocks it. The test log confirms: `blocked forbidden tool "get_elevator_risk"`.

**Decision: empty tool payload map**

Unlike the Phase 1 test, no `schedule_inspection` payload is registered in the mock server. If the guard somehow failed and the forbidden tool reached MCP, the server would return an empty-results default — the test would still catch the forbidden call name in the loop.

---

## Double-check review (Task 1)

After both subtasks were implemented, a review of `router.go` and `agents.go` found no bugs. The following observations were recorded:

- **`mcpCtx` created before the guard:** In Phase 1, the context timeout is allocated unconditionally before the guard check. If the guard fires and blocks the MCP call, the context is created but never used — `defer mcpCancel()` still runs safely on return. Minor wasted allocation, not a bug.
- **`AllowedTools` not directly consumed by the agent:** The guard compares `toolName` directly rather than iterating `req.AllowedTools`. Both checks are independently correct and work toward the same goal.
- **Phase 2 needs no guard:** Phase 2 hardcodes `"schedule_inspection"` directly instead of going through `buildMCPArgs`, so there is no path through which a forbidden tool name could be introduced.
- **Re-dispatch path is clean:** When a pending action is abandoned, the agent re-dispatches through `Route` without setting `AllowedTools`. `Route` reclassifies the message and assigns tools correctly for whatever agent handles it.

---

## S3-5 completion status

All acceptance criteria are met:

| Criterion | How it is satisfied |
|---|---|
| Scheduling agent can use only `schedule_inspection` | Router sets `AllowedTools`; Phase 1 guard blocks any other tool name returned by `buildMCPArgs` |
| Agent shows a preview before any write | Phase 1 calls `schedule_inspection(confirmed=false)` and returns `PendingAction` — tested by `TestSchedulingAgentPhase1Preview` |
| Agent writes only after explicit confirmation | Phase 2 calls `schedule_inspection(confirmed=true)` only after HMAC-verified "yes" — tested by `TestSchedulingAgentPhase2WritesAfterConfirmation` |
| Cancel writes nothing | Cancel path sets `[ACTION CANCELLED]` context without touching MCP — tested by `TestSchedulingAgentCancelWritesNothing` |
| Agent has its own focused system prompt | `prompts/scheduling_prompt.md` — embedded at build time via `go:embed` |
| Tool restriction holds under adversarial input | Guard fires even when a data keyword causes `buildMCPArgs` to return a forbidden tool — tested by `TestSchedulingAgentNeverCallsForbiddenTools` |

**Files changed:** `platform/api/router.go`, `platform/api/agents.go`, `platform/api/agents_test.go`
