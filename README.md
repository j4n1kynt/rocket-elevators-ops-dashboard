# Rocket Elevators — Operations Dashboard

An operations dashboard and data pipeline that analyzes the status of the Ontario
elevator fleet using AI-assisted workflows. The platform helps an operations manager
see fleet status at a glance, find elevators with overdue inspections or expiring
licenses, and review the risk level of each device — all in one place.

## Live Deployment

| Service | URL |
|---|---|
| **Dashboard (web app)** | https://rocket-elevators-ops-dashboard-1.onrender.com/ |
| **Go REST API** | https://rocket-elevators-ops-dashboard.onrender.com/api |
| **Database** | PostgreSQL 16, hosted on the [Neon](https://neon.tech) platform |

## Ceremonies and Trello links

- https://drive.google.com/drive/folders/1hz6l02opPJHJ2YfIx9K1xigwcPuuPXey?usp=sharing
- https://trello.com/b/s3DUaLxa/and-106-rocket-elevator-chatbot

> The app is deployed on [Render](https://render.com). The Go API reads all
> operational data from a managed PostgreSQL database on Neon.

### ⚠️ Note on the Render free tier

The app runs on Render's **free tier**, which has one important limit: the service
**spins down after about 15 minutes with no traffic**. When this happens, the next
request has to start the service again ("cold start"), so the **first load can take
30–60 seconds**. After that, the app is fast again until it goes idle.

What this means for you:

- The **first visit** after a quiet period may feel slow or seem to hang. Just wait —
  it is the service waking up, not an error.
- If both the dashboard and the API were idle, **both** need to wake up, so the first
  page may take longer.
- Free instances also have limited CPU and memory, so heavy jobs (like the bulk risk
  explanation generation) should run locally, not on Render.

**How to use the live app:** open **both** URLs in your browser, not just one. First open
the **API URL** to wake up the API, then open the **Dashboard URL**. The dashboard reads
its data from the API, so if the API is still asleep the page will show no data. Once both
are awake, the dashboard works normally.

1. Open the API: https://rocket-elevators-ops-dashboard.onrender.com/api/fleet/stats — wait
   until you see a JSON response.
2. Open the dashboard: https://rocket-elevators-ops-dashboard.onrender.com/ — the data
   should now load.

For a production setup, move to a **paid Render plan** (no spin-down) to remove the cold
start delay.

## Architecture

The project has three main layers:

```
┌──────────────────────┐      ┌──────────────────────┐      ┌──────────────────────┐
│   Dashboard (web)     │      │     Go REST API      │      │   PostgreSQL (Neon)  │
│  Flask + HTMX +       │ ───▶ │  /api/fleet/...       │ ───▶ │  fleet, inspections, │
│  Tailwind (no JS)     │      │  /api/elevators/...   │      │  predictions, etc.   │
└──────────────────────┘      └──────────────────────┘      └──────────────────────┘
          ▲                                                            ▲
          │                                                            │
          └─ summary cards, fleet table, alerts                        │
                                                                       │
                          ┌──────────────────────┐                     │
                          │   Intelligence layer  │ ── ETL / ML ───────┘
                          │  Jupyter + Python      │
                          │  (predictions, NLP)    │
                          └──────────────────────┘
```

- **Dashboard** — a single-page web app built with Flask, HTMX, Tailwind CSS, and
  Jinja2. It uses no custom JavaScript. All fleet data comes from the Go API.
- **Go REST API** — serves aggregate stats, alerts, and elevator profiles. It reads
  all operational data from PostgreSQL.
- **PostgreSQL** — the single source of truth for fleet, inspections, and ML risk
  predictions. Hosted on Neon in production; runs in Docker for local development.
- **Intelligence layer** — Jupyter notebooks and Python scripts that build the data,
  train the risk model, and write AI-generated risk explanations.

## Tech Stack

- **Backend:** Python 3.10+, Flask, Pandas
- **API:** Go 1.25 (standard library `net/http`)
- **Frontend:** HTMX 2.0.4, Tailwind CSS, Jinja2 — no custom JavaScript
- **Analysis:** Jupyter, scikit-learn, NLTK
- **Database:** PostgreSQL 16 (Neon in production, Docker for local dev)
- **Deployment:** Render

## Repository Structure

```
.
├── data/             Source datasets (read-only — never modify in place)
├── platform/         Flask app, HTML templates, and the Go API
│   └── api/          Go REST API (handlers, DB layer, migrations)
├── intelligence/     Jupyter notebooks and Python scripts (ETL, ML, NLP)
├── docs/             Specifications and reports (dashboard_spec.md is the source of truth)
└── docker-compose.yml
```

## Getting Started (Local Development)

### Prerequisites

- Python 3.10+ (3.12 recommended)
- Go 1.25+
- Docker and Docker Compose

### 1. Install Python dependencies

```bash
pip install -r requirements.txt
```

### 2. Start PostgreSQL and the Go API

```bash
docker-compose up
```

This starts PostgreSQL on port `5432` and the Go API on port `8080`.

> The API port is `8080`, not `8081` — port `8081` is blocked by the company firewall.

### 3. Load the data into PostgreSQL

Run these scripts from the project root. They are idempotent, so it is safe to run
them again.

```bash
# Populate PostgreSQL from the source datasets
py -3 intelligence/etl_to_database.py

# Add risk explanations for HIGH-risk elevators (needs the DB running)
py -3 intelligence/generate_explanations.py
```

### 4. Start the dashboard

```bash
py -3 platform/server.py
```

Open the dashboard at **http://localhost:5000**.

## Common Commands

```bash
# Start the full stack (PostgreSQL + Go API)
docker-compose up

# Run only the Go API (needs PostgreSQL running)
go run ./platform/api

# Regenerate elevator_fleet.csv from the /data sources
py -3 platform/prepare_data.py

# Regenerate data/predictions.csv (CLI mirror of generate_predictions.ipynb)
py -3 intelligence/generate_predictions.py

# Populate PostgreSQL from the source datasets (idempotent)
py -3 intelligence/etl_to_database.py

# Add AI risk explanations for HIGH-risk elevators
py -3 intelligence/generate_explanations.py

# Serve the dashboard at http://localhost:5000
py -3 platform/server.py
```

## API Reference

The Go API is defined in `docs/api_spec.md` and documented in `platform/api/README.md`.

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check |
| `GET` | `/api/fleet/stats` | Aggregate fleet stats: total, risk distribution, pass rate, equipment types |
| `GET` | `/api/fleet/alerts` | High-risk elevators with a failed most-recent inspection, sorted by risk score |
| `GET` | `/api/elevators` | Paginated fleet listing with filter, search, and sort |
| `GET` | `/api/elevators/{id}` | Single elevator profile |
| `GET` | `/api/elevators/{id}/inspections` | Inspection history, sorted newest first |
| `GET` | `/api/elevators/{id}/risk` | ML risk assessment for one elevator |
| `POST` | `/api/chat` | AI chat about the fleet (backed by an LLM) |

## Data Pipeline Notes

- The pipeline filters to **ACTIVE** and **BY REQUEST** licenses only.
- The join key across all datasets is `ElevatingDevicesNumber`.
- Inspections are one-to-many — the pipeline keeps only the most recent record per elevator.
- `elevator_fleet.csv` is written by `prepare_data.py` and loaded into PostgreSQL by the ETL.
- `data/predictions.csv` is generated by `generate_predictions.py` and loaded into the
  `predictions` table by the ETL. The Go API reads predictions from the database.
- `risk_explanation` is filled in by `generate_explanations.py` after the ETL runs.
  Rows without a generated explanation have `risk_explanation = NULL`.

## Branching Strategy

```
main ← dev ← feat/<card-id>-<short-description>
```

- **`main`** — stable, production-ready. Receives merges from `dev` only, at the end of
  a sprint, through a pull request.
- **`dev`** — integration branch. All feature PRs target this branch.
- **`feat/...`** — short-lived feature branches (1–2 days max). Always branch from `dev`.
  One task per branch, one branch per PR.

Each PR needs at least 1 reviewer before it merges into `dev`. Merged branches are kept,
not deleted.

## Documentation

The full project documentation lives in `/docs`:

- `dashboard_spec.md` — the single source of truth for the dashboard
- `api_spec.md` — the Go API contract
- `methodology_report.md` and `executive_report.md` — project reports
- `ai-interaction-log.md` — log of AI-assisted work
