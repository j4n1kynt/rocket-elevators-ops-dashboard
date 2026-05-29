---
name: validate-api
description: Validate API endpoints against docs/api_spec.md. Takes endpoint path (e.g., /api/elevators) and tests it against the specification using strict contract validation. Reports ✅ PASS or ❌ FAIL with detailed violations and fixes.
user-invocable: true
argument-hint: "[endpoint-path]"
disable-model-invocation: true
context: fork
agent: api-validator
---

# /validate-api — API Contract Validator

Validate a live API endpoint at `http://localhost:8080` against `docs/api_spec.md`.

## Usage

```
/validate-api /api/elevators
/validate-api /api/elevators/{id}
/validate-api /api/elevators/{id}/inspections
```

Pass the endpoint path as the argument. The validator tests it against `docs/api_spec.md` and reports ✅ PASS or ❌ FAIL with contract violations and suggested fixes.

See `.claude/agents/api-validator.md` for validation behavior and `.claude/skills/validate-api/VALIDATION_GUIDE.md` for extended documentation.
