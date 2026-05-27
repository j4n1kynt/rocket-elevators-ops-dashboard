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