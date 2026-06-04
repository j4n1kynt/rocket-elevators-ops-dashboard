# AND-106 Module Improvements

Improvements identified after completing Tasks 6 and 7 (LLM risk explanation prototype and
generation pipeline). Ordered by impact.

---

## 1. Harden V3 into V4 — apply reviewer findings

**Task:** 6  
**Impact:** High — addresses confirmed hallucination triggers in the production prompt  
**Status:** Identified but not implemented

The Writer/Reviewer session identified four concrete fixes for `SYSTEM_V3` that were documented
but never applied:

| Issue | Fix |
|-------|-----|
| Expertise persona licenses hallucination | Weaken `"with expertise in TSSA compliance"` → `"familiar with TSSA compliance requirements"` |
| No data-scope fence | Add: `"Use only the information provided in this message. Do not add knowledge from outside the data."` |
| No null/empty field handling | Add: `"If a category has no records, state that explicitly rather than omitting it."` |
| No benign-case instruction | Add: `"If all indicators are benign, state that directly — do not manufacture risk concerns to fill the required sentences."` |

A hardened V4 prompt incorporating these changes should be tested against the same cross-tier
sample (HIGH/MEDIUM/LOW) to verify the improvements before using it in `generate_explanations.py`.

---

## 2. Add quantitative scoring to prompt comparison

**Task:** 6  
**Impact:** Medium — makes V1/V2/V3 comparison objective rather than subjective  
**Status:** Not implemented

The current comparison (cell-c05/c06) is qualitative. A simple automated scoring rubric applied
to each output would make future prompt changes measurable:

| Metric | Measurement |
|--------|-------------|
| Specificity | At least one date or count cited (regex match) |
| Groundedness | No hedging phrases present (`may indicate`, `could suggest`, `appears to`, `seems`) |
| Brevity compliance | 2–3 sentences (sentence tokenizer) |
| Data fidelity | No dates or IDs mentioned that don't appear in the source data |

Scoring can be implemented with a Python function that returns a 0–4 integer per explanation.
Aggregate across 3 elevators per prompt version to produce a comparable score.

---

## 3. Three-parallel-worktree reviewer for V4 validation

**Task:** 6  
**Impact:** Medium — catches more issues than single-reviewer pass  
**Status:** Technique documented in AI Interaction Log; not yet applied to harden the prompt

After writing V4 (improvement 1), run three simultaneous `claude --worktree` sessions with
distinct review angles rather than a single fresh-context reviewer:

- **Reviewer A** — hallucination triggers and data fabrication risks
- **Reviewer B** — instruction ambiguity and incompatible interpretations
- **Reviewer C** — missing guardrails, null cases, benign-case handling

Merge findings and apply confirmed issues (found by ≥2 reviewers) to produce V4.1.
See AI Interaction Log entry "AND-106 Task 6 (addendum)" for full technique documentation.

---

## 4. Async parallel Ollama calls

**Task:** 7  
**Impact:** High — 27,626 HIGH-risk elevators at 15s each = ~115 hours sequentially  
**Status:** Not implemented

`generate_explanations.py` currently calls Ollama sequentially. Using `asyncio` with a bounded
semaphore (e.g., `asyncio.Semaphore(4)`) allows 4 concurrent Ollama requests, reducing total
runtime to ~29 hours. With a GPU-accelerated Ollama instance, concurrent requests + faster
inference could reduce this to under 1 hour.

```python
import asyncio, httpx

async def call_ollama_async(client, semaphore, system, user):
    async with semaphore:
        resp = await client.post(...)
        return resp.json()['message']['content'].strip()

async def run_all(elevators):
    sem = asyncio.Semaphore(4)
    async with httpx.AsyncClient(timeout=300) as client:
        tasks = [call_ollama_async(client, sem, SYSTEM_BEST, user_msg(e)) for e in elevators]
        return await asyncio.gather(*tasks, return_exceptions=True)
```

---

## 5. Checkpoint and resume for the generation script

**Task:** 7  
**Impact:** High — without checkpointing, a crash at elevator 20,000 restarts from zero  
**Status:** Not implemented

`generate_explanations.py` should skip elevators that already have a non-NULL `risk_explanation`
in the `predictions` table. The `UPDATE ... WHERE risk_explanation IS NULL` pattern is sufficient:
if the script is interrupted and restarted, completed rows are skipped automatically. Adding a
`--force` flag to regenerate all explanations is useful for prompt version upgrades.

---

## 6. LLM-as-judge accuracy evaluation

**Task:** 6  
**Impact:** Medium — automates the manual spot-check step  
**Status:** Not implemented

After generating each explanation, a second Ollama call evaluates it against the source data:

```
System: You are evaluating whether an elevator risk explanation is accurate.
        Rate 1-5 for: (a) factual accuracy — does it match the source data?
        (b) specificity — does it cite specific dates or counts?
        (c) operational usefulness — would a field technician act on this?
        Return JSON: {"accuracy": N, "specificity": N, "usefulness": N, "flags": [...]}

User:   Source data: [user_msg output]
        Explanation: [generated explanation]
```

Explanations scoring below 3 on accuracy are flagged for manual review before being written
to the database.

---

## 7. Structured JSON output from the LLM

**Task:** 6 / 7  
**Impact:** Medium — makes explanations machine-readable for dashboard use  
**Status:** Not implemented

Instead of free-text 2–3 sentences, request a structured JSON object:

```json
{
  "primary_factor": "Two unresolved follow-up inspections since 2015",
  "evidence": "Followup inspections on 2015-10-19 and 2016-04-27 both failed",
  "recommendation": "Verify resolution status of 2016 compliance order before next use"
}
```

Benefits: the dashboard can render each field separately; the `primary_factor` field alone can
power a short risk badge tooltip; the `recommendation` field maps directly to a technician
action item. Requires updating `risk_explanation` column type or adding separate columns.

---

## 8. Temperature sensitivity test

**Task:** 6  
**Impact:** Low — informational, improves confidence in output stability  
**Status:** Not implemented

All current Ollama calls use `temperature=0.1`. Running the same elevator at `0.0`, `0.1`, and
`0.3` would show how stable the V3 output is. High variance at low temperature suggests the
prompt is under-specified; very low variance at higher temperatures suggests the model is
over-constrained. Ideal: low variance in key facts (dates, counts cited) with some natural
language variation in phrasing.

---

## 9. Cross-tier sampling as default for prototype evaluation

**Task:** 6  
**Impact:** Low — already implemented; document as a pattern  
**Status:** Implemented in `risk_explanations.ipynb`

Sampling 4 HIGH + 4 MEDIUM + 4 LOW rather than top-10 HIGH surfaces prompt failure modes that
only appear on benign data (the "manufactured concern" pattern in LOW-risk outputs). Any future
prompt prototype notebooks should use cross-tier sampling by default rather than selecting only
the highest-risk elevators.

---

## 10. /risk endpoint: serve risk_explanation field

**Task:** 7  
**Impact:** Medium — required by Task 7 spec; currently missing from API response  
**Status:** Not implemented

`RiskResponse` in `models.go` does not include `risk_explanation`. The `/api/elevators/{id}/risk`
endpoint SQL query does not select the column. Changes needed:

```go
// models.go
type RiskResponse struct {
    ...
    RiskExplanation *string `json:"risk_explanation"` // nullable
}

// handlers.go — GetElevatorRisk SQL
SELECT elevator_id::text, risk_score::float8, risk_level,
       risk_explanation, model_version, prediction_date
FROM predictions WHERE elevator_id = $1
```

The field should be a nullable pointer (`*string`) since LOW and MEDIUM elevators will not have
explanations populated by the generation script.
