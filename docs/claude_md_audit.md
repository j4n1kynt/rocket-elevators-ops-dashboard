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

**Status:** 4 hooks finalized in `.claude/settings.json`

### 1. File Protection (PreToolUse)

**Problem:** Accidental modification of /data source files breaks the data pipeline and violates the "never modify /data in place" rule.

**Solution:** PreToolUse hook blocks all write operations to `/data` directory.

**Why Hook:** Data protection must be enforced automatically — cannot rely on manual discipline when data integrity is at stake.

**Implementation:** `settings.json` PreToolUse rule targets `data/` with action `deny`.

**Test:** Attempt to modify any file in `/data` is blocked before tool execution.

---

### 2. Query Parameter Validation (PreToolUse)

**Problem:** Task 3 revealed runtime issues when API handlers accepted invalid query parameters (e.g., negative `limit` values). These bugs should be caught at code-review time, not runtime.

**Solution:** PreToolUse hook scans Go API handlers for unsafe parameter patterns and warns on detection.

**Why Hook:** Catches logic errors before they reach the runtime; prevents API contract violations from shipping.

**Implementation:** `settings.json` PreToolUse rule targets `platform/api/**/*.go` files and flags patterns like `limit < 0` or `negative.*limit`.

**Test:** Modified API handler with invalid parameter check; warning triggered on pattern match.

---

### 3. Go Formatting (PostToolUse)

**Problem:** Inconsistent code formatting during Go development creates noise in diffs and review burden.

**Solution:** PostToolUse hook runs `gofmt` on all modified `.go` files automatically.

**Why Hook:** Formatting is deterministic and should be enforced, not optional. Keeps commits clean and reviewable.

**Implementation:** `settings.json` PostToolUse rule targets `**/*.go` and runs `gofmt -w {{file}}` after tool use.

**Test:** Modified a `.go` file and verified `gofmt` reformatted it automatically.

---

### 4. Task Completion Awareness (Stop)

**Problem:** No workflow visibility when Claude finishes a task — can lead to lost context or forgotten follow-ups.

**Solution:** Stop hook triggers a notification when task execution ends.

**Why Hook:** Provides workflow feedback without relying on polling; improves task handoff clarity.

**Implementation:** `settings.json` Stop hook with action `notify`.

**Test:** Observed notification generated on task completion.

---

## Summary

**Hook Coverage:** 4 hooks, all in `.claude/settings.json`

**Distribution by Type:**
- **PreToolUse (2 hooks):** File protection, query parameter validation — enforce constraints before tool runs
- **PostToolUse (1 hook):** Go formatting — maintain code quality after modifications  
- **Stop (1 hook):** Task completion awareness — improve workflow visibility

**Key Decisions:**
- File protection is non-negotiable (enforced hook, not skill)
- Query parameter validation caught real Task 3 bugs and prevents regression
- Formatting is deterministic and automated (never manual)
- Task completion awareness is UX improvement (Stop notification)
