---
name: new-endpoint
description: Create a new endpoint in the Go API. Follows a strict 5-step workflow — spec update, handler implementation, route registration, data layer support, and validation. Use when adding any new route to platform/api.
user-invocable: true
argument-hint: "<endpoint-path> <description>"
---

# /new-endpoint — Go API Endpoint Scaffold

Add a new endpoint to the Go REST API at `platform/api` following the project's spec-driven workflow.

## Usage

```
/new-endpoint /api/fleet/stats  "Return aggregate statistics across the full elevator fleet"
/new-endpoint /api/fleet/alerts "Return elevators that are high-risk and failed their most recent inspection"
```

Pass the endpoint path as the first argument and a plain-English description of the business requirement as the second.

---

## Workflow

Execute all five steps in order. Do NOT skip or reorder them.

---

### Step 1 — Update `docs/api_spec.md`

The spec is the contract. Code must follow the spec, never the reverse.

Add a new section for the endpoint that includes:

- **Endpoint line:** `GET <path>` (or the correct HTTP method)
- **Description:** what the endpoint returns and why
- **Request parameters:** list query params with types, defaults, and validation rules (or "none" if no params)
- **Response JSON example:** a realistic, fully populated JSON example using real field names and types
- **Error responses table:** at minimum cover 400 (invalid input) and 404 (not found) where applicable

Follow the style and table format already used in `docs/api_spec.md`. Do not invent field names — derive them from the data sources (`platform/elevator_fleet.csv`, `data/predictions.csv`, `data/inspection.csv`) and existing models in `platform/api/models.go`.

---

### Step 2 — Implement the Go handler in `platform/api/handlers.go`

Add a new handler function following the conventions already in the file:

- Function signature: `func FunctionName(w http.ResponseWriter, r *http.Request)`
- Use `writeJSON(w, status, payload)` for all responses
- Use `ErrorResponse` for all error payloads
- Source data from in-memory slices/maps (`elevators`, `elevatorIdx`, `inspectionIdx`, `riskIdx`) — no file reads at request time
- Validate all inputs; return 400 for invalid parameters before any data processing
- Return 404 with `ElevatorID` populated when a specific elevator is not found

Implement the exact logic the spec describes. Do not invent fields or behaviors not stated in the spec.

---

### Step 3 — Register the route in `platform/api/main.go`

Add the route to the `http.ServeMux` in `main.go` following the existing pattern:

```go
mux.HandleFunc("GET /api/fleet/stats", GetFleetStats)
```

Place the new route alongside related routes (fleet-level routes near each other, elevator-specific routes near each other).

---

### Step 4 — Verify data layer support

Check whether the handler needs data that is not already loaded at startup:

- `elevators []Elevator` — fleet data from `platform/elevator_fleet.csv`
- `elevatorIdx map[string]*Elevator` — keyed by elevator_id
- `inspectionIdx map[int][]Inspection` — keyed by device number
- `riskIdx map[string]*RiskResponse` — keyed by elevator_id

If a join across multiple sources is needed (e.g., risk score + inspection outcome + elevator ID), perform it inside the handler using the existing in-memory maps. Do NOT add new CSV reads. Do NOT add new startup loading unless the data genuinely cannot be derived from existing sources — if you must add loading, add it to `data.go` and call it from `main.go` after existing loaders.

---

### Step 5 — Validate with `/validate-api`

Run `/validate-api <endpoint-path>` to confirm the live response matches the spec you wrote in Step 1.

Fix any mismatches before marking the task complete. Common issues:

- Missing fields in the response struct (add to `models.go`)
- Wrong HTTP status code
- Field serialized as wrong JSON type (check struct tags)
- Null vs omitted field behavior (use `*T` for nullable, `omitempty` if field may be absent)

Do not report the endpoint as done until `/validate-api` returns ✅ PASS.

---

## Constraints

- **Spec first, always.** Never write handler code before the spec section exists.
- **No hardcoded data.** All response values must come from the in-memory data loaded at startup.
- **No new fields invented.** Every response field must be traceable to a column in a source CSV or a computation explicitly described in the spec.
- **Follow existing struct conventions.** New response types go in `models.go`. New helper functions go in `data.go`. Handlers go in `handlers.go`.
- **No external dependencies.** Use only the Go standard library and packages already imported.
