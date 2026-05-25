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

## Summary

**Audit Coverage:** 12 rules, 100% of CLAUDE.md Conventions section

**Distribution:**
- **Always relevant (6 rules):** General principles + cross-cutting architecture (spec-driven, data flow, dataset strategy, data modeling)
- **Skill (5 rules):** Platform-specific implementation (Flask, HTMX patterns, JS constraint, API contracts)
- **Hook (1 rule):** Data protection — pre-commit hook implemented at `scripts/hooks/pre-commit`

**Key Insights:**
- Data architecture rules (`elevator_fleet.csv` flow, `prepare_data.py` gatekeeper) are foundational, not platform-specific
- HTMX interaction patterns are tightly coupled (two-channel swap, OOB card recomputation) and live in the platform skill
- New team members must run `cp scripts/hooks/pre-commit .git/hooks/pre-commit` to install the hook locally
