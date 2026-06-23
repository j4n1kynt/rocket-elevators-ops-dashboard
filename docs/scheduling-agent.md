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

## Double-check review (Task 1)

After both subtasks were implemented, a review of `router.go` and `agents.go` found no bugs. The following observations were recorded:

- **`mcpCtx` created before the guard:** In Phase 1, the context timeout is allocated unconditionally before the guard check. If the guard fires and blocks the MCP call, the context is created but never used — `defer mcpCancel()` still runs safely on return. Minor wasted allocation, not a bug.
- **`AllowedTools` not directly consumed by the agent:** The guard compares `toolName` directly rather than iterating `req.AllowedTools`. Both checks are independently correct and work toward the same goal.
- **Phase 2 needs no guard:** Phase 2 hardcodes `"schedule_inspection"` directly instead of going through `buildMCPArgs`, so there is no path through which a forbidden tool name could be introduced.
- **Re-dispatch path is clean:** When a pending action is abandoned, the agent re-dispatches through `Route` without setting `AllowedTools`. `Route` reclassifies the message and assigns tools correctly for whatever agent handles it.
