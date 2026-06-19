# MCP Tool Input Models

**File:** `platform/mcp/tools/models.py`  
**Replaces:** `platform/mcp/tools/_validators.py` (deleted)

## Overview

Each MCP tool that accepts user-facing parameters has a dedicated Pydantic `BaseModel` subclass. All models use `ConfigDict(strict=True)` — no implicit type coercion. `Field()` carries only the default value and description. All validation logic lives in explicit `@field_validator` methods so that error messages are precise and easy to find.

The function signatures are unchanged — FastMCP still sees flat parameters and generates the same tool schema for the LLM. The model is constructed at the top of each function:

```python
async def get_elevator_incidents(elevator_id: int, limit: int = 20) -> dict:
    inp = GetElevatorIncidentsInput(elevator_id=elevator_id, limit=limit)
    # use inp.elevator_id, inp.limit — already validated
```

## Shared validator helpers

Three module-level functions are reused across models:

| Helper | Purpose |
|---|---|
| `_check_elevator_id(v)` | Asserts `v > 0` and `v ≤ 9,999,999` with explicit messages |
| `_check_limit(v, *, max_val)` | Asserts `1 ≤ v ≤ max_val` with explicit messages |
| `_strip_nonempty(v, *, field, max_len)` | Strips whitespace, rejects blank string, enforces max length |

## Models

### `GetElevatorIncidentsInput`
Used by: `get_elevator_incidents`

| Field | Type | Default | Validator |
|---|---|---|---|
| `elevator_id` | `int` | required | `validate_elevator_id` → `_check_elevator_id` |
| `limit` | `int` | `20` | `validate_limit` → `_check_limit(max_val=100)` |

---

### `GetTssaShutdownElevatorsInput`
Used by: `get_tssa_shutdown_elevators`

| Field | Type | Default | Validator |
|---|---|---|---|
| `limit` | `int` | `50` | `validate_limit` → `_check_limit(max_val=200)` |

---

### `GetInspectionHistoryInput`
Used by: `get_inspection_history`

| Field | Type | Default | Validator |
|---|---|---|---|
| `elevator_id` | `int` | required | `validate_elevator_id` → `_check_elevator_id` |
| `limit` | `int` | `20` | `validate_limit` → `_check_limit(max_val=100)` |

---

### `GetElevatorsNeedingFollowupInput`
Used by: `get_elevators_needing_followup`

| Field | Type | Default | Validator |
|---|---|---|---|
| `limit` | `int` | `50` | `validate_limit` → `_check_limit(max_val=200)` |

---

### `GetElevatorRiskInput`
Used by: `get_elevator_risk`

| Field | Type | Default | Validator |
|---|---|---|---|
| `elevator_id` | `int` | required | `validate_elevator_id` → `_check_elevator_id` |

---

### `ScheduleInspectionInput`
Used by: `schedule_inspection`

| Field | Type | Default | Validator |
|---|---|---|---|
| `elevator_id` | `int` | required | `validate_elevator_id` → `_check_elevator_id` |
| `inspection_date` | `date` | required | `validate_inspection_date` (mode=before) |
| `reason` | `str` | required | `validate_reason` (mode=before) → `_strip_nonempty` |
| `confirmed` | `bool` | `False` | strict mode — `"yes"` and `1` are rejected at the type layer |

**`inspection_date`** — accepts a `YYYY-MM-DD` string from the LLM. The `mode="before"` validator parses it into a `datetime.date` and rejects past dates. The tool function receives `inp.inspection_date` as a native `date` object ready to pass to asyncpg.

**`confirmed`** — `ConfigDict(strict=True)` handles this entirely. Integers and strings never reach the function body.

---

### `SearchMaintenanceDocsInput`
Used by: `search_maintenance_docs`

| Field | Type | Default | Validator |
|---|---|---|---|
| `query` | `str` | required | `validate_query` (mode=before) → `_strip_nonempty(max_len=500)` |
| `n_results` | `int` | `5` | `validate_n_results` → `_check_limit(max_val=20)` |

---

### `SearchIncidentNarrativesInput`
Used by: `search_incident_narratives`

| Field | Type | Default | Validator |
|---|---|---|---|
| `query` | `str` | required | `validate_query` (mode=before) → `_strip_nonempty(max_len=200)` |
| `limit` | `int` | `5` | `validate_limit` → `_check_limit(max_val=20)` |

---

## Tools with no input model

`get_incident_count_last_year` and `get_fleet_stats` take no parameters. The date range in `get_incident_count_last_year` is computed server-side in SQL.

## Migration notes

`_validators.py` has been deleted. Its logic is now split between the shared helpers and per-model validators:

| Old helper | Replaced by |
|---|---|
| `validate_elevator_id(v)` | `_check_elevator_id(v)` called from each model's `validate_elevator_id` |
| `validate_limit(v, max_val)` | `_check_limit(v, max_val=N)` called from each model's `validate_limit` |
| `validate_query_string(v, max_len)` | `_strip_nonempty(v, field=..., max_len=N)` called from `validate_query` |
| `validate_inspection_date(v)` | `validate_inspection_date` validator on `ScheduleInspectionInput` |
| `validate_reason(v)` | `validate_reason` validator on `ScheduleInspectionInput` → `_strip_nonempty` |

**What changed from the initial model implementation:** the original models used `Field(gt=0, le=9_999_999)` and `Field(min_length=1, max_length=N)` constraints. Those produce generic Pydantic error messages. All constraints have been moved into named `@field_validator` methods that raise `ValueError` with precise, human-readable messages.
