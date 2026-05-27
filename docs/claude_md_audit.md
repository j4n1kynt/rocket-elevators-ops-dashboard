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

### 2. Query Parameter Validation (PreToolUse → Prompt Hook)

**Problem:** Task 3 revealed runtime issues when API handlers accepted invalid query parameters (e.g., negative `limit` values). These bugs should be caught at code-review time, not runtime.

**Solution:** PreToolUse prompt hook asks for confirmation when editing API handlers, specifically about parameter validation.

**Why Hook:** Prompts Claude to consciously consider whether parameter bounds are enforced; prevents silent regressions.

**Implementation:** 
- Hook type: `prompt` (asks yes/no verification question)
- Matcher: `Edit` tool on `platform/api/handlers.go`
- Prompts: "Does this change validate query parameters with lower bounds?"
- Timeout: 30 seconds

**Test:** Edit handlers.go and observe the prompt asking about parameter validation before file is modified.

---

### 3. Content-Type Enforcement (PreToolUse → Prompt Hook)

**Problem:** API responses must include `Content-Type: application/json` header. Developers might bypass the `writeJSON()` helper and set headers manually.

**Solution:** PreToolUse prompt hook reminds developers to use the `writeJSON()` helper instead of raw JSON encoding.

**Why Hook:** Ensures consistency through conscious choice rather than relying on code review to catch every instance.

**Implementation:** 
- Hook type: `prompt` (asks for confirmation)
- Matcher: `Edit` tool on `platform/api/handlers.go`
- Prompts: "Did you use writeJSON() helper instead of json.NewEncoder(w).Encode?"
- Timeout: 30 seconds

**Test:** Edit handlers.go and observe the prompt asking about JSON encoding approach before file is modified.

---

### 4. Go Formatting (PostToolUse → Command Hook)

**Problem:** Inconsistent code formatting during Go development creates noise in diffs and review burden.

**Solution:** PostToolUse command hook automatically runs `gofmt -w` on modified `.go` files after Edit tool completes.

**Why Hook:** Formatting is deterministic and should be automated. Keeps commits clean and eliminates formatting from code review.

**Implementation:** 
- Hook type: `command` (runs `gofmt -w ${file_path}`)
- Matcher: `Edit` tool
- Filter: `if: "Edit(*.go)"` — only runs on Go files
- Status message: "Formatting Go code..."
- Timeout: 10 seconds

**Test:** Edit a `.go` file and observe it's automatically formatted by `gofmt` after the edit completes.

---

## Summary

**Hook Coverage:** 3 implemented hooks (4th — Task Completion Awareness — cannot be implemented as "Stop" is not a real Claude Code event type)

**Distribution by Type:**
- **PreToolUse (3 hooks):** File protection (command), query parameter validation (prompt), content-type enforcement (prompt)
- **PostToolUse (1 hook):** Go formatting (command)

**Key Implementation Decisions:**
1. File protection uses a **command hook** that exits with code 2 to block; data/ files cannot be edited
2. Validation hooks use **prompt hooks** for conscious decision-making rather than silent pattern matching
3. Go formatting uses a **command hook** that runs `gofmt -w` automatically post-edit
4. All hooks use proper Claude Code format: `matcher` → `hooks` array with `type`, `command`/`prompt`/etc.

**Hook Framework Compatibility:** All hooks follow Claude Code's actual hook specification from https://code.claude.com/docs/en/hooks
