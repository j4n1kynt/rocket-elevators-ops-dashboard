# CLAUDE.md Audit

This document classifies rules from CLAUDE.md into three categories:
- Always relevant (remain in CLAUDE.md)
- Must always execute (move to hooks)
- Scoped work (move to skills)

---

## Audit Table

| Rule | Category | Justification |
|------|----------|--------------|
| Spec-driven: update spec before implementation | Always relevant | Applies to all development workflows; guides all feature decisions |
| Data pipeline filters (ACTIVE + BY REQUEST) | Always relevant | Represents business logic used across all layers (ETL, server, analysis) |
| Join key: ElevatingDevicesNumber | Always relevant | Foundational for data modeling across all datasets and all project layers |
| Keep only most recent inspection per elevator | Always relevant | Core data modeling rule applied globally (ETL, server, analytics) |
| `elevator_fleet.csv` is the only server dataset; data flow architecture | Always relevant | Foundational architecture: intelligence notebooks generate → prepare_data.py writes → server reads; affects all layers |
| `prepare_data.py` is the only file that writes `elevator_fleet.csv` | Always relevant | Critical data-flow gatekeeper; ensures single source of truth; governs how all layers interact with unified dataset |
| Flask over FastAPI | Skill | Technology choice for platform; HTMX's HTML-fragment pattern fits Flask's template rendering directly |
| HTMX two-channel swap pattern: `#tableBody` (innerHTML) for rows; `#fleetTable` (outerHTML) for full table | Skill | Specific HTMX implementation detail; outerHTML strategy refreshes sort-button URLs when sort state changes |
| No custom JavaScript anywhere | Skill | Platform constraint; all interactivity driven by HTMX server-side logic, not client-side code |
| Summary card metrics computed at `GET /` page load, recomputed via OOB swaps on filter/search/sort | Skill | HTMX out-of-band fragment pattern; keeps cards in sync with filtered dataset without full reload |
| `GET /table` returns HTML fragments only | Skill | API contract for platform/frontend; response shape specifies `tableBody` (innerHTML swap) and `fleetTable` (outerHTML swap) with OOB card snippets |
| Never modify `/data` in place | Hook | Pre-commit hook blocks any commit touching `data/`; tracked at `scripts/hooks/pre-commit`, installed at `.git/hooks/pre-commit` |

---

## Hook Implementation - AND-104 TASK 4

**Status:** 3 hooks implemented using Claude Code's actual hook system (updated 2026-05-27)

### 1. File Protection (PreToolUse → Command Hook)

**Problem:** Accidental modification of `/data` source files breaks the data pipeline and violates the "never modify /data in place" rule.

**Solution:** PreToolUse command hook blocks Edit operations on `/data/**` files before execution.

**Why Hook:** Data protection must be enforced automatically — cannot rely on manual discipline when data integrity is at stake.

**Implementation:** 
- Hook type: `command` (runs `.claude/hooks/protect-data.sh`)
- Matcher: `Edit` tool only
- Filter: `if: "Edit(data/*)"` — only triggers on /data files
- Exit code 2 blocks the operation; exit 0 allows it

**Test:** Attempt to edit any file in `/data` returns a blocking error before the Edit tool runs.

---

### 2. Query Parameter Validation (PreToolUse → Async Echo Command)

**Problem:** Task 3 revealed runtime issues when API handlers accepted invalid query parameters (e.g., negative `limit` values). These bugs should be caught at code-review time, not runtime.

**Solution:** PreToolUse command hook prints a non-blocking reminder when editing API handlers, prompting conscious consideration of parameter bounds.

**Why Hook:** Reminds Claude to verify parameter bounds are enforced before the edit completes; prevents silent regressions without blocking the workflow.

**Implementation:** 
- Hook type: `command` (async echo — non-blocking reminder)
- Matcher: `Edit` tool on `platform/api/handlers.go`
- Prints: "Reminder: Validate query parameters (limit >= 1) when editing handlers.go"
- `async: true` — does not block the edit

**Test:** Edit handlers.go and observe the reminder message printed before/during the edit.

---

### 3. Content-Type Enforcement (PreToolUse → Async Echo Command)

**Problem:** API responses must include `Content-Type: application/json` header. Developers might bypass the `writeJSON()` helper and set headers manually.

**Solution:** PreToolUse command hook prints a non-blocking reminder to use the `writeJSON()` helper instead of raw JSON encoding.

**Why Hook:** Ensures consistency through conscious choice rather than relying on code review to catch every instance.

**Implementation:** 
- Hook type: `command` (async echo — non-blocking reminder)
- Matcher: `Edit` tool on `platform/api/handlers.go`
- Prints: "Reminder: Use writeJSON() helper — do not call json.NewEncoder(w).Encode directly"
- `async: true` — does not block the edit

**Test:** Edit handlers.go and observe the reminder message printed before/during the edit.

---

### 4. Go Formatting (PostToolUse → Command Hook)

**Problem:** Inconsistent code formatting during Go development creates noise in diffs and review burden.

**Solution:** PostToolUse command hook automatically runs `gofmt -w` on the edited file path after every Edit tool completes. `gofmt` exits cleanly on non-Go files, so the hook is safe to run without a file-type filter.

**Why Hook:** Formatting is deterministic and should be automated. Keeps commits clean and eliminates formatting from code review.

**Implementation:** 
- Hook type: `command` (runs `.claude/hooks/gofmt.sh`)
- Matcher: `Edit` tool (all file edits — no `if` filter; `gofmt` handles non-Go files gracefully)
- Status message: "Formatting Go code..."
- Timeout: 10 seconds

**Test:** Edit a `.go` file and observe it's automatically formatted by `gofmt` after the edit completes.

---

---

### 5. Task Completion Notification (Stop → Async Command Hook)

**Problem:** No visibility into when Claude Code finishes a task in the background, especially during long multi-file operations.

**Solution:** Stop hook sends a Windows system notification message when Claude Code's session ends.

**Why Hook:** Provides ambient awareness without interrupting the workflow; useful during long tasks.

**Implementation:**
- Hook type: `command` (runs `msg * "Claude Code: Task completed"`)
- Matcher: Stop event (fires when Claude Code session ends)
- `async: true` — fires after session close, does not delay it

**Test:** Complete a Claude Code task and observe the Windows system message popup.

---

## Summary

**Hook Coverage:** 5 implemented hooks

**Distribution by Type:**
- **PreToolUse (3 hooks):** File protection (command, blocking), query parameter validation (async echo command), content-type enforcement (async echo command)
- **PostToolUse (1 hook):** Go formatting (command)
- **Stop (1 hook):** Task completion notification (async command)

**Key Implementation Decisions:**
1. File protection uses a **command hook** that exits with code 2 to block; data/ files cannot be edited
2. Validation hooks use **async echo commands** as non-blocking reminders — they print guidance without interrupting the edit
3. Go formatting uses a **command hook** that runs `gofmt` automatically post-edit on all files; `gofmt` handles non-Go files gracefully so no file-type filter is needed
4. Task notification uses the **Stop hook** to fire a Windows system message on session end
5. All hooks use proper Claude Code format: `matcher` → `hooks` array with `type`, `command`, etc.

**Hook Framework Compatibility:** All hooks follow Claude Code's actual hook specification from https://code.claude.com/docs/en/hooks
