---
name: new-endpoint
description: Create a new endpoint in the Go API. Follows a strict 5-step workflow — spec update, handler implementation, route registration, data layer support, and validation. Use when adding any new route to platform/api.
user-invocable: true
argument-hint: "<endpoint-path> <description>"
---

# /new-endpoint — Go API Endpoint Scaffold

Add a new endpoint to the Go REST API at `platform/api` following the project's spec-driven workflow.

**Required arguments:**
- `<endpoint-path>` — The REST path (e.g., `/api/fleet/stats`)
- `<description>` — Plain-English business requirement for what the endpoint returns

## Usage

```
/new-endpoint /api/fleet/stats  "Return aggregate statistics across the full elevator fleet"
/new-endpoint /api/fleet/alerts "Return elevators that are high-risk and failed their most recent inspection"
```

---

## Workflow

Execute all five steps in order. Do NOT skip or reorder them.

---

### Step 1 — Update `docs/api_spec.md`

The spec is the contract. Code must follow the spec, never the reverse.

Add a new section for endpoint `<endpoint-path>` with description: `<description>`

Include:

- **Endpoint line:** `GET <endpoint-path>` (or the correct HTTP method)
- **Description:** `<description>` — what the endpoint returns and why
- **Request parameters:** list query params with types, defaults, and validation rules (or "none" if no params)
- **Response JSON example:** a realistic, fully populated JSON example using real field names and types from `models.go`
- **Error responses table:** at minimum cover 400 (invalid input) and 404 (not found) where applicable

Follow the style and table format already used in `docs/api_spec.md`. Do not invent field names — derive them from the data sources (`platform/elevator_fleet.csv`, `data/predictions.csv`, `data/inspection.csv`) and existing models in `platform/api/models.go`.

---

### Step 2 — Implement the Go handler in `platform/api/handlers.go`

Add a new handler function following the conventions already in the file. The function should handle the business logic described in your spec section.

**Function signature and naming:**
```go
func HandlerNameFromEndpoint(w http.ResponseWriter, r *http.Request) {
    // implementation
}
```

Example: For `/api/fleet/stats`, name it `GetFleetStats`.

**Required implementation patterns:**

**1. Parameter extraction and validation (FIRST — before any data processing):**

Query parameters:
```go
q := r.URL.Query()
page := 1
if v := q.Get("page"); v != "" {
    if n, err := strconv.Atoi(v); err == nil && n > 0 {
        page = n
    } else {
        writeJSON(w, 400, ErrorResponse{Error: "page must be a positive integer"})
        return
    }
}
```

Path parameters (if your endpoint has `{id}`):
```go
id := r.PathValue("id")
if !isNumeric(id) {
    writeJSON(w, 400, ErrorResponse{Error: "Invalid elevator ID format. ID must be numeric."})
    return
}
```

Enum validation (for status, type, etc.):
```go
statusFilter := q.Get("status")
if statusFilter != "" && statusFilter != "ACTIVE" && statusFilter != "BY REQUEST" {
    writeJSON(w, 400, ErrorResponse{Error: "Invalid status value. Accepted: ACTIVE, BY REQUEST"})
    return
}
```

Range validation (for limits, counts, etc.):
```go
if limit < 1 {
    writeJSON(w, 400, ErrorResponse{Error: "limit must be at least 1"})
    return
}
if limit > 200 {
    writeJSON(w, 400, ErrorResponse{Error: "limit must not exceed 200"})
    return
}
```

**2. Existence checks (SECOND — before processing data):**
```go
// For endpoints that fetch a specific record, check existence first
if _, ok := elevatorIdx[id]; !ok {
    writeJSON(w, 404, ErrorResponse{Error: "Elevator not found.", ElevatorID: id})
    return
}
```

**3. Data processing and response building:**
- Query the database using the global `db *pgxpool.Pool` defined in `db.go`
- Use parameterized queries — never interpolate user values into SQL strings
- For single-row results use `db.QueryRow(...).Scan(...)`:
  ```go
  var count int
  err := db.QueryRow(r.Context(), `SELECT COUNT(*) FROM elevators WHERE status = $1`, status).Scan(&count)
  if err != nil {
      writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
      return
  }
  ```
- For multi-row results use `db.Query(...)` with a rows loop:
  ```go
  rows, err := db.Query(r.Context(), `SELECT elevator_id, location FROM elevators ORDER BY elevator_id`)
  if err != nil {
      writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
      return
  }
  defer rows.Close()
  for rows.Next() {
      var id, loc string
      if err := rows.Scan(&id, &loc); err != nil {
          writeJSON(w, 500, ErrorResponse{Error: "scan failed"})
          return
      }
      // append to results
  }
  if err := rows.Err(); err != nil {
      writeJSON(w, 500, ErrorResponse{Error: "row iteration failed"})
      return
  }
  ```

**4. Response serialization (FINAL):**
```go
// Always use writeJSON for all responses
writeJSON(w, 200, ResponsePayload{
    Total:   len(results),
    Results: results,
})
```

Implement the exact logic your spec describes in Step 1. Do not invent fields or behaviors not stated in the spec.

---

### Step 3 — Register the route in `platform/api/main.go`

Add the route to the `http.ServeMux` in `main.go` after line 37 (after existing routes, before `http.ListenAndServe`):

```go
mux.HandleFunc("GET <endpoint-path>", HandlerNameFromEndpoint)
```

**Route registration patterns:**

Fleet-level endpoint (no path parameters):
```go
mux.HandleFunc("GET /api/fleet/stats", GetFleetStats)
```

Elevator-specific endpoint (with path parameter):
```go
mux.HandleFunc("GET /api/elevators/{id}", GetElevatorByID)
```

Nested resource endpoint:
```go
mux.HandleFunc("GET /api/elevators/{id}/inspections", GetElevatorInspections)
```

**Placement rule:** Add your route between line 34 and line 37 (alongside related endpoints). Group routes by resource level:
- Lines 34–36: `/api/elevators` (fleet endpoints)
- Lines 37–38: `/api/elevators/{id}` (elevator endpoints, nested)

---

### Step 4 — Verify data access

Check whether your handler needs data that isn't already available through standard SQL queries against the existing schema.

**Available tables (PostgreSQL — see `db.go` for connection pool setup):**

| Table | Primary key | Key columns |
|---|---|---|
| `elevators` | `elevator_id` | `location`, `elevator_type`, `status`, `license_expiration_date` |
| `inspections` | `(elevator_id, latest_inspection_date)` | `inspection_type`, `outcome` |
| `incidents` | `incident_id` | `elevator_id`, `date_of_occurrence`, `category`, `injury_severity` |
| `alterations` | `alteration_id` | `elevator_id`, `alteration_type`, `status` |
| `predictions` | `elevator_id` | `risk_score`, `risk_level`, `predicted_failure_date`, `confidence`, `model_version`, `risk_explanation` |

**Decision:**

1. **Can your handler use standard SQL JOINs against these tables?**
   - YES → Write the queries in your handler. Skip to Step 5.

2. **Do you need new response types?**
   - YES → Add new struct types to `models.go` following existing PascalCase / snake_case JSON tag conventions.

**Constraints:**
- Do NOT open files at request time — all data comes from the database
- Do NOT define new global variables for per-request state
- Use `r.Context()` as the context argument for all database calls (`db.Query`, `db.QueryRow`)

---

### Step 5 — Validate with `/validate-api`

Run `/validate-api <endpoint-path>` to confirm the live response matches the spec you wrote in Step 1.

**Before running validation, start the API server:**
```bash
go run ./platform/api
```

Server should log: `server running on :8080`

**Run the validator:**
```
/validate-api <endpoint-path>
```

Example:
```
/validate-api /api/fleet/stats
```

The validator tests the endpoint at `http://localhost:8080` against `docs/api_spec.md` and reports:
- ✅ PASS — endpoint matches spec exactly
- ❌ FAIL — endpoint violates the spec (details provided)

---

## Troubleshooting: `/validate-api` Failures

If `/validate-api` reports failures, use this table to find and fix each issue:

| Failure Message | Root Cause | Fix |
|---|---|---|
| **"Missing fields in the response struct"** | Response struct is missing a field required by the spec | Add the field to the response struct in `models.go` with correct JSON tag.<br><br>Example:<br>`FieldName string \`json:"field_name"\``<br>`FieldName *string \`json:"field_name,omitempty"\`` |
| **"Wrong HTTP status code"** | Handler returns incorrect status (e.g., 200 instead of 400) | Review Step 2 logic. Ensure validation errors return 400, not-found returns 404, success returns 200. Check that `writeJSON(w, 400, ...)` is called before any processing. |
| **"Field serialized as wrong JSON type"** | Field type in struct doesn't match spec (e.g., string vs number) | Check struct field type. Compare against spec example. Common issues:<br>- Spec expects number, struct has string → use `float64` or `int`<br>- Spec expects object, struct has string → use a nested struct type |
| **"Null vs omitted field behavior mismatch"** | Nullable field is null when it should be omitted, or vice versa | Use `*T` for nullable fields (serialize as `null`):<br>`LatestInspectionDate *string \`json:"latest_inspection_date"\``<br><br>Use `omitempty` for optional-in-response fields:<br>`ElevatorType *string \`json:"elevator_type,omitempty"\`` |
| **"Response payload does not match spec example"** | Field values are wrong (hardcoded, computed incorrectly, or missing) | Verify Step 2: all field values must come from data sources, not hardcoded. Re-check filters, joins, and transformations. Compare actual response JSON against your spec example field-by-field. |
| **"Pagination not working"** | Page/limit params not parsed or applied correctly | Check Step 2 parameter parsing and offset calculation. Example pattern (see `GetElevators` lines 111-121):<br>`offset := (page - 1) * limit`<br>`results = results[offset:end]` |
| **"Filters not applied"** | Query param filters are ignored | Check Step 2: filter values are extracted but not used in data loop. Ensure `if statusFilter != "" { ... }` logic filters the slice. |
| **"Connection refused at localhost:8080"** | API server is not running | Run `go run ./platform/api` from the project root. Wait for log: `server running on :8080`. Keep it running while you test. |
| **"Endpoint path not found (404)"** | Route not registered in `main.go` or handler name wrong | Review Step 3: confirm route registered in `main.go` between lines 34-37. Confirm handler function name matches the second argument in `HandleFunc(...)`. Check for typos. |

**After fixing each issue:**
1. Stop the API server (Ctrl+C)
2. Run `go run ./platform/api` to restart
3. Run `/validate-api <endpoint-path>` again
4. Repeat until ✅ PASS

Do not report the endpoint as done until `/validate-api` returns ✅ PASS.

---

## Go Conventions for This API

**Struct field naming (models.go):**
- Name Go struct fields in **PascalCase**: `ElevatorID`, `RiskScore`, `LicenseNumber`
- Add **JSON tags in snake_case**: `` `json:"elevator_id"` ``, `` `json:"risk_score"` ``
- Match field names to CSV column names or spec field names
- Example from `models.go` line 3-13:
  ```go
  type Elevator struct {
      ElevatorID              string  `json:"elevator_id"`
      Location                string  `json:"location"`
      LicenseExpirationDate   string  `json:"license_expiration_date"`
      LatestInspectionDate    *string `json:"latest_inspection_date"`
  }
  ```

**Nullable fields (may be null in responses):**
- Use **pointer types**: `*string`, `*int`, `*float64`
- JSON tag without `omitempty`: `` `json:"field_name"` ``
- In handlers, check with `if field != nil` before dereferencing
- Example: `LatestInspectionDate` in spec can be null → use `*string` in struct
- This field will serialize as `null` when nil, or `"2025-05-29"` when set

**Optional-in-response fields (may be omitted entirely):**
- Use **pointer types** with **`omitempty` tag**: `` `json:"field_name,omitempty"` ``
- Field omitted from JSON if nil or zero-value
- Example from `models.go` line 49-50:
  ```go
  ElevatorID string `json:"elevator_id,omitempty"`
  Endpoint   string `json:"endpoint,omitempty"`  // in ErrorResponse
  ```

**When to use each:**
- Nullable (present, but null): `LatestInspectionDate *string \`json:"latest_inspection_date"\``
- Optional (omitted if not set): `ElevatorType *string \`json:"elevator_type,omitempty"\``
- Required (always present): `ElevatorID string \`json:"elevator_id"\``

---

## Constraints

- **Spec first, always.** Never write handler code before the spec section exists in `docs/api_spec.md`.
- **No hardcoded data.** All response values must come from the in-memory data loaded at startup (`elevators`, `inspectionIdx`, `riskIdx`).
- **No new fields invented.** Every response field must be traceable to a column in a source CSV or a computation explicitly described in the spec.
- **Follow existing struct conventions.** New response types go in `models.go`. New helper functions go in `data.go`. Handlers go in `handlers.go`.
- **No external dependencies.** Use only the Go standard library and packages already imported (`encoding/json`, `net/http`, `strconv`, `strings`, etc.).
- **Routes must pass `/validate-api` before merging.** The endpoint is not complete until the validator returns ✅ PASS.
