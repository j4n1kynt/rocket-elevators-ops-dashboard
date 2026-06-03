# AND-105 Task 5: Code Review Report — PostgreSQL Integration

**Branch:** `database-integration`
**Date:** 2026-06-03

## Review Methods Used

| Method | Scope | Session |
|--------|-------|---------|
| Parallel Explore agents (A: SQL safety, B: connection/pooling) | All Go API files | db-writer |
| `/code-review` skill — 7-angle finder + verifier | Go API diff vs pre-AND-105 base | db-writer |
| `/security-review` skill — vulnerability + FP filter | Branch diff | db-writer |
| `claude -p` fan-out — handlers.go | SQL injection + row handling | db-writer |
| `claude -p` fan-out — db.go | Connection lifecycle + credentials | db-writer |
| `claude --worktree db-reviewer` — isolated reviewer session | All Go API files (file paths only, no context) | db-reviewer (worktree) |

---

## CRITICAL

*No critical issues found.*

---

## WARNINGS

### W1 — `handlers.go:271, 450` — Sentinel error compared with `==` instead of `errors.Is`

**Source:** Explore agents (sql-reviewer) · `/code-review` · fan-out handlers.go

```go
if err == pgx.ErrNoRows {   // line 271 (GetElevatorByID)
if err == pgx.ErrNoRows {   // line 450 (GetElevatorRisk)
```

`pgx.ErrNoRows` is a sentinel error value. Direct equality works today because pgx v5's `QueryRow().Scan()` returns the sentinel unwrapped. If pgx ever wraps it, `==` silently routes the "not found" case to the 500 path instead of 404. The Go-idiomatic pattern `errors.Is` traverses the error chain correctly.

**Status: ✅ Fixed** — both comparisons replaced with `errors.Is(err, pgx.ErrNoRows)` in commit `c8e468a`.

---

### W2 — `db.go:35, 41` — No per-attempt timeout in `InitDB` retry loop

**Source:** Explore agents (conn-reviewer) · `/code-review` · fan-out db.go

```go
pool, err = pgxpool.New(context.Background(), dsn)      // line 35
if err = pool.Ping(context.Background()); err != nil {  // line 41
```

Both calls use `context.Background()` with no deadline. A firewall black-hole that accepts TCP connections but never responds causes each attempt to stall at OS TCP timeout (minutes on Windows) rather than the intended 2-second interval. 10 attempts × unbounded wait can delay startup significantly.

**Fix:** `ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second); defer cancel()` per attempt.

---

### W3 — `db.go:28` — Credentials in URL DSN break on special characters

**Source:** fan-out db.go · db-reviewer worktree session *(independently confirmed)*

```go
// original
dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s", user, password, host, port, dbname)
```

A password containing `@`, `%`, or `/` corrupts the URL. `@` shifts the host boundary; `%` breaks percent-encoding. pgx parses the wrong host or errors silently. Enterprise password policies commonly require these characters.

First fix used `url.QueryEscape` on credentials. The reviewer worktree session independently found the same issue and recommended the key=value DSN format, which requires no encoding at all. The key=value approach is adopted as the cleaner solution.

**Status: ✅ Fixed** — switched to key=value DSN format in `db.go:28`:
```go
dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s", host, port, user, password, dbname)
```

---

### W4 — `main.go:30` — No HTTP server timeouts

**Source:** db-reviewer worktree session *(new — not caught by any prior method)*

```go
http.ListenAndServe(":"+port, mux)   // no timeouts
```

No `ReadTimeout`, `WriteTimeout`, or `IdleTimeout` set. A slow or malicious client holds a goroutine open indefinitely. Under load, goroutines waiting on DB queries occupy pool connections — coupling client slowness to database exhaustion.

**Status: ✅ Fixed** — explicit server timeouts added to `main.go`:
```go
srv := &http.Server{
    Addr:         ":" + port,
    Handler:      mux,
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 15 * time.Second,
    IdleTimeout:  60 * time.Second,
}
```

---

### W5 — `main.go:9–33` — No graceful shutdown; `db.Close()` never called

**Source:** db-reviewer worktree session *(new)*

On SIGTERM/SIGINT the process exits abruptly. Any in-flight pgxpool connections are dropped without a clean `Terminate` message; ongoing transactions remain open on the server until PostgreSQL's idle-in-transaction timeout fires.

**Fix:** Add a signal goroutine: `os.Signal` → `srv.Shutdown(ctx)` → `db.Close()`.

---

### W6 — `db.go:34–49` — Default `MaxConns` may be too low under concurrent load

**Source:** db-reviewer worktree session *(new)*

`pgxpool.New` defaults to `MaxConns = max(4, runtime.NumCPU())`. `GetElevatorInspections` and `GetFleetStats` each issue 3 sequential queries per request. At low CPU count, 4 connections × 3 queries means only ~1 concurrent request makes full progress without queueing.

**Fix (optional):** Configure the pool explicitly via `pgxpool.ParseConfig` with `cfg.MaxConns = 20` and idle/lifetime limits.

---

## SUGGESTIONS

### S1 — `data.go:8–13` — Dead code: `nullableString()` is never called

**Source:** Independent · `/code-review` · fan-out handlers.go

The helper was used by `LoadFleetCSV` to handle empty CSV fields. All callers were deleted in the CSV-to-DB migration. The dead definition misleads readers about the intended API surface.

**Fix:** Remove the function.

---

### S2 — `handlers.go:111` — LIKE wildcards in `search` parameter are unescaped

**Source:** Explore agents (sql-reviewer) · db-reviewer worktree session *(independently confirmed)*

```go
args = append(args, "%"+strings.ToLower(search)+"%")
```

`?q=%` matches every row; `?q=_` matches any single-character field value. No injection risk (value is parameterized) but the behaviour is surprising.

**Fix (optional):** `strings.NewReplacer("%", "\\%", "_", "\\_").Replace(search)` with `LIKE $N ESCAPE '\'`.

---

### S3 — `handlers.go:573` — `WHERE p.risk_level = 'HIGH'` is case-sensitive

**Source:** Explore agents (sql-reviewer) · `/code-review` · fan-out handlers.go

ETL writes uppercase, so this is correct today. If predictions source ever changes casing, alerts silently return zero results.

**Fix (optional):** `WHERE LOWER(p.risk_level) = 'high'`.

---

### S4 — `handlers.go:27–31` — `json.Encode` error silently discarded in `writeJSON`

**Source:** Explore agents (conn-reviewer) · db-reviewer worktree session *(independently confirmed)*

```go
json.NewEncoder(w).Encode(v)   // error never checked
```

If `Encode` fails, the client receives a truncated body silently. The status code cannot be changed after `WriteHeader` but the failure could be logged.

**Fix (optional):** `if err := json.NewEncoder(w).Encode(v); err != nil { log.Printf("writeJSON encode: %v", err) }`.

---

### S5 — `handlers.go:53–55` — No upper bound on `page` parameter

**Source:** Explore agents (sql-reviewer) · `/code-review`

No cap on `page`. Overflow of `(page-1)*limit` is guarded by the `if offset < total` check; no practical impact given dataset size. Adding a cap documents intent.

---

### S6 — `handlers.go:194, 388` — Inline date formatting bypasses `formatDate()` helper

**Source:** `/code-review`

Non-nullable fields format inline; nullable fields use `formatDate()`. A format change must be applied in two places.

---

### S7 — `handlers.go:93–95` — Invalid `order` param silently defaults to ASC

**Source:** db-reviewer worktree session *(new)*

Any value other than `"desc"` silently becomes ASC with no 400 response. Safe but confusing to API consumers.

**Fix (optional):** Reject unknown values: `if order != "" && order != "asc" && order != "desc" { writeJSON(w, 400, ...) }`.

---

### S8 — `handlers.go:499–514` — `GetFleetStats` pass rate counts historical passes, not current status

**Source:** db-reviewer worktree session *(new)*

```sql
SELECT COUNT(DISTINCT elevator_id) FROM inspections
WHERE LOWER(outcome) IN ('passed', 'all orders resolved')
```

Counts any elevator that has ever passed, not whether its most recent inspection passed. An elevator failing since 2021 is still counted. The `GetElevators` endpoint correctly uses `DISTINCT ON` to restrict to latest inspection; `GetFleetStats` does not.

**Fix (optional):** Restrict to latest inspection per elevator via a CTE mirroring the `DISTINCT ON` pattern in `GetElevators`.

---

## FALSE POSITIVES NOTED

| Reviewer claim | Method | Verdict | Reason |
|---|---|---|---|
| CRITICAL: SQL injection via `fmt.Sprintf(where)` | Explore agents | **False positive** | `conds` strings are hardcoded Go literals; only values come from user via `args[]` |
| CRITICAL: Connection leak in `InitDB` retry loop | Explore agents | **False positive** | `pool.Close()` called on every ping failure; final `return` reached only after pool is closed |
| CRITICAL: Double `WriteHeader` in `writeJSON` | Explore agents | **False positive** | Each handler has exactly one `writeJSON` call per code path |
| CRITICAL: `rows.Close()` error discarded | Explore agents | **False positive** | pgx v5 `Rows.Close()` has no return value |
| CRITICAL: DSN password logged on connect error | fan-out db.go | **False positive** | pgx v5 actively redacts passwords via `redactPW()` in all error formatting paths |
| HIGH: Hardcoded credentials in docker-compose.yml | `/security-review` | **False positive** | Development-only placeholder credentials scoped to local Docker network |
| No SQL injection found | db-reviewer worktree | **Confirmed clean** | Independently verified: sort whitelist, parameterized values, no user input reaches SQL structure |

---

## VERDICT

**No critical issues.** SQL injection safety confirmed by four independent methods. All user values are parameterized; sort columns use a hardcoded whitelist.

**Fixes applied (4 total):**
| Fix | Commit | Source |
|-----|--------|--------|
| W1 — `errors.Is` for sentinel error comparison | `c8e468a` | Explore agents, `/code-review`, fan-out |
| W3 — key=value DSN (special chars in password) | `f481c20` → improved | fan-out db.go · worktree confirmed |
| W3 improved — switched from `url.QueryEscape` to key=value DSN format | current commit | db-reviewer worktree |
| W4 — HTTP server timeouts (`ReadTimeout`, `WriteTimeout`, `IdleTimeout`) | current commit | db-reviewer worktree *(new)* |

**Key insight from worktree reviewer:** W4 (HTTP timeouts) and W5–W6 (graceful shutdown, pool config) were missed by all prior methods — Explore agents, `/code-review`, `/security-review`, and fan-out. The worktree session with no prior context and only file paths produced three new warnings and two new suggestions, validating the Writer/Reviewer isolation workflow.

**Remaining open (non-blocking):**
1. W2 — InitDB per-attempt timeout (mitigated by `service_healthy` gate)
2. W5 — Graceful shutdown / `db.Close()` on signal
3. W6 — Explicit pool `MaxConns` configuration
4. S1, S3, S7, S8 — Dead code, case sensitivity, order validation, pass rate accuracy
