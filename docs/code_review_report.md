# AND-105 Task 5: Code Review Report — PostgreSQL Integration

**Branch:** `database-integration`
**Date:** 2026-06-03
**Reviewer methodology:**
- Reviewer agent A: SQL safety and injection analysis (fan-out)
- Reviewer agent B: Connection handling, pooling, and error flow analysis (fan-out)
- Independent pass: dead code, sentinel error idioms, edge cases

---

## CRITICAL

*No critical issues found.*

---

## WARNINGS

### W1 — `handlers.go:271, 450` — Sentinel error compared with `==` instead of `errors.Is`

**Source:** sql-reviewer

```go
// Line 271
if err == pgx.ErrNoRows {

// Line 450
if err == pgx.ErrNoRows {
```

`pgx.ErrNoRows` is a sentinel error value. Direct equality (`==`) works today because pgx v5's `QueryRow().Scan()` returns the sentinel unwrapped. However, if pgx wraps the error in a future release (or if any middleware wraps it), `==` silently falls through to the `if err != nil` branch, returning HTTP 500 instead of 404 for legitimate "not found" lookups. The Go-idiomatic pattern is `errors.Is(err, pgx.ErrNoRows)`, which traverses the error chain correctly.

**Fix:** Replace both comparisons with `errors.Is`.

---

### W2 — `db.go:35, 41` — No per-attempt timeout in `InitDB` retry loop

**Source:** conn-reviewer

```go
pool, err = pgxpool.New(context.Background(), dsn)   // line 35
// ...
if err = pool.Ping(context.Background()); err != nil { // line 41
```

Both `pgxpool.New` and `pool.Ping` use `context.Background()`, which has no deadline. If the database host is unreachable but accepts TCP connections (e.g., a firewall black-hole), each attempt can stall indefinitely before timing out at the OS TCP level (minutes on Windows). The retry loop runs up to 10 times, so startup could block for 10× OS timeout instead of the intended ~20 seconds.

**Fix:** Use `context.WithTimeout(context.Background(), 5*time.Second)` per attempt.

---

## SUGGESTIONS

### S1 — `data.go:8–13` — Dead code: `nullableString()` is never called

**Source:** independent

The helper `nullableString(s string) *string` is defined but has zero call sites. The migration from CSV-based loading left it behind. Dead exported-style functions can mislead readers about the intended API surface.

**Fix:** Remove the function.

---

### S2 — `handlers.go:111` — LIKE wildcards in `search` parameter are unescaped

**Source:** sql-reviewer (downgraded from CRITICAL — the value IS parameterized, ruling out injection)

```go
args = append(args, "%"+strings.ToLower(search)+"%")
```

The search value is safely parameterized, so there is no SQL injection risk. However, LIKE metacharacters `%` and `_` in user input are not escaped before wrapping. Sending `?q=%` matches every row (like no filter); `?q=_` matches any single character position. For a dashboard read endpoint with no authentication this is a nuisance rather than a vulnerability, but the behaviour is surprising.

**Fix (optional):** Escape `%` and `_` in the search value before wrapping: `strings.ReplaceAll(strings.ReplaceAll(search, "\\", "\\\\"), "%", "\\%")` with `... LIKE $N ESCAPE '\'`.

---

### S3 — `handlers.go:573` — `WHERE p.risk_level = 'HIGH'` is case-sensitive

**Source:** sql-reviewer

The ETL always loads `risk_level` as uppercase, so this works today. Other queries use `LOWER()` for consistency. If the ETL is ever updated to store mixed case, this filter would silently return no alerts without any error.

**Fix (optional):** Change to `WHERE LOWER(p.risk_level) = 'high'` for defensive consistency.

---

### S4 — `handlers.go:27–31` — `json.Encode` error silently discarded in `writeJSON`

**Source:** conn-reviewer

```go
func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(v)   // error never checked
}
```

Once `WriteHeader` has been called, the status code cannot be changed. If `Encode` fails (e.g., a `map[string]int` with a non-string key, network reset mid-write), the client receives a partial response body. There is nothing useful to do about the status code at this point, but the failure could be logged.

**Fix (optional):** `if err := json.NewEncoder(w).Encode(v); err != nil { log.Printf("writeJSON encode: %v", err) }`.

---

### S5 — `handlers.go:53–55` — No upper bound on `page` parameter

**Source:** sql-reviewer (downgraded from WARNING — practical impact is nil)

```go
if n, err := strconv.Atoi(v); err == nil && n > 0 {
    page = n
}
```

There is no cap on `page`. For `page` values large enough to cause `(page - 1) * limit` to overflow `int64`, the result wraps to a negative or unexpected offset. The `if offset < total` guard at line 132 prevents any DB query from running for out-of-range pages, so the overflow has no practical effect given dataset size (~43 000 rows). Adding a cap (e.g., 100 000) documents intent.

---

## FALSE POSITIVES NOTED

The following findings from the reviewers were investigated and ruled out:

| Reviewer claim | Verdict | Reason |
|---|---|---|
| CRITICAL: SQL injection via `fmt.Sprintf(where)` | **False positive** | `conds` strings are hardcoded Go literals (`"e.status = $%d"` etc.); only values come from user input via `args[]` |
| CRITICAL: Connection leak in `InitDB` retry loop | **False positive** | `pool.Close()` is called on every ping failure; the final `return fmt.Errorf` is only reached after the last attempt's pool is already closed |
| CRITICAL: Double `WriteHeader` in `writeJSON` | **False positive** | Each handler has exactly one `writeJSON` call per code path; no scenario calls it twice in the same request |
| CRITICAL: `rows.Close()` error discarded | **False positive** | pgx v5 `Rows.Close()` has no return value; there is no error to check |

---

## VERDICT

**No critical issues.** The PostgreSQL migration is safe from SQL injection — all user values are parameterized, sort columns derive from a validated whitelist, and no user-controlled strings reach SQL structure.

**Fix applied (see commit):** W1 — replaced `err == pgx.ErrNoRows` with `errors.Is(err, pgx.ErrNoRows)` at `handlers.go:271` and `handlers.go:450`.

**Remaining warnings are low operational risk:**
- W2 (InitDB timeout) only matters if the container network is black-hole slow; the Docker `service_healthy` gate makes this extremely unlikely in the deployed setup.

**The three most impactful optional improvements** (not blocking): S1 (remove dead code), S2 (LIKE escape), S3 (case-insensitive risk_level filter).
