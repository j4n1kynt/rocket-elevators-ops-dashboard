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