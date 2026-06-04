# AND-106 Module Improvements

High-value improvements to implement after completing all base tasks.

---

## 1. Async parallel Ollama calls in the generation script

**Task:** 7  
**Impact:** Critical — 27,626 HIGH-risk elevators at 15s each = ~115 hours sequentially

Use `asyncio` with a bounded semaphore to run 4 concurrent Ollama requests, reducing total
runtime to ~29 hours. With a GPU-accelerated Ollama instance this drops further.

```python
import asyncio, httpx

async def call_ollama_async(client, semaphore, system, user):
    async with semaphore:
        resp = await client.post(f'{OLLAMA_URL}/api/chat', json={...}, timeout=300)
        return resp.json()['message']['content'].strip()

async def run_all(elevators):
    sem = asyncio.Semaphore(4)
    async with httpx.AsyncClient() as client:
        tasks = [call_ollama_async(client, sem, SYSTEM_BEST, user_msg(e)) for e in elevators]
        return await asyncio.gather(*tasks, return_exceptions=True)
```

---

## 2. Checkpoint and resume for the generation script

**Task:** 7  
**Impact:** Critical — a crash at elevator 20,000 restarts from zero without this

Skip elevators that already have a non-NULL `risk_explanation`. The `UPDATE ... WHERE
risk_explanation IS NULL` pattern is sufficient: if the script is interrupted and restarted,
completed rows are skipped automatically. Add a `--force` flag to regenerate all explanations
when the prompt version changes.

---

## 3. Serve `risk_explanation` from the Go API `/risk` endpoint

**Task:** 7  
**Impact:** High — required by the Task 7 spec; currently missing from the API response

Two changes needed:

```go
// models.go — add nullable field
type RiskResponse struct {
    ...
    RiskExplanation *string `json:"risk_explanation"`
}

// handlers.go — include column in SELECT
SELECT elevator_id::text, risk_score::float8, risk_level,
       risk_explanation, model_version, prediction_date
FROM predictions WHERE elevator_id = $1
```

`*string` (nullable pointer) is correct since LOW and MEDIUM elevators will not have
explanations populated by the generation script.

---

## 4. LLM-as-judge accuracy evaluation

**Task:** 6 / 7  
**Impact:** Medium — automates the manual spot-check, catches factual errors before DB write

After generating each explanation, a second Ollama call scores it against the source data:

```
System: You are evaluating an elevator risk explanation for accuracy.
        Return JSON: {"accuracy": 1-5, "specificity": 1-5, "flags": [...]}
        accuracy=1 means the explanation contradicts or fabricates data.
        flags lists any specific inaccuracies found.

User:   Source data: [user_msg output]
        Explanation: [generated text]
```

Explanations with `accuracy < 3` are flagged for manual review and not written to the database.

---

## 5. Structured JSON output

**Task:** 6 / 7  
**Impact:** Medium — makes explanations machine-readable; enables richer dashboard display

Request a JSON object instead of free-text sentences:

```json
{
  "primary_factor": "Two unresolved follow-up inspections since 2015",
  "evidence": "Followup inspections on 2015-10-19 and 2016-04-27 both failed",
  "recommendation": "Verify resolution status of 2016 compliance order before next service"
}
```

`primary_factor` powers a short risk badge tooltip; `recommendation` maps to a technician
action item. Requires updating `risk_explanation` to a `JSONB` column or adding separate
`risk_primary_factor` and `risk_recommendation` columns.
