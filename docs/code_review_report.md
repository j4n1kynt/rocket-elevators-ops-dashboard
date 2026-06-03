# AND-105 Task 5: Code Review Report — PostgreSQL Integration

**Branch:** `database-integration`
**Date:** 2026-06-03

## Review Methods Used

| Method | Scope | Session |
|--------|-------|---------|
| Parallel Explore agents (A: SQL safety, B: connection/pooling) | All Go API files | db-writer (current) |
| `/code-review` skill — 7-angle finder + verifier | Go API diff vs pre-AND-105 base | db-writer |
| `/security-review` skill — vulnerability + FP filter | Branch diff | db-writer |
| `claude -p` fan-out — handlers.go | SQL injection + row handling | db-writer |
| `claude -p` fan-out — db.go | Connection lifecycle + credentials | db-writer |

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

### W3 — `db.go:28` — Password not URL-encoded in DSN construction

**Source:** fan-out db.go *(new — not caught by Explore agents or `/code-review`)*

```go
dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s", user, password, host, port, dbname)
```

If `DB_PASSWORD` contains any URL-reserved character (`@`, `#`, `?`, `/`, `:`), the DSN is silently malformed — pgx parses the wrong host, port, or database name rather than failing with a clear authentication error. Enterprise and production password policies commonly require special characters, making this a latent hard-to-diagnose failure.

**Fix:** Use `pgxpool.ParseConfig` with `config.ConnConfig.Password` set directly (bypasses URL-encoding entirely), or URL-encode the credentials:
```go
dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
    url.QueryEscape(user), url.QueryEscape(password), host, port, dbname)
```

**Status: ✅ Fixed** — `url.QueryEscape` applied to both user and password at `db.go:28` in commit following `55cd124`.

---

## SUGGESTIONS

### S1 — `data.go:8–13` — Dead code: `nullableString()` is never called

**Source:** Independent · `/code-review` · fan-out handlers.go

The helper `nullableString(s string) *string` was used by `LoadFleetCSV` to handle empty CSV fields. All callers were deleted in the CSV-to-DB migration. The dead definition misleads readers about the intended API surface.

**Fix:** Remove the function.

---

### S2 — `handlers.go:111` — LIKE wildcards in `search` parameter are unescaped

**Source:** Explore agents (sql-reviewer) — downgraded from CRITICAL (value IS parameterized, no injection risk)

```go
args = append(args, "%"+strings.ToLower(search)+"%")
```

`?q=%` matches every row (effectively removes the filter); `?q=_` matches any single character. No security risk for a read-only dashboard, but the behaviour is surprising.

**Fix (optional):** Escape `%` and `_` before wrapping, with `LIKE $N ESCAPE '\'`.

---

### S3 — `handlers.go:573` — `WHERE p.risk_level = 'HIGH'` is case-sensitive

**Source:** Explore agents (sql-reviewer) · `/code-review` · fan-out handlers.go — confirmed by verifying ETL writes uppercase

The ETL (`generate_predictions.py`) writes uppercase `HIGH`/`MEDIUM`/`LOW`, so this is correct today. `GetFleetStats` defensively uses `LOWER(p.risk_level)` in its CASE statements; `GetFleetAlerts` does not. If the predictions source ever changes casing, alerts silently return zero results.

**Fix (optional):** `WHERE LOWER(p.risk_level) = 'high'` for defensive consistency.

---

### S4 — `handlers.go:27–31` — `json.Encode` error silently discarded in `writeJSON`

**Source:** Explore agents (conn-reviewer)

```go
json.NewEncoder(w).Encode(v)   // error never checked
```

After `WriteHeader` is called, the status code is fixed. If `Encode` fails (network reset mid-write), the client receives a truncated body with no indication. Nothing useful can be done about the status, but the failure could be logged.

**Fix (optional):** `if err := json.NewEncoder(w).Encode(v); err != nil { log.Printf("writeJSON encode: %v", err) }`.

---

### S5 — `handlers.go:53–55` — No upper bound on `page` parameter

**Source:** Explore agents (sql-reviewer) · `/code-review`

No cap on `page`. Astronomically large values cause `(page-1)*limit` to overflow `int64`. The `if offset < total` guard at line 132 prevents any DB query from running, so there is no practical impact given dataset size (~43 000 rows). Adding a cap documents intent.

---

### S6 — `handlers.go:194, 388` — Inline date formatting bypasses `formatDate()` helper

**Source:** `/code-review` (PLAUSIBLE — confirmed as code pattern inconsistency)

```go
licExp = licExpiry.Format("2006-01-02")   // line 195 — inline
dateStr = inspDate.Format("2006-01-02")  // line 388 — inline
```

Nullable `*time.Time` fields use `formatDate()` (lines 205, 293, 616); non-nullable fields format inline. Both are correct today but a format change (e.g. adding timezone) must be made in two places. All date formatting should go through `formatDate()` or a non-pointer variant.

---

## FALSE POSITIVES NOTED

| Reviewer claim | Method | Verdict | Reason |
|---|---|---|---|
| CRITICAL: SQL injection via `fmt.Sprintf(where)` | Explore agents | **False positive** | `conds` strings are hardcoded Go literals; only values come from user via `args[]` |
| CRITICAL: Connection leak in `InitDB` retry loop | Explore agents | **False positive** | `pool.Close()` called on every ping failure; final `return` reached only after pool is closed |
| CRITICAL: Double `WriteHeader` in `writeJSON` | Explore agents | **False positive** | Each handler has exactly one `writeJSON` call per code path |
| CRITICAL: `rows.Close()` error discarded | Explore agents | **False positive** | pgx v5 `Rows.Close()` has no return value |
| CRITICAL: DSN password logged on connect error | fan-out db.go | **False positive** | pgx v5 actively redacts passwords via `redactPW()` in all error formatting paths |
| HIGH: Hardcoded credentials in docker-compose.yml | `/security-review` | **False positive** | Development-only placeholder credentials scoped to local Docker network; excluded per "env vars are trusted values" rule |
| CRITICAL: `err == pgx.ErrNoRows` masked wrapped errors | Explore agents | **Fixed (was valid)** | Real latent risk; fixed in commit `c8e468a` |

---

## VERDICT

**No critical issues.** The PostgreSQL migration is safe from SQL injection — all user values are parameterized, sort columns derive from a validated whitelist, and no user-controlled strings reach SQL structure. Three independent review methods (Explore agents, `/code-review`, fan-out) reached the same conclusion.

**Fixes applied:**
- W1 (`errors.Is`) — committed in `c8e468a`
- W3 (DSN URL encoding) — `url.QueryEscape` on user + password at `db.go:28`

**New finding from fan-out (W3):** Not caught by Explore agents or `/code-review`; surfaces the value of independent file-focused review. Fixed before merge.

**Remaining open items (non-blocking):**
1. W2 — InitDB per-attempt timeout (startup safety; mitigated by Docker `service_healthy` gate)
2. S1 — Remove dead `nullableString()` function
3. S3 — Case-insensitive risk_level filter in GetFleetAlerts
