"""
platform/server.py  — Flask backend for the Rocket Elevators HTMX dashboard.

Run guide:
    py -3.12 platform\\server.py
    open http://127.0.0.1:5000/

Endpoints:
    GET /        Renders platform/index.html with summary card metrics
    GET /table   Returns an HTML fragment for HTMX swapping.
                 Query params: status, type, q, sort, order
                 HX-Target: tableBody  -> <tr> rows only  (innerHTML swap)
                 HX-Target: fleetTable -> full <table>    (outerHTML swap, refreshes sort-button URLs)
"""

from flask import Flask, render_template, request, make_response
import pandas as pd
from pathlib import Path
from datetime import date, timedelta

HERE = Path(__file__).resolve().parent
app  = Flask(__name__, template_folder=str(HERE))

# ── Data ──────────────────────────────────────────────────────────────────────
# Load once at startup; NaN -> empty string so templates get simple falsy checks
df_fleet = pd.read_csv(HERE / "elevator_fleet.csv", dtype=str).fillna("")

DATA = HERE.parent / "data"

df_merged = pd.read_csv(
    DATA / "merged_elevator_data.csv",
    dtype=str,
    usecols=["ElevatingDevicesNumber", "LocationoftheElevatingDevice",
             "LICENSESTATUS", "originating service request number"],
).fillna("")

_df_insp_raw = pd.read_csv(
    DATA / "inspection.csv",
    dtype=str,
    usecols=["ElevatingDevicesNumber", "Latest_INSPECTION_Date", "InspectionOutcome"],
).fillna("")

# Normalize inspection dates to ISO format (YYYY-MM-DD) to ensure:
# - consistent display in detail panel
# - reliable sorting in endpoint response
_dates = pd.to_datetime(_df_insp_raw["Latest_INSPECTION_Date"], errors="coerce")
_df_insp_raw["Latest_INSPECTION_Date"] = _dates.dt.strftime("%Y-%m-%d").where(_dates.notna(), "")
df_inspections = _df_insp_raw

TODAY        = date.today()
ONE_YEAR_AGO = TODAY - timedelta(days=365)

# Maps the URL `sort=` param to the CSV column name
SORT_FIELDS = {
    "license_expiry":    "License Expiration Date",
    "latest_inspection": "Latest Inspection Date",
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
    </tr>
  </thead>
  <tbody id="tableBody" class="divide-y divide-slate-100 text-slate-700">
{rows_html}
  </tbody>
</table>"""


# ── Routes ────────────────────────────────────────────────────────────────────

@app.route("/")
def index():
    metrics = compute_metrics(df_fleet)
    return render_template("index.html", **metrics)


@app.route("/table")
def table():
    df = df_fleet.copy()

    # 1. Filter ----------------------------------------------------------------
    status = request.args.get("status", "").strip()
    etype  = request.args.get("type",   "").strip()
    q      = request.args.get("q",      "").strip().lower()

    if status:
        df = df[df["Status"] == status]
    if etype:
        df = df[df["Elevator Type"] == etype]
    if q:
        mask = (
            df["Elevator ID"].str.lower().str.contains(q, na=False) |
            df["Location"].str.lower().str.contains(q, na=False)
        )
        df = df[mask]

    # 2. Sort ------------------------------------------------------------------
    # Parse dates to datetime for correct chronological order; blanks sort last.
    sort  = request.args.get("sort",  "").strip()
    order = request.args.get("order", "asc").strip()

    if sort in SORT_FIELDS:
        col = SORT_FIELDS[sort]
        df["_key"] = pd.to_datetime(df[col], errors="coerce")
        df = df.sort_values("_key", ascending=(order == "asc"), na_position="last")
        df = df.drop(columns=["_key"])

    # 3. Annotate overdue flag -------------------------------------------------
    rows = df.to_dict("records")
    for row in rows:
        row["_overdue"] = is_overdue(row.get("Latest Inspection Date", ""))

    # 4. Render rows via template (reused by both response paths) --------------
    rows_html = render_template("_table_rows.html", rows=rows)

    # 5. Decide response shape based on HTMX target header --------------------
    # Sort buttons target #fleetTable (outerHTML); filters/search target #tableBody (innerHTML).
    if request.headers.get("HX-Target") == "fleetTable":
        return make_response(build_full_table(rows_html, sort, order))

    return make_response(rows_html)


@app.route("/elevator/<elev_id>")
def elevator_detail(elev_id):
    rows = df_merged[df_merged["ElevatingDevicesNumber"] == elev_id]
    if rows.empty:
        return "<h1>404 — Elevator not found</h1>", 404

    first = rows.iloc[0]
    alt_count = (
        rows["originating service request number"]
        .replace("", pd.NA).dropna().nunique()
    )

    insp = (
        df_inspections[df_inspections["ElevatingDevicesNumber"] == elev_id]
        [["Latest_INSPECTION_Date", "InspectionOutcome"]]
        .sort_values(
            by="Latest_INSPECTION_Date",
            ascending=False,
            na_position="last"
        )
        .drop_duplicates()
        .to_dict("records")
    )
    # TODO: integrate incident count from incident dataset
    return render_template(
        "_elevator_detail.html",
        elevator_id=elev_id,
        location=first["LocationoftheElevatingDevice"],
        status=first["LICENSESTATUS"],
        inspections=insp,
        incident_count=0,
        alteration_count=alt_count,
    )


@app.errorhandler(404)
def not_found(e):
    return "<h1>404 — Page not found</h1>", 404


@app.errorhandler(500)
def server_error(e):
    return "<h1>500 — Internal server error</h1>", 500


if __name__ == "__main__":
    app.run(debug=True)
