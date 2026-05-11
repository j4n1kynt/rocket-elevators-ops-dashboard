# Rocket Elevators — AND-2

Purpose: Build an operations dashboard and data pipeline to analyze Rocket Elevators datasets using AI-assisted workflows.

Tech stack: Python 3.10+ (pandas, Jupyter), HTML, Tailwind CSS, HTMX, Python web server (Flask/FastAPI).

Repo structure:
- /data: source datasets (read-only)
- /docs: specifications and reflections
- /intelligence: Jupyter notebooks (analysis + experiments)
- /platform: dashboard UI and server-facing assets

Data files (in /data):
- license.csv (CSV)
- inspection.csv (CSV)
- order.csv (CSV)
- installed.json (JSON)
- altered.json (JSON)
- incident.json (JSON)

Conventions:
- Never modify datasets in /data in-place; create derived outputs elsewhere.
- Use Python type hints for new Python code.