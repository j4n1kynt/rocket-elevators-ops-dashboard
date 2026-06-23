# Scheduling Agent — Runbook

## Overview

The scheduling agent handles inspection scheduling via a two-phase confirmation flow (spec §7.2). It is the only agent with a write tool and the only one whose `AllowedTools` restriction is enforced at runtime.

## Flow

| Phase | Trigger | What happens |
|---|---|---|
| Phase 1 | User requests an inspection; no `PendingAction` in request | Calls `schedule_inspection(confirmed=false)` → preview returned; `PendingAction` with HMAC signature set on response |
| Phase 2 | User replies "yes"; `PendingAction` echoed back | HMAC verified, TTL checked, `schedule_inspection(confirmed=true)` called → database write |
| Cancel | User replies "cancel" | No MCP call; `[ACTION CANCELLED]` context set; `PendingAction` cleared |
| Abandon | Ambiguous reply | Re-dispatched through `Route`; original `PendingAction` discarded |

## Tool restriction

`router.go` sets `req.AllowedTools = []string{"schedule_inspection"}` for every path that invokes the scheduling agent. The agent enforces this via `toolAllowed(req.AllowedTools, toolName)` in Phase 1, blocking any tool name that `buildMCPArgs` returns other than `schedule_inspection`.

Phase 2 hardcodes `"schedule_inspection"` directly and never goes through `buildMCPArgs`, so no guard is needed there.

Cross-agent gating for the data, knowledge, and general agents is tracked as tech debt in AND-109.

## Acceptance criteria (S3-5)

| Criterion | How it is satisfied |
|---|---|
| Scheduling agent can use only `schedule_inspection` | Router sets `AllowedTools`; `toolAllowed()` enforces it in Phase 1 |
| Agent shows a preview before any write | Phase 1 calls `schedule_inspection(confirmed=false)` and returns `PendingAction` |
| Agent writes only after explicit confirmation | Phase 2 calls `schedule_inspection(confirmed=true)` only after HMAC-verified "yes" |
| Cancel writes nothing | Cancel path sets `[ACTION CANCELLED]` context without touching MCP |
| Tool restriction holds under adversarial input | Guard fires even when a data keyword causes `buildMCPArgs` to return a forbidden tool |

Note: `prompts/scheduling_prompt.md` (go:embed) shipped in #55 and is not part of S3-5.

## Pending-action TTL and signature

`PendingAction.ExpiresAt` is a Unix timestamp set to `now + pendingActionTTL` in Phase 1 and checked in Phase 2. `PendingAction.Signature` is an HMAC over `(elevator_id, inspection_date, inspection_type, reason)` — Phase 2 rejects any action whose signature does not verify.
