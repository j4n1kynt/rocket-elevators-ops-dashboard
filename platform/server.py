"""
platform/server.py  — Flask backend for the Rocket Elevators HTMX dashboard.

Run guide:
    py -3.12 platform\\server.py
    open http://127.0.0.1:5000/

Endpoints:
    GET /        Renders platform/index.html with summary card metrics
    GET /table   Returns an HTML fragment for HTMX swapping.
                 Query params: status, type, q, sort, order, page
                 HX-Target: tableBody  -> <tr> rows only  (innerHTML swap)
                 HX-Target: fleetTable -> full <table>    (outerHTML swap, refreshes sort-button URLs)
    GET /fleet-health   Returns fleet health panel HTML fragment (from Go API /api/fleet/stats)
    GET /alerts         Returns critical alerts HTML fragment (from Go API /api/fleet/alerts)
    GET /elevator/<id>  Returns elevator detail panel HTML fragment
    DELETE /elevator/<id> Clears the detail panel
"""

from flask import Flask, render_template, request, make_response
import pandas as pd
import requests
from pathlib import Path
from datetime import date, timedelta

HERE = Path(__file__).resolve().parent
app  = Flask(__name__, template_folder=str(HERE))

# ── Data ──────────────────────────────────────────────────────────────────────
# Load once at startup; NaN -> empty string so templates get simple falsy checks.
# df_fleet is used only for summary card computation (overdue/expiring metrics)
# since the Go API does not expose those date-arithmetic aggregates.
df_fleet = pd.read_csv(HERE / "elevator_fleet.csv", dtype=str).fillna("")

DATA = HERE.parent / "data"

df_merged = pd.read_csv(
    DATA / "merged_elevator_data.csv",
    dtype=str,
    usecols=["ElevatingDevicesNumber", "LocationoftheElevatingDevice",
             "LICENSESTATUS", "originating service request number"],
).fillna("")

_df_inc = pd.read_json(DATA / "incident.json")
_df_inc["elevating devices number"] = _df_inc["elevating devices number"].astype(str)
df_incidents = _df_inc

TODAY        = date.today()
ONE_YEAR_AGO = TODAY - timedelta(days=365)

GO_API     = "http://localhost:8080"
TABLE_LIMIT = 50

# Maps Flask sort param values to Go API sort param values
GO_SORT = {
    "license_expiry":    "license_expiration_date",
    "latest_inspection": "latest_inspection_date",
}

# ── Helpers ───────────────────────────────────────────────────────────────────

def compute_metrics(df: pd.DataFrame) -> dict:
    """Compute summary card values from the unified fleet DataFrame (§2 spec)."""
    total        = len(df)
    active_count = int((df["Status"] == "ACTIVE").sum())
    active_pct   = round(active_count / total * 100) if total else 0

    insp_dates   = pd.to_datetime(df["Latest Inspection Date"], errors="coerce")
    overdue_mask = df["Latest Inspection Date"].eq("") | (insp_dates.dt.date < ONE_YEAR_AGO)

    expiry_dates  = pd.to_datetime(df["License Expiration Date"], errors="coerce")
    thirty_days   = TODAY + timedelta(days=30)
    expiring_mask = (expiry_dates.dt.date >= TODAY) & (expiry_dates.dt.date <= thirty_days)

    return {
        "total_elevators":     f"{total:,}",
        "active_elevators":    f"{active_count:,} ({active_pct}%)",
        "active_percentage":   active_pct,
        "overdue_inspections": int(overdue_mask.sum()),
        "expiring_soon":       int(expiring_mask.sum()),
    }


def is_overdue(date_str: str) -> bool:
    """True when the inspection date is blank or more than one year old (§4.5)."""
    if not date_str:
        return True
    try:
        return date.fromisoformat(date_str) < ONE_YEAR_AGO
    except ValueError:
        return True


def _sort_btn(field: str, active: str, order: str):
    """Return (next_order_param, direction_icon) for a sortable column button."""
    if active == field:
        return ("desc", "↑") if order == "asc" else ("asc", "↓")
    return ("asc", "↕")


def build_full_table(rows_html: str, active: str, order: str) -> str:
    """
    Wrap the rendered <tr> rows in a complete <table id="fleetTable">.
    Sort button URLs are updated to the NEXT direction so toggling works
    without any client-side JavaScript (Approach A, per dashboard spec §3).
    Risk Level column header added per AND-104 Task 8 spec.
    """
    le_next, le_icon = _sort_btn("license_expiry",    active, order)
    li_next, li_icon = _sort_btn("latest_inspection", active, order)

    TH  = "px-5 py-3 text-xs font-semibold uppercase tracking-wider text-slate-400"
    BTN = ("flex items-center gap-1 text-xs font-semibold uppercase "
           "tracking-wider text-slate-400 hover:text-slate-700 cursor-pointer")

    return f"""\
<table id="fleetTable" class="w-full text-sm text-left">
  <thead class="border-b border-slate-200">
    <tr>
      <th class="{TH} whitespace-nowrap">Elevator ID</th>
      <th class="{TH}">Location</th>
      <th class="{TH} whitespace-nowrap">License Number</th>
      <th class="{TH}">Status</th>
      <th class="{TH} whitespace-nowrap">
        <button hx-get="/table?sort=license_expiry&amp;order={le_next}"
                hx-target="#fleetTable" hx-swap="outerHTML" hx-include="#controls"
                class="{BTN}">
          License Expiration Date <span class="text-base leading-none">{le_icon}</span>
        </button>
      </th>
      <th class="{TH} whitespace-nowrap">
        <button hx-get="/table?sort=latest_inspection&amp;order={li_next}"
                hx-target="#fleetTable" hx-swap="outerHTML" hx-include="#controls"
                class="{BTN}">
          Latest Inspection Date <span class="text-base leading-none">{li_icon}</span>
        </button>
      </th>
      <th class="{TH} whitespace-nowrap">Latest Inspection Outcome</th>
      <th class="{TH} whitespace-nowrap">Elevator Type</th>
      <th class="{TH} whitespace-nowrap">Risk Level</th>
    </tr>
  </thead>
  <tbody id="tableBody" class="divide-y divide-slate-100 text-slate-700">
{rows_html}
  </tbody>
</table>"""


def build_pagination_oob(page: int, total: int, limit: int) -> str:
    """Return an OOB swap snippet for the #pagination div below the table."""
    total_pages = max(1, (total + limit - 1) // limit)
    if total_pages <= 1:
        return '<div id="pagination" hx-swap-oob="outerHTML"></div>'

    BTN = ("px-3 py-1.5 text-xs font-medium rounded border border-slate-200 "
           "hover:border-slate-400 text-slate-600 hover:text-slate-800 cursor-pointer")
    prev_btn = (
        f'<button hx-get="/table?page={page - 1}" hx-target="#tableBody" '
        f'hx-swap="innerHTML" hx-include="#controls" class="{BTN}">&#8592; Prev</button>'
    ) if page > 1 else ""
    next_btn = (
        f'<button hx-get="/table?page={page + 1}" hx-target="#tableBody" '
        f'hx-swap="innerHTML" hx-include="#controls" class="{BTN}">Next &#8594;</button>'
    ) if page < total_pages else ""

    return (
        f'<div id="pagination" hx-swap-oob="outerHTML" '
        f'class="flex items-center justify-between px-5 py-3 border-t border-slate-200 text-sm text-slate-500">'
        f'<span>Page {page} of {total_pages} &mdash; {total:,} results</span>'
        f'<div class="flex gap-2">{prev_btn}{next_btn}</div>'
        f'</div>'
    )


# ── Routes ────────────────────────────────────────────────────────────────────

@app.route("/")
def index():
    metrics = compute_metrics(df_fleet)
    return render_template("index.html", **metrics)


@app.route("/table")
def table():
    # 1. Parse params ----------------------------------------------------------
    status = request.args.get("status", "").strip()
    etype  = request.args.get("type",   "").strip()
    q      = request.args.get("q",      "").strip()
    sort   = request.args.get("sort",   "").strip()
    order  = request.args.get("order",  "asc").strip().lower()
    try:
        page = max(1, int(request.args.get("page", 1) or 1))
    except (ValueError, TypeError):
        page = 1

    # 2. Build Go API params ---------------------------------------------------
    api_params: dict = {"page": page, "limit": TABLE_LIMIT, "order": order}
    if status:
        api_params["status"] = status
    if etype:
        api_params["elevator_type"] = etype   # Go API uses elevator_type, not type
    if q:
        api_params["q"] = q
    if sort in GO_SORT:
        api_params["sort"] = GO_SORT[sort]

    # 3. Fetch from Go API -----------------------------------------------------
    try:
        api_resp = requests.get(f"{GO_API}/api/elevators", params=api_params, timeout=10)
        api_resp.raise_for_status()
        data  = api_resp.json()
        rows  = data.get("results", [])
        total = data.get("total", 0)
    except requests.exceptions.RequestException:
        rows  = []
        total = 0

    # 4. Annotate rows with overdue flag (Flask-computed) ----------------------
    for row in rows:
        row["_overdue"] = is_overdue(row.get("latest_inspection_date") or "")

    # 5. Render rows -----------------------------------------------------------
    rows_html = render_template("_table_rows.html", rows=rows)

    # 6. Compute card metrics from CSV (overdue/expiring not in Go API) --------
    df_sub = df_fleet.copy()
    if status:
        df_sub = df_sub[df_sub["Status"] == status]
    if etype:
        df_sub = df_sub[df_sub["Elevator Type"] == etype]
    if q:
        ql   = q.lower()
        mask = (
            df_sub["Elevator ID"].str.lower().str.contains(ql, na=False) |
            df_sub["Location"].str.lower().str.contains(ql, na=False)
        )
        df_sub = df_sub[mask]
    m = compute_metrics(df_sub)

    cards_oob = (
        f'\n<p id="card-total"    hx-swap-oob="innerHTML">{total:,}</p>'
        f'\n<p id="card-active"   hx-swap-oob="innerHTML">{m["active_elevators"]}</p>'
        f'\n<p id="card-overdue"  hx-swap-oob="innerHTML">{m["overdue_inspections"]}</p>'
        f'\n<p id="card-expiring" hx-swap-oob="innerHTML">{m["expiring_soon"]}</p>'
    )

    # 7. Build pagination OOB -------------------------------------------------
    pagination_oob = build_pagination_oob(page, total, TABLE_LIMIT)

    # 8. Decide response shape based on HTMX target header --------------------
    if request.headers.get("HX-Target") == "fleetTable":
        return make_response(build_full_table(rows_html, sort, order) + cards_oob + pagination_oob)

    return make_response(rows_html + cards_oob + pagination_oob)


@app.route("/fleet-health")
def fleet_health():
    """Return fleet health panel HTML from Go API /api/fleet/stats."""
    try:
        resp = requests.get(f"{GO_API}/api/fleet/stats", timeout=5)
        resp.raise_for_status()
        stats = resp.json()
    except Exception:
        return "<p class='text-sm text-slate-400 p-4'>Fleet health data unavailable.</p>"

    rd    = stats["risk_distribution"]
    total = stats["total_elevators"]

    def pct(n: int) -> int:
        return round(n / total * 100) if total else 0

    return render_template(
        "_fleet_health.html",
        total=total,
        low=rd["low"],       low_pct=pct(rd["low"]),
        medium=rd["medium"], medium_pct=pct(rd["medium"]),
        high=rd["high"],     high_pct=pct(rd["high"]),
        unknown=rd["unknown"], unknown_pct=pct(rd["unknown"]),
        pass_rate=round(stats["inspection_pass_rate"] * 100, 1),
    )


@app.route("/alerts")
def alerts_panel():
    """Return critical alerts section HTML from Go API /api/fleet/alerts."""
    try:
        resp = requests.get(f"{GO_API}/api/fleet/alerts", timeout=10)
        resp.raise_for_status()
        data  = resp.json()
        shown = data["alerts"][:20]
        total = data["total"]
    except Exception:
        return "<p class='text-sm text-slate-400 p-4'>Alerts unavailable.</p>"

    return render_template("_alerts.html", alerts=shown, total=total, shown_count=len(shown))


@app.route("/elevator/<elev_id>", methods=["GET", "DELETE"])
def elevator_detail(elev_id):
    if request.method == "DELETE":
        return make_response("")

    # Fetch elevator core data from Go API
    try:
        api_resp = requests.get(f"{GO_API}/api/elevators/{elev_id}", timeout=5)
        if api_resp.status_code == 404:
            return "<p class='p-5 text-sm text-slate-500'>Elevator not found.</p>", 404
        api_resp.raise_for_status()
        elev = api_resp.json()
    except requests.exceptions.RequestException:
        return "<p class='p-5 text-sm text-red-600'>Unable to fetch elevator data from API.</p>", 503

    # Fetch inspection history from Go API
    try:
        insp_resp = requests.get(
            f"{GO_API}/api/elevators/{elev_id}/inspections",
            params={"limit": 200},
            timeout=5,
        )
        insp_resp.raise_for_status()
        raw = insp_resp.json().get("inspections", [])
    except requests.exceptions.RequestException:
        raw = []

    # Map Go API field names to keys the template expects
    inspections = [
        {"Latest_INSPECTION_Date": r["inspection_date"], "InspectionOutcome": r["outcome"]}
        for r in raw
    ]

    # Fetch risk assessment from Go API
    risk_data    = None
    risk_message = ""
    try:
        risk_resp = requests.get(f"{GO_API}/api/elevators/{elev_id}/risk", timeout=3)
        if risk_resp.status_code == 200:
            risk_data = risk_resp.json()
        elif risk_resp.status_code == 503:
            risk_message = "Risk pipeline not yet deployed."
        elif risk_resp.status_code == 404:
            risk_message = "No prediction available for this elevator."
    except requests.exceptions.RequestException:
        risk_message = "Risk data unavailable."

    # incident_count and alteration_count are not exposed by the Go API;
    # keep CSV-based lookups for these supplementary counts only.
    rows = df_merged[df_merged["ElevatingDevicesNumber"] == elev_id]
    alt_count = int(
        rows["originating service request number"]
        .replace("", pd.NA).dropna().nunique()
    )
    inc_count = int(
        df_incidents[df_incidents["elevating devices number"] == elev_id]
        ["Incident Number"].nunique()
    )

    return render_template(
        "_elevator_detail.html",
        elevator_id=elev_id,
        location=elev.get("location", ""),
        status=elev.get("status", ""),
        inspections=inspections,
        incident_count=inc_count,
        alteration_count=alt_count,
        risk_data=risk_data,
        risk_message=risk_message,
        is_overdue=is_overdue,
    )


@app.errorhandler(404)
def not_found(e):
    return "<h1>404 — Page not found</h1>", 404


@app.errorhandler(500)
def server_error(e):
    return "<h1>500 — Internal server error</h1>", 500


if __name__ == "__main__":
    app.run(debug=True)
