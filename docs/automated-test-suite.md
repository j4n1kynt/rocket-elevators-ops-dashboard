# Automated Test Suite

## Overview

The self-contained suite (no live services required) runs with:

```bash
make test
```

This runs the Go package tests and all isolated Python suites. DB-backed integration tests are in a separate target.

---

## Prerequisites

| Requirement | Notes |
|---|---|
| `make` | Shipped with Git for Windows (Git Bash); also available via Chocolatey |
| Go 1.25+ | For `test-go` |
| Python 3.10+ | For `test-python` and `test-integration` |
| PostgreSQL 16 running | Required by `test-integration` only |
| `DATABASE_URL` env var | Required by `test-integration` and `eval` only |

```bash
# Only needed for test-integration / eval
export DATABASE_URL=postgresql://rocketuser:rocketpass@localhost:5432/rocket_elevators
```

---

## Targets

| Command | Live services needed | What it does |
|---|---|---|
| `make test` | None | Self-contained suite — `test-go` + `test-python` |
| `make test-go` | None | `go build ./...`, `go vet ./...`, `go test ./...` in `platform/api` |
| `make test-python` | None | Isolated Python suites (mocked DB/LLM/MCP) |
| `make test-integration` | PostgreSQL | DB-backed integration tests (requires `DATABASE_URL`) |
| `make eval` | PostgreSQL + Go API + MCP | Chatbot eval harnesses |
| `make validate-rag` | None | Validates ChromaDB RAG preprocessing output |
| `make help` | — | Lists all targets with descriptions |

---

## Test Inventory

### Go (`platform/api/`) — all isolated

All Go tests use `httptest.NewServer` fake servers and `t.Setenv` for all external URLs and API keys. No live service is contacted.

| File | Verdict | Mechanism |
|---|---|---|
| `main_test.go` | Isolated | Package setup — `os.Unsetenv("OPENROUTER_API_KEY")` only |
| `env_test.go` | Isolated | `t.TempDir()` temp files; no network |
| `mcp_client_test.go` | Isolated | `httptest` fake MCP server; `t.Setenv("MCP_SERVER_URL", ...)` |
| `intent_test.go` | Isolated | Pure function calls; no network (27+ cases) |
| `chat_test.go` | Isolated | `httptest` fake Ollama + OpenRouter; `t.Setenv` for all URLs/keys |
| `agents_test.go` | Isolated | `newTrackingMCPServer` + `fakeLLMServer` via `httptest`; `t.Setenv` throughout |
| `router_test.go` | Isolated | Calls real `Route()` entry point; same fake server helpers; asserts `AgentName` for 18 representative queries across all 4 agents |

### Python — Isolated (`make test-python`)

| File | Verdict | Mechanism |
|---|---|---|
| `intelligence/tests/test_rag_preprocessing.py` | Isolated | Pure Python text/chunking functions; `@pytest.mark.slow`/`integration` opt-in only |
| `intelligence/tests/test_index_incident_narratives.py` | Isolated | Pure Python dicts; no DB or model |
| `platform/mcp/tools/test_validation.py` | Isolated | Mock fires before DB call; `ValidationError` asserted before any I/O |
| `platform/mcp/tools/test_rag_search.py` | Isolated | `unittest.mock.patch` on `rag_query`; no live ChromaDB |
| `platform/mcp/tools/test_source_attribution.py` | Isolated | `_FakeConn` class patches `get_connection`; no real DB connection |

### Python — DB-backed (`make test-integration`)

Both files open a real `asyncpg` connection. They seed synthetic elevator IDs (9\_000\_000+) and clean up after themselves, but they require PostgreSQL to be running and `DATABASE_URL` to be set.

| File | Verdict | What it needs |
|---|---|---|
| `platform/mcp/tools/test_risk_assessment.py` | Needs-DB | Real asyncpg connection; seeds + cleans synthetic prediction rows |
| `platform/mcp/tools/test_schedule_inspection.py` | Needs-DB | Real asyncpg connection; applies DDL for audit table/sequence |

### Chatbot Eval Harnesses (`tests/chatbot/`) — manual only

| File | Covers |
|---|---|
| `run_eval.py` | 5 mandatory data queries + risk + RAG + boundary + scheduling scenarios |
| `run_eval2.py` | Edge cases: scheduling write-gate, no-prediction path, RAG-citation determinism |

Run with `make eval` when the full stack is up (PostgreSQL + Go API + MCP server).

---

## Environment Variables

| Variable | Required by | Set by Makefile |
|---|---|---|
| `DATABASE_URL` | `test-integration`, `eval` | No — must be exported by caller |
| `PYTHONPATH` | All Python targets | Yes — set to `.` |
| `ALLOW_EMPTY_CHROMADB` | All Python targets | Yes — set to `1` |
| `MCP_SKIP_CONFIRMATION` | All Python targets | Yes — set to `1` |

---

## S3-10 Acceptance Criteria Coverage

The Gherkin scenarios for S3-10 live in `tests/chatbot_suite.feature`. All criteria are satisfied by existing tests — no new code was needed.

| AC | Requirement | Satisfied by |
|---|---|---|
| 1 | Routing accuracy for all four agents | `TestRouteIntent` (all 4 targets), `TestClassifyIntent` (all 4 intents), `TestBuildMCPArgsRouting`; **`TestRouteSelectsCorrectAgent`** (calls real `Route()`, checks `AgentName` for all 4 agents + incident-RAG split); **`TestRoutePendingActionPreemptsClassification`** |
| 2 | Response quality on grounded answers | `TestDataAgentRiskLookupUsesRiskTool` (reply contains source/level/score), `TestDataAgentFleetStatsBlockInReply` (fleet-stats block verbatim in reply), `TestFormatToolResult` (6 tool formatters), `TestKnowledgeAgentProcedureUsesMaintenanceDocs` (reply cites "Maintenance Document 10078"), `TestKnowledgeAgentIncidentQueryUsesNarratives` (reply references incident ID 1163652) |
| 3 | Edge cases: ambiguous, multi-domain, unanswerable | `TestConfidenceFallbackToAdvisory`, `TestAdvisoryByDefaultConfidence`, `TestKnowledgeAgentNoResultsAdvisoryOnly`, `TestKnowledgeAgentNoResultsSystemPromptClean` (no phantom [DATA SOURCE:] injected when corpora return empty; no-fabrication instruction present), `TestSchedulingAgentMissingInfoAsksNotFabricates` (missing date → zero MCP calls, nil PendingAction), `TestDataAgentNeverCallsForbiddenTools`, `TestKnowledgeAgentNeverCallsDataTools`, `TestBuildReplyMalformedLLMOutput`, `TestGeneralAgentNoFallbackOnSubstantiveMessage`; note: "not found" and "no prediction" formatter assertions live in `TestFormatRiskBlock` (unit) — hybrid path proven by `TestDataAgentRiskLookupUsesRiskTool` |
| 4 | Error recovery when a service is down | `TestDataAgentMCPTransportFailureNoRawError` (MCP closed → graceful reply), `TestKnowledgeAgentBothCorporaToolErrorNoRawError` (tool error payload → graceful reply), `mcp_client_test.go` (HTTP errors, unreachable host) |
| 5 | Whole suite runs with one command | `make test` |

All tests in AC 1–4 run under `make test-go` (`go test ./...` in `platform/api`) using `httptest` fake servers and `t.Setenv` — no live database, LLM, or MCP server is contacted.

---

## Decisions & Notes

- **`make test` is self-contained** — no PostgreSQL, no LLM, no MCP server required. All Go tests use `httptest` fakes; all Python tests in `test-python` use `unittest.mock` or pure functions.
- **`test_risk_assessment.py` and `test_schedule_inspection.py`** were moved out of `make test` into `make test-integration` because they open real `asyncpg` connections and cannot run without a live PostgreSQL.
- **`PYTHONPATH`, `ALLOW_EMPTY_CHROMADB`, `MCP_SKIP_CONFIRMATION`** are set inside the Makefile so callers don't need to export them manually.
- **`DATABASE_URL`** is intentionally not hardcoded — local credentials differ from CI. The Makefile fails fast with a clear error if it is missing.
- **Eval harnesses** are excluded from `make test` — they require a live Go API + MCP server and are manual verification tools, not regression tests.
- **CI parity:** `test-integration` mirrors the DB-backed steps in `.github/workflows/ci.yml`. CI still runs both suites sequentially.
- **CLAUDE.md:** All four primary targets are documented in the Commands section.
