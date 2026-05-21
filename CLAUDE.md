# Rocket Elevators — AND-2

**Purpose:** Operations dashboard and data pipeline analyzing Ontario elevator fleet status using AI-assisted workflows.

## Tech Stack

- **Backend:** Python 3.10+, Flask, Pandas
- **Frontend:** HTMX 2.0.4, Tailwind CSS, Jinja2 — no custom JavaScript
- **Analysis:** Jupyter, scikit-learn, NLTK

## Directory Structure

- `/data` — source datasets (read-only, never modify in-place)
- `/platform` — Flask app, `elevator_fleet.csv`, HTML templates
- `/intelligence` — Jupyter notebooks (ETL, NLP, exploratory analysis)
- `/docs` — `dashboard_spec.md` is the single source of truth for the dashboard

## Commands

```bash
py -3 platform/prepare_data.py   # regenerate elevator_fleet.csv from /data sources
py -3 platform/server.py         # serve dashboard at http://localhost:5000
```

## Conventions

- **Spec-driven:** `platform/index.html` may only reflect content explicitly defined in `docs/dashboard_spec.md`. Update the spec first, then the HTML — never the reverse.
- Flask over FastAPI — HTMX's HTML-fragment pattern fits Flask's template rendering directly.
- HTMX two-channel swap: filter/search → `#tableBody` (innerHTML); sort → `#fleetTable` (outerHTML) to refresh sort-button URLs. No custom JavaScript anywhere.
- `elevator_fleet.csv` is the only dataset the server reads; `prepare_data.py` is the only file that writes it.
- Data pipeline filters to ACTIVE + BY REQUEST licenses only. Join key across all datasets: `ElevatingDevicesNumber`.
- Inspections are one-to-many — always keep only the most recent record per elevator.
- Summary card metrics are computed once at `GET /` page load (initial full-fleet values); when filters or search update the table, cards are recomputed via out-of-band swaps to reflect the filtered dataset.
- `GET /table` returns HTML only. Response shape: `tableBody` → `<tr>` rows + OOB card snippets; `fleetTable` → full `<table>` + OOB card snippets. OOB snippets update the 4 summary card values to reflect the filtered dataset.
