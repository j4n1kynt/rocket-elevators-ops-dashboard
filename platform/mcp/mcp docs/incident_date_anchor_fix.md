# Incident Date Anchor Fix — `get_incident_count_last_year`

## What changed

`get_incident_count_last_year` in `platform/mcp/tools/incident_tools.py` previously computed its date window relative to `CURRENT_DATE`:

```sql
WHERE date_of_occurrence >= DATE_TRUNC('year', CURRENT_DATE) - INTERVAL '1 year'
  AND date_of_occurrence <  DATE_TRUNC('year', CURRENT_DATE)
```

It now uses a hardcoded anchor date of `2016-11-22` (the latest date present in the dataset):

```sql
WHERE date_of_occurrence >= DATE_TRUNC('year', DATE '2016-11-22') - INTERVAL '1 year'
  AND date_of_occurrence <  DATE_TRUNC('year', DATE '2016-11-22')
```

The `year_queried` field in the response was updated to match (`2015` instead of `EXTRACT(YEAR FROM CURRENT_DATE) - 1`).

## What was fixed

The tool was permanently returning zero incidents. The dataset spans **2011–2016** only, so querying "last year" relative to today (2026) produced an empty window with no data. The tool appeared to work — it returned a valid JSON structure with no error — but every count was 0.

After the fix, the tool returns **506 total incidents, 2 fatal, 142 injury** for `year_queried: 2015`.

## What to consider

- **The anchor is hardcoded.** `2016-11-22` is the latest `date_of_occurrence` in the incidents table at the time of this fix. If new incident data is ever loaded into the database, this value will need to be updated manually — or replaced with a dynamic `(SELECT MAX(date_of_occurrence) FROM incidents)` subquery.
- **"Last year" means 2015.** The tool name implies recency, but with a frozen 2016 dataset, the most meaningful "last year" is 2015 (a full calendar year of data). 2016 is partial (Jan–Nov), so anchoring to the end of 2016 and querying the prior year is the more complete result.
- **The chatbot LLM will receive `year_queried: 2015`** in the tool response. The OpsBot system prompt should account for this so the LLM doesn't present 2015 data as if it were recent.
