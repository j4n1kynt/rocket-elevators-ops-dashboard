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
| `agents_test.go` | Isolated | Per-agent behavior + error recovery. `newTrackingMCPServer` / `fakeLLMServer` / `newFailingLLMServer` / `newCapturingLLMServer` via `httptest`; `t.Setenv` throughout. Includes the failure-mode × agent matrix and the scheduling write-gate test |
| `router_test.go` | Isolated | Drives the real `Route()` entry point: routing accuracy (`AgentName` across all 4 agents), ambiguous/multi-domain edge cases, the §4.3 error contract (`TestRouteErrorContract`), and the panic/recover guarantee (`TestRouteRecoversFromAgentPanic`) |

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

## Simulating a service being down

Every "service down" scenario is reproduced **in-process** with `net/http/httptest` fake servers and `t.Setenv` — no real PostgreSQL, MCP server, or LLM is ever contacted. This is what makes the error-recovery suite deterministic and fast (~2 s) with zero network flakiness: a "failure" is a fake server that closes its socket or returns a chosen status/payload, not a real outage we have to provoke.

The agents reach three downstream dependencies, each behind an env var the test overrides:

| Dependency | Env var (set per-test) | Fake server |
|---|---|---|
| MCP tool server | `MCP_SERVER_URL` | `newTrackingMCPServer` |
| LLM (Ollama path) | `OLLAMA_BASE_URL`, `OLLAMA_API_KEY` | `fakeLLMServer` / `newFailingLLMServer` / `newCapturingLLMServer` |
| LLM (OpenRouter path) | `OPENROUTER_API_KEY` | unset by `TestMain` so tests stay on the Ollama path |

### Failure-simulation matrix

| Failure mode | How it is simulated | Representative test |
|---|---|---|
| **MCP server unreachable** | Create `newTrackingMCPServer`, then `.Close()` it **before** the agent runs → the client gets "connection refused" | `TestAgentsMCPUnreachableRecoverGracefully`, `TestDataAgentMCPTransportFailureNoRawError` |
| **MCP tool returns an error** | Tracking server returns an error payload — `{"error":true,...}` (data → `isToolError`) or `{"success":false,"error":"…"}` (scheduling → `extractScheduleError`) | `TestAgentsToolErrorPayloadRecoverGracefully`, `TestKnowledgeAgentBothCorporaToolErrorNoRawError` |
| **MCP returns no results** | Tracking server's default payload `{"total_returned":0,"results":[]}` (any tool not in the map) | `TestKnowledgeAgentNoResultsAdvisoryOnly`, `TestDataAgentNoResultsReportsNoRecordsNotInvented` |
| **LLM endpoint down (HTTP 5xx)** | `newFailingLLMServer(t, http.StatusServiceUnavailable)` → `callChatLLM` returns a non-200 error | `TestAgentsLLMFailureRecoverGracefully`, `TestSchedulingAgentLLMDownAcrossPhases` |
| **LLM endpoint unreachable** | `fakeLLMServer(...)` then `.Close()` → "connection refused" | `TestRouteErrorContract` (the `llmDown` rows) |
| **LLM returns garbage** | `fakeLLMServer(t, "<raw error string or JSON blob>")` — the model "answers" with infra text instead of prose | `TestRouteErrorContract`, `TestBuildReplyMalformedLLMOutput`, `TestDataAgentHybridIntroDropsMalformedLLMOutput` |
| **Agent panics** | Swap a panicking stub into the package-level `agents` map (restored via `t.Cleanup`) | `TestRouteRecoversFromAgentPanic` |

### Helpers

| Helper (in `agents_test.go` unless noted) | Role |
|---|---|
| `newTrackingMCPServer(t, payloads)` | Fake MCP server; records every `tools/call` name + args; serves a canned payload by tool name, else the empty-results default. `.Close()` it to simulate "unreachable". |
| `fakeLLMServer(t, reply)` | Fake LLM returning a fixed assistant reply; `.Close()` to simulate "unreachable". |
| `newFailingLLMServer(t, status)` | Fake LLM that always responds with the given HTTP status (e.g. 503). |
| `newCapturingLLMServer(t, reply)` | Fake LLM that records the **system messages** the agent sent, so a test can assert what reached the model (e.g. an unavailability notice, no raw error). |
| `assertSafeReply(t, reply)` | Reply is non-empty **and** carries no raw error text or JSON blob. |
| `assertNoRawErrorLeak(t, sys)` | A captured system message carries no raw infrastructure fragments (`dial tcp`, `connectex:`, …). |
| `assertErrorContract(t, resp)` *(router_test.go)* | Design §4.3: reply populated + plain-language, `PendingAction` nil. |
| `assertNoConfirmedWrite(t, mcp)` | `schedule_inspection` was never called with `confirmed=true` — the write-gate invariant. |

**Why a *live* fake server is used for the write-gate.** `TestSchedulingFailuresNeverWriteToDatabase` keeps the MCP server **open** (returning a write-like success payload) on its reject-path cases, rather than closing it. A closed server would make an erroneous `confirmed=true` call fail silently and go unrecorded; a live server records the call so an accidental write is actually caught.

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
| 3 | Edge cases: ambiguous, multi-domain, unanswerable | **Ambiguous:** `TestConfidenceFallbackToAdvisory`, `TestAdvisoryByDefaultConfidence` (classification layer); `TestAmbiguousQueriesHandledGracefully` (Route() end-to-end: zero-signal, single-keyword-below-floor, competing-signals-tie — each checks AgentName + reply + zero MCP calls). **Multi-domain:** `TestMultiDomainQueriesRoutedDeterministically` (Route() end-to-end for 3 cross-intent combinations: RAG beats DataQuery, DataQuery beats RAG, Action beats RAG — each checks AgentName + reply + correct tool called + no cross-domain tool leakage + determinism); `TestMultiDomainSignalTraceRecordsAllKeywords` (Signals trace includes losing-domain keywords for auditability); `TestBuildMCPArgsRouting: rag_procedure_reporting_incident` (classification-layer tool selection). **No-results/unanswerable:** `TestDataAgentNoResultsReportsNoRecordsNotInvented` (data agent — not-found, no-prediction, and empty-list payloads each carry the deterministic "no records" block into the user-facing reply; neutral LLM intro proves the wording is grounded in the tool result, not invented), `TestKnowledgeAgentNoResultsAdvisoryOnly`, `TestKnowledgeAgentNoResultsSystemPromptClean`, `TestSchedulingAgentMissingInfoAsksNotFabricates`; out-of-scope/off-domain queries route to the general agent with a graceful advisory reply and zero MCP calls — covered by `TestAmbiguousQueriesHandledGracefully` (zero-signal case) + `TestGeneralAgentNeverCallsMCPTools`. **Safe response on every edge case:** the shared `assertSafeReply` helper (non-empty + no leaked raw error text or JSON blob) is applied across `TestAmbiguousQueriesHandledGracefully`, `TestMultiDomainQueriesRoutedDeterministically`, and `TestDataAgentNoResultsReportsNoRecordsNotInvented`, so every ambiguous / multi-domain / no-results path is checked for a clean, user-safe reply. **Scope guards:** `TestDataAgentNeverCallsForbiddenTools`, `TestKnowledgeAgentNeverCallsDataTools`. **Malformed output:** `TestBuildReplyMalformedLLMOutput` (general/knowledge/data-fallback paths via `buildReply`), `TestDataAgentHybridIntroDropsMalformedLLMOutput` (hybrid data path — a raw-error or JSON-blob summary intro is dropped and the grounded Go block is shown alone, so the path `buildReply` does not run through cannot leak); note: "not found" and "no prediction" formatter assertions live in `TestFormatRiskBlock` (unit); their delivery through the hybrid path into the user-facing reply is proven by `TestDataAgentNoResultsReportsNoRecordsNotInvented` (no-records) and `TestDataAgentRiskLookupUsesRiskTool` (found) |
| 4 | Error recovery when a service is down | Full **failure-mode × agent matrix** (3 modes × 4 agents). **MCP unreachable:** `TestDataAgentMCPTransportFailureNoRawError` (data), `TestAgentsMCPUnreachableRecoverGracefully` (knowledge + scheduling — closed server → plain-language notice injected, no raw `connectex`/`dial tcp` leak, no dangling `PendingAction`). **Tool returns error payload:** `TestKnowledgeAgentBothCorporaToolErrorNoRawError` (knowledge), `TestAgentsToolErrorPayloadRecoverGracefully` (data `{"error":true}` envelope hidden behind `DATA SERVICE ERROR`; scheduling `success:false` → `ACTION VALIDATION ERROR`). **LLM call failing (HTTP 503):** `TestAgentsLLMFailureRecoverGracefully` (all four agents — `buildReply` paths return the graceful fallback; the data hybrid path drops the failed intro and delivers the deterministic Go block alone, so the grounded answer is never lost), `TestSchedulingAgentLLMDownAcrossPhases` (scheduling's real branches under an LLM outage — Phase 1 preview keeps the signed confirmation token so the user can still confirm; Phase 2 still commits the write with `confirmed=true` and clears pending state, so a committed inspection is never silently dropped). **Write-gate (no unintended DB write on failure):** `TestSchedulingFailuresNeverWriteToDatabase` asserts the safety invariant that `schedule_inspection` is never called with `confirmed=true` (the only write shape) on any failure or invalid path — tampered signature, expired confirmation, ambiguous yes+cancel, MCP unreachable during preview, and tool validation error. Each case uses a *live* MCP server so an erroneous write attempt would be recorded and caught; the reject branches additionally make zero MCP calls. Unit-level gate functions are covered by `TestVerifyPendingActionRejectsTampering` / `RejectsUnsigned` / `TestDetectConfirmation` (chat_test.go); cancel-path no-write by `TestSchedulingAgentCancelWritesNothing`. Helpers: `newFailingLLMServer`, `assertNoRawErrorLeak`, `assertSafeReply`, `assertNoConfirmedWrite`. Client-level: `mcp_client_test.go` (HTTP 5xx, unreachable host); `TestCallLLMNon200` (LLM 429). **Error contract (design §4.3) on the public surface:** `TestRouteErrorContract` drives the real `Route()` across all failure modes with the LLM *also* misbehaving (parrots a raw error, returns a JSON blob, or is unreachable) and asserts via `assertErrorContract` that the user reply is always populated + plain-language (no raw error/JSON) and `PendingAction` is nil — proving the guarantee holds without a cooperative model. `TestRouteRecoversFromAgentPanic` covers Route's `defer/recover` last-resort guarantee (a panicking agent still yields a populated `"router"` reply, never a crash or empty response). |
| 5 | Whole suite runs with one command | `make test` |

All tests in AC 1–4 run under `make test-go` (`go test ./...` in `platform/api`) using `httptest` fake servers and `t.Setenv` — no live database, LLM, or MCP server is contacted.

---

## Pre-release trust checklist

The suite is trustworthy to release behind when:

- [ ] **`make test` exits 0** — Go (`build` + `vet` + `test`) and every isolated Python suite pass, with **no skipped Go tests**. This needs no live services.
- [ ] **All four acceptance criteria are green inside that one command** — routing (AC 1), grounded answers (AC 2), edge cases (AC 3), error recovery (AC 4). See the coverage table below.
- [ ] **The error-recovery matrix is complete** — every cell of (MCP unreachable | tool error | LLM down) × (data | knowledge | scheduling | general) is exercised (AC 4), plus the §4.3 contract and panic/recover at the `Route()` level.
- [ ] **The scheduling write-gate holds** — `TestSchedulingFailuresNeverWriteToDatabase` proves no failure path issues a `confirmed=true` write.
- [ ] **No raw error or JSON ever reaches the user** — `assertSafeReply` / `assertErrorContract` guard the reply on every degraded path.
- [ ] **DB-backed checks pass** — run `make test-integration` against a real PostgreSQL whenever schema or SQL changed.
- [ ] **Eval sanity-run** — `make eval` against the full stack for any prompt or agent-routing change (manual confidence check, not an automated gate).

---

## Gaps & Assumptions

`make test` proves the **Go agent pipeline is correct given a model** — routing, tool selection, scope guards, grounding-block assembly, error handling, and the write-gate. It deliberately does **not** prove the things below. Read this before treating a green run as full release confidence.

- **Live model quality is not tested — the LLM is faked.** Every Go test uses a canned `fakeLLMServer` reply (or a forced failure). The suite verifies that the pipeline routes correctly, injects the right context/notice, keeps the deterministic data block exact, and never leaks raw errors — but it cannot verify that the *real* model (mistral:7b) produces accurate, well-phrased, prompt-compliant prose. Assertions like "the system message carries the no-fabrication instruction" prove the agent *asked* the model to behave, not that the model *did*. **Live answer quality is checked only manually via `make eval`** (`run_eval.py` / `run_eval2.py`), which is not a regression gate.
- **"Service down" means closed / erroring / 5xx — never *slow*.** Fake servers respond instantly, so the agents' context timeouts (data 25 s, knowledge 25 s per corpus, scheduling 10 s) are never exercised. A dependency that is reachable but slow (a real risk on constrained CPU, where warm embedding queries take ~8 s) is untested.
- **MCP payload fixtures are hand-authored, not contract-verified.** The fake MCP server returns canned JSON shaped to match what the Go formatters expect. If the live MCP tool's response schema drifts, a formatter could break (or silently mis-parse) without the isolated suite noticing. The real Go↔MCP contract is only exercised by `make eval` and the Python MCP-tool tests.
- **DB-write safety is two-layered; this suite owns only the Go layer.** `TestSchedulingFailuresNeverWriteToDatabase` proves the agent never *requests* a write (`confirmed=true`) on a failure path. That the MCP `schedule_inspection` tool itself honors the confirmed flag server-side is a **separate** guarantee, covered by `test_schedule_inspection.py` (`make test-integration`, needs PostgreSQL).
- **Pending-action signing uses an ephemeral secret in tests.** With `CHAT_SIGNING_SECRET` unset, each run signs with a throwaway HMAC key (the "ephemeral signing secret" log line). Signature verification logic is tested, but **survival of a pending confirmation across a server restart** (which requires a stable configured secret) is not.
- **Provider path assumption: Ollama.** `TestMain` unsets `OPENROUTER_API_KEY`, so the agent error-recovery tests all run the Ollama branch of `callChatLLM`. The OpenRouter branch is covered only at the `chat.go` unit level, not through the agents.
- **Out of scope for this suite entirely:** the Flask server (`server.py`), HTMX templates and front-end behavior, full HTTP end-to-end, and live RAG retrieval quality (ChromaDB content — see `make validate-rag`).

---

## Decisions & Notes

- **`make test` is self-contained** — no PostgreSQL, no LLM, no MCP server required. All Go tests use `httptest` fakes; all Python tests in `test-python` use `unittest.mock` or pure functions.
- **`test_risk_assessment.py` and `test_schedule_inspection.py`** were moved out of `make test` into `make test-integration` because they open real `asyncpg` connections and cannot run without a live PostgreSQL.
- **`PYTHONPATH`, `ALLOW_EMPTY_CHROMADB`, `MCP_SKIP_CONFIRMATION`** are set inside the Makefile so callers don't need to export them manually.
- **`DATABASE_URL`** is intentionally not hardcoded — local credentials differ from CI. The Makefile fails fast with a clear error if it is missing.
- **Eval harnesses** are excluded from `make test` — they require a live Go API + MCP server and are manual verification tools, not regression tests.
- **CI parity:** `test-integration` mirrors the DB-backed steps in `.github/workflows/ci.yml`. CI still runs both suites sequentially.
- **CLAUDE.md:** All four primary targets are documented in the Commands section.
- **Hybrid-intro safety guard (fix):** the data agent's hybrid path builds `reply = intro + "\n\n" + block`, where the intro comes from `summarizeForUser` and is the only unguarded LLM output on that path (the block is Go-built). It previously had no raw-error/JSON guard, so a misbehaving summary model could leak an error string or JSON blob into the reply — the one path `buildReply`'s `looksLikeRawError`/`looksLikeJSON` guards did **not** cover. `summarizeForUser` now drops a malformed intro and shows the deterministic block alone (the answer is never lost). Regression test: `TestDataAgentHybridIntroDropsMalformedLLMOutput`.
- **`assertSafeReply` helper:** shared edge-case assertion (non-empty + no leaked raw errors/JSON) in `agents_test.go`, applied across the ambiguous, multi-domain, and no-results tests so every degraded path is checked for a clean user-facing reply.
- **`looksLikeRawError` prefix→substring fix:** the guard that discards an LLM reply that echoed an infrastructure error originally matched only with `strings.HasPrefix`. `TestRouteErrorContract` (scheduling row) caught that the canonical Go HTTP error — `Post "URL": dial tcp ...: connection refused` — leads with the method verb and therefore slipped past the prefix check, leaking the raw error into the user reply. Fixed by matching the unambiguous transport fragments (`dial tcp`, `connectex:`, `connection refused`, `mcp server unreachable`, `llm returned status`) as substrings while keeping the ambiguous `"mcp "` token prefix-only (so legitimate prose like "the MCP server…" is not discarded). Regression covered by `TestRouteErrorContract`.
