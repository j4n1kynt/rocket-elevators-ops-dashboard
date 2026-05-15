# Rocket Elevators — AND-2

**Purpose:** Operations dashboard and data pipeline analyzing Ontario elevator fleet status using AI-assisted workflows.

## Tech Stack

- **Backend:** Python 3.10+, Flask, Pandas
- **Frontend:** HTMX 2.0.4, Tailwind CSS, Jinja2 — no custom JavaScript
- **Analysis:** Jupyter, scikit-learn, NLTK

## Directory Structure

- `/data` — source datasets (read-only, never modify in-place)
- `/platform` — Flask app, prepared dataset (`elevator_fleet.csv`), HTML templates
- `/intelligence` — Jupyter notebooks (ETL, NLP, exploratory analysis)
- `/docs` — technical spec, executive report, AI interaction logs

## Commands

```bash
python platform/prepare_data.py   # regenerate elevator_fleet.csv from /data sources
python platform/server.py         # serve dashboard at http://localhost:5000
```

## Conventions

- Flask over FastAPI — HTMX's HTML-fragment pattern fits Flask's template rendering directly.
- HTMX endpoints return HTML fragments, not JSON. Filter/search swaps `#tableBody` (innerHTML); sort swaps `#fleetTable` (outerHTML) to refresh button URLs.
- `elevator_fleet.csv` is the only dataset the server reads; `prepare_data.py` is the only file that writes it.
- Inspections are one-to-many per elevator — always keep only the most recent record.
- Summary card metrics are computed once at `GET /` page load; they do not respond to HTMX filter or sort changes.