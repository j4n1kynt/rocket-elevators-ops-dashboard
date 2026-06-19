# Inspection Scheduling via Chat

This document describes how the Fleet Assistant chatbot schedules inspections: the
allowed inspection types, the pending-confirmation state machine, how parameters
are extracted and validated, and the two-phase confirmation flow that gates every
database write.

> This is the implementation reference for the feature defined in
> `docs/dashboard_spec.md` §7. Where the two disagree, the spec is authoritative —
> update the spec first, then this document.

---

## 1. Architecture at a glance

A scheduling request crosses four processes:

```
Browser (HTMX)
  │  POST /chat  (message, history, pending_action)
  ▼
Flask  platform/server.py
  │  forwards JSON to the Go API
  ▼
Go API  platform/api/chat.go         ← intent classification, confirmation detection
  │  MCP JSON-RPC over HTTP (platform/api/mcp_client.go)
  ▼
MCP server  platform/mcp/server.py    ← FastMCP, schedule_inspection tool
  │  asyncpg (parameterized SQL)
  ▼
PostgreSQL  inspections + scheduling_audit_log
```

The browser holds no server-side session. Conversation `history` and the
`pending_action` state are round-tripped through hidden form fields and updated
via HTMX out-of-band (OOB) swaps. See §3.

---

## 2. Allowed inspection types

The TSSA source data uses 25+ granular `ED-*` inspection codes. These are not
user-facing. The chatbot exposes **five** simplified values:

| Value        | Maps to (TSSA source intent)                    |
|--------------|-------------------------------------------------|
| `Periodic`   | ED-Periodic Inspection (the default)            |
| `Followup`   | ED-Followup Inspection                          |
| `Initial`    | ED-Initial Inspection                           |
| `Incident`   | ED-Perform L1 Incident Insp                     |
| `Alteration` | ED-Minor A / Major Alteration Inspection        |

**Source of truth:** the allowlist is defined once, in
`platform/mcp/tools/models.py`:

```python
_ALLOWED_INSPECTION_TYPES = {"Periodic", "Followup", "Initial", "Incident", "Alteration"}
```

Rules:

- The value is **case-sensitive** and must match the allowlist exactly. `"periodic"`
  is rejected; `"Periodic"` is accepted.
- An empty string (`""`) is permitted and means "unspecified". The write tool
  resolves an empty type to `Periodic` before insert (`write_tools.py`,
  `resolved_type`).
- Any non-empty value outside the allowlist is rejected with a message listing the
  allowed values.

---

## 3. The pending-confirmation state

Scheduling is a two-turn interaction: the assistant first previews the action, then
waits for the user to approve it on the **next** message. Because there is no
server-side session, the state needed to execute on approval is carried by the
client between turns.

### 3.1 Wire contract (spec §7.2)

```
ChatRequest  { message, history, pending_action? }
ChatResponse { reply,   history, pending_action? }

PendingAction {
  elevator_id     int      // resolved during Phase 1
  inspection_date string   // normalized YYYY-MM-DD
  inspection_type string   // one of the five values, or "" 
  reason          string   // the originating user message (capped at 500 chars)
  summary         string   // the human-readable preview shown to the user
  signature       string   // HMAC over the execution fields (see §6)
}
```

The `signature` binds the four execution fields to a server secret. Because the
client holds and replays this object, the signature is what makes Phase 1 a real
gate — Phase 2 refuses any `pending_action` it did not itself sign.

`pending_action` is `null` on a normal turn. It becomes a populated object only
after a successful Phase 1 preview, and returns to `null` the moment the action is
confirmed, cancelled, or abandoned.

### 3.2 Client-side transport

| Location | Mechanism |
|---|---|
| `platform/layout.html` | `<input id="chatPendingAction" type="hidden" name="pending_action" value="null">` |
| `platform/_chat_reply.html` | OOB swap rewrites the hidden input with the new `pending_action` |
| `platform/_chat_clear.html` | OOB reset to `null` when the conversation is cleared |
| `platform/server.py` | reads `pending_action` from the form, forwards it to the Go API only when it is a JSON object, and renders the API's returned value back into the hidden field |

### 3.3 State transitions

```
            ┌────────────────────────────────────────────────┐
            │                  pending = null                 │
            └───────────────┬────────────────────────────────┘
        action intent +     │
        valid elevator/date │  Phase 1 preview succeeds
                            ▼
            ┌────────────────────────────────────────────────┐
            │        pending = {elevator, date, type, …}      │
            └───┬──────────────┬───────────────────┬──────────┘
       "yes" /  │      "no" /   │     anything      │
       confirm  │      cancel   │     else          │
                ▼              ▼                     ▼
         Phase 2 write    no write,           pending cleared,
         then pending     pending cleared     message reclassified
         = null           (clear message)     as a fresh intent
```

Per spec §7.2, **any** message that is not a recognized confirmation or
cancellation abandons the pending action — it is never silently carried forward.

---

## 4. Parameter extraction

Extraction is deterministic and rule-based (no LLM), implemented in
`platform/api/intent.go`. The same input always yields the same entities.

### 4.1 Elevator ID

Two regexes, applied in order and de-duplicated:

- **Context-cued:** `elevator|device|unit|elev|car|id` (or `#`) followed by 1–8
  digits — catches short IDs like "elevator 42".
- **Standalone:** a bare run of 5–8 digits — long enough to avoid matching a
  4-digit year.

### 4.2 Date

- **ISO:** `YYYY-MM-DD`.
- **Month-name:** `January 15`, `Jan 15, 2026`. A missing year defaults to the
  current year (`now.Year()`, passed in so the function stays pure).
- All dates are normalized to `YYYY-MM-DD` before storage in `pending_action`.

### 4.3 Inspection type

`extractInspectionType()` matches the message against the five allowed values,
**ordered most-specific first** so that a more specific cue wins:

```
incident / accident / near-miss   → Incident
alteration / modification         → Alteration
initial                           → Initial
follow up / follow-up / followup  → Followup
periodic / annual / routine       → Periodic
(no match)                        → ""   (defaults to Periodic at write time)
```

Because matching is ordered, a message mentioning more than one cue resolves to the
first match in this list (e.g. "periodic inspection after an incident" → `Incident`).

### 4.4 The "needs more info" gate

An action intent only proceeds to Phase 1 when **both** an elevator ID and a date
were extracted (`chat.go`). If either is missing, the assistant receives an
`[ACTION NEEDS MORE INFO]` context tag and asks the user for the missing field — no
tool is called and no values are invented.

---

## 5. Validation

All validation lives in the MCP server's Pydantic model
(`ScheduleInspectionInput`, strict mode) so it runs identically on both phases and
cannot be bypassed by the caller.

| Field | Rule | On failure |
|---|---|---|
| `elevator_id` | positive integer, ≤ 9,999,999 | rejected at model layer |
| `elevator_id` | must exist in `elevators` table | `success=false`, `error` set (checked at runtime, not in the model) |
| `inspection_date` | parseable `YYYY-MM-DD`, **not in the past** (`< date.today()`) | rejected with format / past-date message |
| `reason` | non-empty after strip, ≤ 500 chars | over-length input is **capped to 500 runes** by `capReason()` in the Go layer before it reaches the model, so a long message no longer hard-fails |
| `inspection_type` | `""` or one of the five allowed values (case-sensitive) | rejected with allowlist |
| `confirmed` | strict `bool` — `"yes"` or `1` are **not** coerced | rejected at model layer |

**Error surfacing.** Validation failures are made visible to the user rather than
swallowed. In `chat.go`, an MCP error on an action intent injects an
`[ACTION VALIDATION ERROR]` context tag (with the Pydantic boilerplate stripped by
`cleanValidationError()`); the system prompt instructs the assistant to state the
problem plainly and **not** show a confirmation prompt. The elevator-not-found case
(`success=false` + string `error`) is detected by `extractScheduleError()`.

**SQL safety.** Every query uses asyncpg parameterized placeholders
(`$1, $2, …`) — no string interpolation of user input into SQL.

---

## 6. Confirmation flow (the write gate)

The database write is gated by a two-phase protocol in
`platform/mcp/tools/write_tools.py`.

### Phase 1 — preview (`confirmed=false`)

1. Validate inputs (§5) and confirm the elevator exists.
2. Return a summary dict **without writing**:
   ```json
   { "success": false, "confirmed": false, "pending_confirmation": true,
     "summary": "You are about to schedule an inspection: …" }
   ```
3. `chat.go` `buildPendingAction()` detects `pending_confirmation=true` and builds
   the `PendingAction` returned to the client.

### Phase 2 — execute (`confirmed=true`)

Triggered only when the client carries a `pending_action` **and** the user's next
message is a confirmation. `detectConfirmation()` in `chat.go` runs **before** the
intent classifier so that a bare "yes" is never misclassified:

- Confirm words: `yes, confirm, confirmed, proceed, approve, ok, okay, sure`
- Cancel words: `no, cancel, nope, stop, abort, decline, nevermind`
- Matching is **word-level** (`strings.Fields`), so "know" never matches "no".

**Integrity gate (HMAC).** Before building Phase 2 arguments, `chat.go` verifies
the `pending_action` signature with `verifyPendingAction()`. The signature is an
HMAC-SHA256 over `elevator_id | inspection_date | inspection_type | reason`,
computed in Phase 1 with a server secret (`CHAT_SIGNING_SECRET`, or an ephemeral
per-process secret if unset). A forged or tampered `pending_action` fails
verification and is rejected with an `[ACTION VALIDATION ERROR]` — no write occurs.
This is what makes Phase 1 enforceable rather than advisory: Phase 2 only executes
field values that a genuine Phase 1 preview on this server produced. (The `summary`
is intentionally excluded from the HMAC — it is cosmetic and never drives the write.)

**Duplicate guard (idempotency).** A partial unique index, `uq_pending_inspection`
on `(elevator_id, earliest_inspection_date, inspection_type)` where
`outcome = 'Pending' AND inspection_id >= 9000000`, prevents a double-submit or
replay from creating two identical pending inspections. On conflict, the tool
returns a clear "already scheduled — no duplicate was created" message instead of
inserting a second row. The index is scoped to chatbot-created rows so it never
conflicts with imported source data.

On confirmation, `chat.go` rebuilds the tool arguments from the stored
`pending_action`, sets `confirmed=true`, and calls `schedule_inspection`. The write
runs inside a single transaction:

```
BEGIN
  INSERT INTO inspections (...)            -- id from mcp_inspection_id_seq (≥ 9,000,000)
  INSERT INTO scheduling_audit_log (...)   -- outcome = 'success'
COMMIT
```

Both inserts share one transaction, so the audit row can never diverge from the
inspection it records. New inspection IDs come from a dedicated sequence starting at
9,000,000 — above the source-data range — to avoid collisions with imported records.

### Cancellation

A cancel word skips the classifier and the MCP call entirely (no write occurs) and
injects an `[ACTION CANCELLED]` context tag. The system prompt binds the assistant
to a clear confirmation that nothing was scheduled and nothing was written.

### The `MCP_SKIP_CONFIRMATION` escape hatch

When the environment variable `MCP_SKIP_CONFIRMATION` is set, Phase 1 is bypassed
and the very first call writes immediately. **This is a test-only affordance.** It
must never be set in any environment that serves real users, because it removes the
human-approval gate.

---

## 7. Audit trail

Both successful and failed **confirmed** scheduling actions are recorded in
`scheduling_audit_log` (migration `platform/api/migrations/002_audit_log.sql`; the
MCP server also creates the table idempotently at startup in `db.py`):

| Column | Notes |
|---|---|
| `log_id` | `BIGSERIAL` primary key |
| `elevator_id`, `inspection_id` | not FK-constrained — audit rows are immutable and must survive source deletes; `inspection_id` is `NULL` on error rows |
| `inspection_date`, `inspection_type`, `reason` | as written |
| `outcome` | `'success'` on commit; `'error'` when a confirmed attempt fails (elevator gone at execution time, duplicate, or mid-write exception) |
| `error_message` | the failure detail for `'error'` rows (truncated to 1000 chars); `NULL` on success |
| `performed_at` | `TIMESTAMPTZ`, defaults to `now()` |

Error rows are written **best-effort** by `_audit_error()` in a separate
connection that never raises — an audit failure must not mask the original error or
the DB outage that may have caused it. Pydantic input-validation failures are not
audited: they re-raise to the caller and are surfaced to the user directly, and no
normalized values exist to record. Phase 1 preview failures are not audited either —
only a confirmed (Phase 2) attempt is a real action.

---

## 8. Files

| File | Responsibility |
|---|---|
| `platform/api/intent.go` | Deterministic extraction of elevator ID, date, inspection type |
| `platform/api/chat.go` | Confirmation detection, two-phase routing, error surfacing |
| `platform/api/models.go` | `ChatRequest` / `ChatResponse` / `PendingAction` wire types |
| `platform/api/mcp_client.go` | MCP JSON-RPC over Streamable HTTP |
| `platform/mcp/tools/models.py` | Pydantic validation + inspection-type allowlist |
| `platform/mcp/tools/write_tools.py` | Two-phase `schedule_inspection`, transactional write + audit |
| `platform/mcp/db.py` | Connection pool, inspection-ID sequence, audit-table + duplicate-guard DDL |
| `platform/api/migrations/002_audit_log.sql` | `scheduling_audit_log` schema |
| `platform/api/migrations/003_pending_inspection_unique.sql` | `uq_pending_inspection` duplicate guard |
| `platform/api/prompts/system_prompt.md` | Identity statement of the one write action + confirmation-before-action principle, plus action-context rules (cancel / error / needs-info / pending) |
| `platform/server.py`, `layout.html`, `_chat_reply.html`, `_chat_clear.html` | Client-side `pending_action` transport |
| `platform/mcp/tools/test_schedule_inspection.py` | Integration tests for the write flow (success, validation failure, idempotency, preview) |
| `platform/api/chat_test.go` | Unit tests for confirmation detection, HMAC signing, reason capping |

---

## 9. Testing

The flow is covered at the layer where each behavior actually lives: confirmation
detection and signing in Go (no DB), the database write and its side effects in
Python (integration, real PostgreSQL).

### Go unit tests — `platform/api/chat_test.go`

| Test | Verifies |
|---|---|
| `TestDetectConfirmation` | yes/cancel/neither word-level detection, incl. "know" ≠ "no" and the abandon case |
| `TestSignAndVerifyPendingAction` | a freshly signed `pending_action` verifies |
| `TestVerifyPendingActionRejectsTampering` | mutating any execution field invalidates the signature |
| `TestVerifyPendingActionRejectsUnsigned` | an unsigned `pending_action` never verifies |
| `TestCapReason` | over-length reason capped to 500 runes, multibyte-safe |

Run: `cd platform/api && go test .`

### Python integration tests — `platform/mcp/tools/test_schedule_inspection.py`

Each test patches `get_connection` with a direct asyncpg connection and seeds a
synthetic elevator (`id ≥ 9,000,000`); the fixture creates the sequence, audit
table, and duplicate-guard index from `db.py`'s DDL so it runs without `init_pool()`
or a specific migration order, then cleans up.

| Test | Scenario | Asserts |
|---|---|---|
| `test_success_path_writes_inspection_and_audit` | **Success path** | one `inspections` row + one `'success'` audit row, `error_message` NULL |
| `test_confirmed_write_to_missing_elevator_errors_and_audits` | **Validation failure** (runtime) | no inspection, one `'error'` audit row with the message |
| `test_preview_of_missing_elevator_does_not_audit` | preview of a bad ID | no audit row (a preview is not an auditable action) |
| `test_duplicate_confirmation_is_idempotent` | **Duplicate confirmation** | second write blocked, exactly one `'Pending'` row, audit shows `['error','success']` |
| `test_phase1_preview_writes_nothing` | **Phase 1 preview** | summary returned, neither table written |

Input-shape validation (out-of-range ID, past date, bad type, non-strict
`confirmed`, over-length reason) is covered in `test_validation.py`. **Cancellation**
is verified by `TestDetectConfirmation` in Go — the cancel path short-circuits in
`chat.go` before any MCP call, so there is nothing to assert at the Python layer.

Run (needs PostgreSQL; the host port is 5433 with the bundled Docker DB):
```
DATABASE_URL=postgresql://api_user:<pw>@localhost:5433/rocket_elevators \
  PYTHONPATH=. pytest platform/mcp/tools/test_schedule_inspection.py -v
```

### CI

`.github/workflows/ci.yml` applies migrations 001–003 before the Python job and runs
this file as a dedicated step. CI sets `MCP_SKIP_CONFIRMATION=1` globally; the two
preview tests defend against that with a `_no_skip_confirmation()` helper that
removes the variable for the duration of the call, so the two-phase path is exercised
deterministically regardless of the ambient environment.
