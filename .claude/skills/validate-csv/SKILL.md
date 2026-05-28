---
name: validate-csv
description: Validate Rocket Elevators CSV datasets against expected schemas. Takes dataset name (elevator_fleet, inspection, or all) and checks schema, data types, formats, duplicates, and referential integrity. Reports ✅ VALID or ❌ ISSUES with severity and fixes.
user-invocable: true
argument-hint: "[elevator_fleet|inspection|all]"
disable-model-invocation: true
context: fork
agent: csv-validator
---

# /validate-csv — CSV Data Validator

Validate source datasets before they reach the Go API or the dashboard.

## Usage

```
/validate-csv elevator_fleet   — validate platform/elevator_fleet.csv
/validate-csv inspection       — validate data/inspection.csv
/validate-csv all              — validate both + cross-dataset referential integrity
```

Pass the dataset name as the argument. The validator checks schema, types, formats, null rates, duplicates, and referential integrity against the contracts defined in `data.go` and `prepare_data.py`.

See `.claude/agents/csv-validator.md` for full validation rules.
