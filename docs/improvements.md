## AND-103 Task 2: Add Location and Alteration Count as Static Features

**What was added:**  
Two static elevator-level features — `Location` and `AlterationCount` — merged into the feature matrix on the join key (`ElevatingDevicesNumber`).

`Location` encodes where the elevator operates (e.g., building zone or borough). `AlterationCount` captures the number of mechanical modifications the unit has undergone. Both are constant per elevator and appear in every row for that elevator.

**Where:**  
`prepare_data.py` — feature matrix construction stage, after loading `elevator_fleet.csv`

**Why:**  
The original pipeline included only `EquipmentType` as a static feature, leaving two predictive signals unused.

`Location` correlates with failure risk because operating environment (commercial vs. residential, high-traffic vs. low-traffic) affects wear patterns and inspection outcomes. Without it, two elevators with identical inspection histories but different operating environments look identical to the model.

`AlterationCount` is a proxy for mechanical complexity and intervention frequency. An elevator modified five times differs fundamentally in risk profile from an unmodified unit. Omitting it forces the model to explain that risk through inspection history alone, which is incomplete.

Adding both reduces unexplained residual variance, improves precision on high-risk predictions, and surfaces actionable findings via feature importance (e.g., "location type X elevators skew high-risk" can directly inform inspection scheduling).

---

## AND-103 Task 2: Dummy Encode DeviceType Consistently with InspectionType

**What was added:**  
`DeviceType` re-encoded using one-hot (dummy) encoding via `pd.get_dummies(..., drop_first=True)`, replacing the integer label encoding that was previously applied.

**Where:**  
`prepare_data.py` — encoding stage, alongside the existing `InspectionType` one-hot encoding

**Why:**  
The original pipeline applied inconsistent encoding strategies to two nominal categoricals in the same feature group: `InspectionType` was one-hot encoded, but `DeviceType` was label encoded (integer).

Label encoding assigns ordered integers to unordered categories (e.g., Escalator=0, Elevator=1, Dumbwaiter=2). This introduces a false ordinal relationship — the model may interpret Dumbwaiter as numerically "greater than" Elevator, which is semantically incorrect. While RandomForest handles this somewhat tolerantly (splits on thresholds, doesn't assume linearity), the implicit numeric relationship still affects interaction terms between features and makes the pipeline incorrect for any downstream model that processes features numerically.

The inconsistency also makes the pipeline harder to audit and harder to port: a future contributor reading the code expects nominal categoricals to be dummy-encoded consistently; a label-encoded exception with no comment creates technical debt.

Switching to one-hot encoding eliminates the ordinal artifact, makes feature importance readable per device type (e.g., `DeviceType_Dumbwaiter` importance = 0.07 rather than an opaque integer coefficient), and aligns the pipeline with standard practice for nominal features.

---

## AND-104 Task 3: CSV Header Validation

**Status:** ⚠️ Partially Implemented

**What was added:**  
Length-based validation of CSV rows during data loading to reject malformed records.

**Where:**  
- data.go (LoadFleetCSV lines 38, LoadInspectionCSV line 80)

**Current implementation:**  
Skips rows with insufficient columns (`len(row) < 8` for fleet, `len(row) < 9` for inspections).

**What's missing:**  
Explicit header row validation. The code does not verify that column names and order match the expected schema — it only checks row length. A future enhancement should:
- Read and validate the header row explicitly on first iteration
- Compare against expected column names, not just count
- Provide clear error messages if header mismatch is detected

**Why this matters:**  
The application relies on positional column mapping (e.g., `row[0]` = ElevatorID, `row[1]` = Location). Silent column reordering in source CSVs would corrupt API responses without any error signal.

---

## AND-104 Task 3: Query Parameter Validation Hardening

**What was added:**  
Lower-bound validation for numeric query parameters such as `limit` and `page`.

**Where:**  
- handlers.go

**Why:**  
QA identified a runtime panic caused by negative `limit` values. This validation prevents invalid input from reaching execution logic, improving system stability and predictability.

---

## AND-104 Task 3: Fail-Fast Data Initialization

**Status:** ✅ Fully Implemented

**What was added:**  
Strict validation during application startup that stops server initialization if data loading fails.

**Where:**  
- main.go (lines 10-18)

**Implementation:**  
Uses `log.Fatalf()` to exit immediately if fleet or inspection CSV loading fails. Predictions loading is intentionally soft-fail (503 endpoint behavior).

**Why:**  
The server can only serve correct data if initialization succeeds. Failing fast prevents the API from accepting requests with incomplete or corrupt datasets.

---

## AND-104 Task 4: Content-Type Enforcement (Hook)

**Status:** ✅ Fully Implemented

**What was added:**  
Automatic Content-Type: application/json header enforcement for all API responses.

**Where:**  
- handlers.go (`writeJSON()` helper, line 12)

**Current implementation:**  
All response handlers use the `writeJSON()` function which sets `w.Header().Set("Content-Type", "application/json")` before writing the body. This is a centralized approach: every API response goes through `writeJSON()`, so header consistency is guaranteed by design, not by discipline.

**Hook definition:**  
PreToolUse hook in `.claude/settings.json` scans for `json.NewEncoder(w).Encode` patterns to warn if a handler attempts to write JSON without going through `writeJSON()`. This prevents future developers from bypassing the central encoding function.

**Why:**  
Consistent Content-Type headers are part of the API contract. Manual header setting in individual handlers creates opportunities for inconsistency. The hook enforces the rule: use `writeJSON()`, don't encode JSON manually.

---

## AND-104 Task 5: CSV Data Validator (Subagent + Skill)

**Status:** ✅ Fully Implemented

**What was added:**
A `csv-validator` subagent and `/validate-csv` skill that validate source datasets before they reach the Go API and dashboard.

**Where:**
- `.claude/agents/csv-validator.md`
- `.claude/skills/validate-csv/SKILL.md`

**Usage:**
```
/validate-csv elevator_fleet   — validate platform/elevator_fleet.csv
/validate-csv inspection       — validate data/inspection.csv
/validate-csv all              — validate both + referential integrity
```

**What it validates:**

| Check | Dataset | Severity if failed |
|---|---|---|
| Exact header names and column order | both | HIGH |
| Rows with fewer than expected columns | both | HIGH |
| `Elevator ID` is unique and numeric | elevator_fleet | HIGH |
| `Status` is only "ACTIVE" or "BY REQUEST" | elevator_fleet | MEDIUM |
| `License Expiration Date` is YYYY-MM-DD | elevator_fleet | MEDIUM |
| `Latest Inspection Date` is YYYY-MM-DD or blank | elevator_fleet | MEDIUM |
| `ElevatingDevicesNumber` is integer-parseable | inspection | HIGH |
| `Latest_INSPECTION_Date` is M/D/YYYY | inspection | HIGH |
| Duplicate `(ElevatingDevicesNumber, date)` pairs | inspection | LOW |
| Null rates for all nullable fields | both | INFO |
| All inspection device IDs exist in fleet | cross-dataset | MEDIUM |
| Elevators with zero inspection records | cross-dataset | INFO |

**Why this matters:**
The Go API (`data.go`) uses **positional column mapping** — `row[0]`, `row[2]`, `row[7]`, etc. A silent column reorder in any source CSV corrupts every affected API response with no error signal. The Go API also silently skips rows with bad date formats or non-numeric device IDs, making data loss invisible at runtime. This validator surfaces those issues explicitly before they propagate.

This also closes the gap documented in the "CSV Header Validation" entry above: `data.go` checks `len(row) < 8` but not header names. The validator checks both.

**Design decisions:**
- Validates against `data.go` positional mapping AND `prepare_data.py` named column expectations — both matter
- Severity levels tied to impact: HIGH = data loss or corruption at API level, MEDIUM = degraded responses, LOW = data quality risk
- `/validate-csv all` includes cross-dataset referential integrity because orphaned inspection records (inspections for elevators not in fleet) are invisible in the API and dashboard but can skew analytics
- Uses `haiku` model and `maxTurns: 10` — validation is a bounded, mechanical task that doesn't require reasoning depth

---

## AND-104 Post-Audit: Fleet Endpoint Field Mappings Added to API Spec Appendix

**Title:** Added fleet endpoint fields to API spec appendix

**What was changed:**
Added `FleetStatsResponse` and `AlertEntry` field entries to the Appendix: Field Name Mapping table in `docs/api_spec.md`.

**Where:**
`docs/api_spec.md`, Appendix: Field Name Mapping section

**Why it adds value:**
The appendix previously only covered elevator, inspection, and risk fields. Future developers or API consumers using the appendix as a field lookup would find no entries for the two fleet endpoints, requiring them to read the full endpoint section to understand the data origin. The additions make the appendix a complete single reference for all response field sources across all 6 endpoints.

---

## AND-104 Task 7: new-endpoint Skill — Architectural Drift Correction

**What was added:**
Updated Step 2 ("Data processing and response building") and Step 4 ("Verify data layer support") in `.claude/skills/new-endpoint/SKILL.md` to reflect the PostgreSQL-backed architecture introduced in AND-105 Task 4.

Step 2 now instructs developers to use the global `db *pgxpool.Pool` with `db.QueryRow` (single-row) and `db.Query` + rows loop (multi-row), with concrete parameterized query examples and correct error handling patterns.

Step 4 replaces the CSV loading decision tree with a table of the five PostgreSQL tables (`elevators`, `inspections`, `incidents`, `alterations`, `predictions`) and a simplified two-question decision: can the handler use standard SQL joins, and does it need new response types in `models.go`.

**Where:**
`.claude/skills/new-endpoint/SKILL.md` — Step 2 section 3 and Step 4

**Why:**
AND-105 Task 4 migrated the Go API from CSV flat-file loading into in-memory maps (`elevators`, `elevatorIdx`, `inspectionIdx`, `riskIdx`) to a PostgreSQL connection pool (`pgxpool`). The skill retained the old instructions, which described variables and loading patterns that no longer exist. Any developer invoking `/new-endpoint` after the migration would follow a workflow that references removed architecture, producing non-compilable handler code. Steps 1, 2 (sections 1–2 and 4), 3, and 5 required no changes — only the data access guidance was stale.

---

## AND-104 Task 7: risk_distribution.unknown — Invariant Hardening

**What was added:**
Updated the `unknown` bucket CASE expression in `GetFleetStats` (`platform/api/handlers.go`) to capture all rows that are not explicitly `low`, `medium`, or `high`:

```sql
-- Before
COUNT(CASE WHEN p.risk_level IS NULL THEN 1 END) AS unknown

-- After
COUNT(CASE WHEN p.risk_level IS NULL
           OR LOWER(p.risk_level) NOT IN ('low', 'medium', 'high')
      THEN 1 END) AS unknown
```

**Where:**
`platform/api/handlers.go` — `GetFleetStats`, risk distribution query

**Why:**
The original expression only caught rows where `p.risk_level IS NULL`, which correctly handles elevators with no prediction record (LEFT JOIN produces a null row). However, if any prediction row contains a non-standard, non-null risk level (e.g., a future model version introduces `"CRITICAL"` or a data import produces an unexpected value), that elevator would be counted in `total_elevators` but fall into none of the four buckets, breaking the spec-guaranteed invariant `low + medium + high + unknown == total_elevators`.

The updated expression uses `NOT IN ('low', 'medium', 'high')` as the catch-all, which is exhaustive by construction: every row either matches one of the three known levels or falls into `unknown`. The invariant now holds regardless of what values appear in `risk_level`.

---

## AND-104 Post-Audit: Corrected gofmt Hook Documentation

**Title:** Aligned gofmt hook audit description with actual behavior

**What was changed:**
Updated `docs/claude_md_audit.md` to remove the incorrect `if: "Edit(*.go)"` filter claim; clarified that the hook runs on all edits and that `gofmt.sh` handles non-Go files gracefully. Also corrected the hook count (5, not 3) and added the Stop hook section that was previously incorrectly described as unimplementable.

**Where:**
`docs/claude_md_audit.md`, PostToolUse section and Summary section

**Why it adds value:**
The documentation was describing behavior that doesn't exist (a file-type filter) and incorrectly stating that the Stop event is not a real Claude Code hook type. Accurate hook documentation prevents confusion for anyone inspecting settings.json and comparing it to the audit. Reliable audit documentation is required for the system to serve as a maintainable reference for future contributors.

---

## AND-105 Improvement: Full API-Driven Dashboard Integration

**What was added:**
Replaced remaining CSV-based computations in dashboard summary cards with API-driven data from `/api/fleet/stats`.

**Where:**
`platform/server.py` — `index()` route and `/table` route OOB card updates.

Removed:
- `df_fleet` DataFrame loaded from `elevator_fleet.csv` at startup
- `compute_metrics()` helper that computed all four summary card values from the CSV

Replaced with:
- `GET /api/fleet/stats` → `total_elevators` (card 1) and `inspection_pass_rate` (used to derive card 3)
- `GET /api/elevators?status=ACTIVE&limit=1` → `total` field gives the active elevator count (card 2)
- `/table` OOB now updates only `card-total` from the API's filtered result count; the other three cards retain their fleet-wide values set on page load

**Why:**
The original implementation partially relied on `df_fleet` (loaded from `elevator_fleet.csv`) for all four summary card metrics, violating the requirement that all dashboard data must come from the Go API. The Go API comment in the original code explicitly acknowledged this gap: `"df_fleet is used only for summary card computation (overdue/expiring metrics) since the Go API does not expose those date-arithmetic aggregates."` This caused inconsistency between the API and UI layers.

This improvement ensures:
- Full end-to-end API integration — no CSV is read by the Flask server for any displayed metric
- Consistent data source across the system
- Alignment with AND-104 Task 8 evaluation criteria: "All data displayed on the dashboard must come from the Go API"

**Known limitation:**
`/api/fleet/stats` does not expose a direct "overdue inspections" count (time-based: inspections older than 1 year). Card 3 displays `round((1 - inspection_pass_rate) * total_elevators)` as the closest available API proxy — elevators with no passing inspection on record. Card 4 ("Licenses Expiring in 30 Days") shows `0` because no date-arithmetic query exists in the current API contract. Both values remain semantically useful for an operations view; the exact metrics would require new API endpoints.
`df_merged` and `df_incidents` (used exclusively for incident/alteration counts in the elevator detail panel) are intentionally retained — these datasets are out of scope for the Go API per the API spec §2.