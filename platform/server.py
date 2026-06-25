"""
platform/server.py  — Flask backend for the Rocket Elevators HTMX dashboard.

Run guide:
    py -3.12 platform\\server.py
    open http://127.0.0.1:5000/

Pages (spec §6) — full layout.html shell on a direct hit / refresh,
content-only partial on an HX-Request (sidebar + top bar are not reloaded):
    GET /        Overview — summary cards + fleet health
    GET /fleet   Elevator Fleet — controls, paginated table, detail panel
    GET /alerts  Critical Alerts — alerts table

Fragments (HTML for HTMX swaps):
    GET /table   Query params: status, type, q, sort, order, page
                 HX-Target: tableBody  -> <tr> rows only  (innerHTML swap)
                 HX-Target: fleetTable -> full <table>    (outerHTML swap, refreshes sort-button URLs)
    GET /fleet-health      Fleet health panel (from Go API /api/fleet/stats)
    GET /elevator/<id>     Elevator detail panel
    DELETE /elevator/<id>  Clears the detail panel
"""

import json
import os
import re

from flask import Flask, render_template, request, make_response
import pandas as pd
import requests
from pathlib import Path
from datetime import date, datetime, timedelta, timezone

HERE = Path(__file__).resolve().parent
app  = Flask(__name__, template_folder=str(HERE))

# ── Data ──────────────────────────────────────────────────────────────────────
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

GO_API     = os.environ.get("GO_API", "http://localhost:8080")
TABLE_LIMIT  = 50
ALERTS_LIMIT = 50

# Maps Flask sort param values to Go API sort param values
GO_SORT = {
    "license_expiry":    "license_expiration_date",
    "latest_inspection": "latest_inspection_date",
}

# ── Helpers ───────────────────────────────────────────────────────────────────


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
<table id="fleetTable" class="w-full min-w-[960px] text-sm text-left">
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


def build_pagination_oob(page: int, total: int, limit: int, *,
                         pag_id: str = "pagination", body_id: str = "tableBody",
                         controls_id: str = "controls", url: str = "/table") -> str:
    """Return an OOB swap snippet for a pagination bar below a table.

    Defaults target the fleet table; pass pag_id / body_id / controls_id / url to
    reuse it for the alerts table.
    """
    total_pages = max(1, (total + limit - 1) // limit)
    if total_pages <= 1:
        return f'<div id="{pag_id}" hx-swap-oob="outerHTML"></div>'

    BTN = ("px-3 py-1.5 text-xs font-medium rounded border border-slate-200 "
           "hover:border-slate-400 text-slate-600 hover:text-slate-800 cursor-pointer")
    prev_btn = (
        f'<button hx-get="{url}?page={page - 1}" hx-target="#{body_id}" '
        f'hx-swap="innerHTML" hx-include="#{controls_id}" class="{BTN}">&#8592; Prev</button>'
    ) if page > 1 else ""
    next_btn = (
        f'<button hx-get="{url}?page={page + 1}" hx-target="#{body_id}" '
        f'hx-swap="innerHTML" hx-include="#{controls_id}" class="{BTN}">Next &#8594;</button>'
    ) if page < total_pages else ""

    return (
        f'<div id="{pag_id}" hx-swap-oob="outerHTML" '
        f'class="flex items-center justify-between px-5 py-3 border-t border-slate-200 text-sm text-slate-500">'
        f'<span>Page {page} of {total_pages} &mdash; {total:,} results</span>'
        f'<div class="flex gap-2">{prev_btn}{next_btn}</div>'
        f'</div>'
    )


# ── Page shell helper (spec §6.4) ───────────────────────────────────────────

# Top-bar title + subtitle for each navigable page (spec §6.2)
PAGES = {
    "overview":       ("Operational Fleet Overview",
                       "Active and by-request licensed devices"),
    "fleet":          ("Elevator Fleet",
                       "Search, filter, and inspect individual devices"),
    "alerts":         ("Critical Alerts",
                       "HIGH risk devices with a failed most-recent inspection"),
    "conversations":  ("Conversations",
                       "Chatbot usage and history"),
}


def render_page(active: str, page_template: str, **ctx):
    """
    Render a navigable page (spec §6.4).

    HX-Request -> content partial + OOB top bar + OOB nav, so the title and the
                  active sidebar link update while #main-content swaps. The shell
                  itself is never reloaded.
    Direct/F5  -> full layout.html shell with the page embedded.
    """
    title, subtitle = PAGES[active]
    base = dict(active_page=active, page_title=title, page_subtitle=subtitle, **ctx)

    if request.headers.get("HX-Request"):
        parts = [
            render_template(page_template, **base),
            render_template("_page_header.html", oob=True, **base),
            render_template("_nav.html", oob=True, **base),
        ]
        return make_response("".join(parts))

    return render_template("layout.html", page_template=page_template, **base)


def fetch_alerts(q: str = "", outcome: str = "", page: int = 1, limit: int = 50):
    """Fetch a page of critical alerts from the Go API. Returns (alerts, total) on
    success, or (None, 0) when the API is unreachable. `total` is the full filtered
    count across all pages (for pagination), not just the current page length."""
    params: dict = {"page": page, "limit": limit}
    if q:
        params["q"] = q
    if outcome:
        params["outcome"] = outcome
    try:
        resp = requests.get(f"{GO_API}/api/fleet/alerts", params=params, timeout=10)
        resp.raise_for_status()
        data = resp.json()
        return data.get("alerts", []), data.get("total", 0)
    except Exception:
        return None, 0


# ── Routes ────────────────────────────────────────────────────────────────────

@app.route("/")
def index():
    total, pass_rate = 0, 0.0
    try:
        r = requests.get(f"{GO_API}/api/fleet/stats", timeout=5)
        r.raise_for_status()
        s         = r.json()
        total     = s["total_elevators"]
        pass_rate = s["inspection_pass_rate"]
    except Exception:
        pass

    active_count = 0
    try:
        r2 = requests.get(
            f"{GO_API}/api/elevators",
            params={"status": "ACTIVE", "limit": 1},
            timeout=5,
        )
        r2.raise_for_status()
        active_count = r2.json().get("total", 0)
    except Exception:
        pass

    active_pct = round(active_count / total * 100) if total else 0

    return render_page(
        "overview", "_page_overview.html",
        total_elevators=f"{total:,}",
        active_elevators=f"{active_count:,} ({active_pct}%)",
        overdue_inspections=f"{round((1 - pass_rate) * total):,}",
        expiring_soon=0,
    )


@app.route("/fleet")
def fleet():
    """Elevator Fleet page (spec §6.2). Honors an optional ?status= filter so the
    Overview 'Active' drill-down lands pre-filtered (spec §6.7): the Status dropdown
    pre-selects, and the initial hx-trigger='load' -> /table includes that value."""
    return render_page(
        "fleet", "_page_fleet.html",
        sel_status=request.args.get("status", "").strip(),
    )


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

    # 6. Build pagination OOB — carries the filtered result count (spec §6.5).
    #    The summary cards live on the Overview page now, so /table no longer
    #    sends a #card-total OOB swap.
    pagination_oob = build_pagination_oob(page, total, TABLE_LIMIT)

    # 7. Decide response shape based on HTMX target header --------------------
    if request.headers.get("HX-Target") == "fleetTable":
        return make_response(build_full_table(rows_html, sort, order) + pagination_oob)

    return make_response(rows_html + pagination_oob)


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

    # Conic-gradient stops for the risk donut (Feature 3, no JS). Cumulative
    # boundaries are built from exact counts so the segments always total 100%.
    segments = [
        ("#16a34a", rd["low"]),      # green — LOW
        ("#d97706", rd["medium"]),   # amber — MEDIUM
        ("#dc2626", rd["high"]),     # red   — HIGH
        ("#94a3b8", rd["unknown"]),  # slate — no prediction
    ]
    if total:
        stops, acc = [], 0
        for color, count in segments:
            start = acc / total * 100
            acc += count
            stops.append(f"{color} {start:.3f}% {acc / total * 100:.3f}%")
        donut_gradient = ", ".join(stops)
    else:
        donut_gradient = "#94a3b8 0% 100%"

    return render_template(
        "_fleet_health.html",
        total=total,
        low=rd["low"],       low_pct=pct(rd["low"]),
        medium=rd["medium"], medium_pct=pct(rd["medium"]),
        high=rd["high"],     high_pct=pct(rd["high"]),
        unknown=rd["unknown"], unknown_pct=pct(rd["unknown"]),
        pass_rate=round(stats["inspection_pass_rate"] * 100, 1),
        donut_gradient=donut_gradient,
    )


@app.route("/alerts")
def alerts_page():
    """Critical Alerts page (spec §6.2). The table, result count, and pagination
    load via /alerts-table; search and outcome filter are server-driven."""
    return render_page("alerts", "_page_alerts.html")


@app.route("/alerts-table")
def alerts_table():
    """Alerts table fragment (spec AND-104 Feature 4): <tr> rows for #alertsBody,
    plus OOB pagination and result-count, driven by q / outcome / page params."""
    q       = request.args.get("q", "").strip()
    outcome = request.args.get("outcome", "").strip()
    try:
        page = max(1, int(request.args.get("page", 1) or 1))
    except (ValueError, TypeError):
        page = 1

    alerts, total = fetch_alerts(q=q, outcome=outcome, page=page, limit=ALERTS_LIMIT)
    if alerts is None:
        return make_response(
            '<tr><td colspan="5" class="px-5 py-8 text-center text-sm text-slate-400">'
            'Alerts unavailable.</td></tr>'
        )

    rows_html      = render_template("_alerts_rows.html", alerts=alerts)
    pagination_oob = build_pagination_oob(
        page, total, ALERTS_LIMIT,
        pag_id="alerts-pagination", body_id="alertsBody",
        controls_id="alerts-controls", url="/alerts-table",
    )
    count_oob = (
        f'<span id="alerts-count" hx-swap-oob="innerHTML">'
        f'Showing {len(alerts):,} of {total:,}</span>'
    )
    return make_response(rows_html + pagination_oob + count_oob)


@app.route("/alerts-preview")
def alerts_preview():
    """Compact top-5 critical alerts for the Overview page (spec §6.7)."""
    alerts, total = fetch_alerts(limit=5)
    return render_template(
        "_alerts_preview.html",
        alerts_ok=alerts is not None,
        alerts=alerts or [],
        total=total,
    )


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


import re as _re
import mistune as _mistune

# Markdown renderer shared across requests.
# escape=True: raw HTML in LLM output is neutralised before it reaches the browser.
# hard_wrap=True: single \n becomes <br />, so Go-formatted plain-text data blocks
# (which use \n between field lines) keep their line structure after conversion.
_md = _mistune.create_markdown(
    escape=True,
    hard_wrap=True,
)

_RISK_BADGES = {
    "HIGH":   '<span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-semibold bg-red-100 text-red-700">HIGH</span>',
    "MEDIUM": '<span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-semibold bg-amber-100 text-amber-700">MEDIUM</span>',
    "LOW":    '<span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-semibold bg-green-100 text-green-700">LOW</span>',
}

def _render_reply(text: str) -> str:
    """Render an assistant reply as safe HTML.

    Steps:
    1. Convert markdown to HTML (handles **bold**, - bullets, ## headers).
       escape=True prevents raw HTML injection; hard_wrap=True turns single
       newlines into <br /> so Go-formatted data blocks keep their line layout.
    2. Bold short field labels at line or paragraph start (e.g. "Risk level:").
    3. Swap risk-level words for coloured badges.
    """
    from markupsafe import Markup
    safe = _md(text)
    # Bold labels at the start of a physical line (after <br />\n) ...
    safe = _re.sub(r'(?m)^([A-Za-z][A-Za-z ()/\-]{0,38}):', r'<strong>\1:</strong>', safe)
    # ... and at the start of a paragraph (immediately after <p>).
    safe = _re.sub(r'(<p>)([A-Za-z][A-Za-z ()/\-]{0,38}):', r'\1<strong>\2:</strong>', safe)
    for level, badge in _RISK_BADGES.items():
        safe = _re.sub(r'\b' + level + r'\b', badge, safe, flags=_re.IGNORECASE)
    return Markup(safe)


@app.route("/chat", methods=["POST"])
def chat():
    message = (request.form.get("message") or "").strip()
    if not message:
        return make_response("")

    history_raw = request.form.get("history", "[]")
    try:
        history = json.loads(history_raw)
        if not isinstance(history, list):
            history = []
    except Exception:
        history = []

    pending_action_raw = request.form.get("pending_action", "null")
    try:
        pending_action = json.loads(pending_action_raw)
        if not isinstance(pending_action, dict):
            pending_action = None
    except Exception:
        pending_action = None

    try:
        conversation_id = int(request.form.get("conversation_id", "0"))
    except (TypeError, ValueError):
        conversation_id = 0

    api_payload = {"message": message, "history": history}
    api_payload["conversation_id"] = conversation_id
    if pending_action is not None:
        api_payload["pending_action"] = pending_action

    try:
        api_resp = requests.post(
            f"{GO_API}/api/chat",
            json=api_payload,
            timeout=330,  # the LLM can be slow on free models
        )
        if api_resp.status_code in (500, 503):
            try:
                detail = api_resp.json().get("error", "")
            except Exception:
                detail = ""
            if api_resp.status_code == 503:
                msg_text = detail or "The assistant is currently unavailable. Please try again in a moment."
            else:
                msg_text = "The assistant encountered an internal error. Please try again."
            return render_template(
                "_chat_reply.html",
                message=message,
                reply_html=None,
                error=msg_text,
                history=json.dumps(history),
                pending_action="null",
                conversation_id=conversation_id,
            )
        api_resp.raise_for_status()
        data = api_resp.json()
    except requests.exceptions.Timeout:
        return render_template(
            "_chat_reply.html",
            message=message,
            reply_html=None,
            error="The assistant took too long to respond. Please try again.",
            history=json.dumps(history),
            pending_action="null",
            conversation_id=conversation_id,
        )
    except Exception:
        return render_template(
            "_chat_reply.html",
            message=message,
            reply_html=None,
            error="Failed to reach the assistant. Please try again.",
            history=json.dumps(history),
            pending_action="null",
            conversation_id=conversation_id,
        )

    return render_template(
        "_chat_reply.html",
        message=message,
        reply_html=_render_reply(data.get("reply", "")),
        error=None,
        history=json.dumps(data.get("history", [])),
        pending_action=json.dumps(data.get("pending_action")),
        conversation_id=data.get("conversation_id", conversation_id),
    )


@app.route("/chat/clear")
def chat_clear():
    return render_template("_chat_clear.html")


# ── Conversations helpers ─────────────────────────────────────────────────────

def _relative_time(iso_str: str) -> tuple[str, str]:
    """Return (relative_label, exact_YYYY-MM-DD HH:MM) from an RFC3339 string.

    relative_label examples: "just now", "5 min ago", "2 hours ago", "3 days ago".
    Falls back to the date string when parsing fails.
    """
    exact = iso_str  # fallback
    try:
        # Python 3.10 fromisoformat does not handle trailing Z — replace it
        dt = datetime.fromisoformat(iso_str.replace("Z", "+00:00"))
        exact = dt.strftime("%Y-%m-%d %H:%M")
        now = datetime.now(timezone.utc)
        diff = now - dt
        total_seconds = int(diff.total_seconds())
        if total_seconds < 60:
            return "just now", exact
        if total_seconds < 3600:
            mins = total_seconds // 60
            return f"{mins} min ago", exact
        if total_seconds < 86400:
            hours = total_seconds // 3600
            return f"{hours} hour{'s' if hours != 1 else ''} ago", exact
        days = total_seconds // 86400
        if days < 30:
            return f"{days} day{'s' if days != 1 else ''} ago", exact
        return exact, exact  # old — show date as label too
    except Exception:
        return iso_str, iso_str


# ── Conversations routes ──────────────────────────────────────────────────────

@app.route("/conversations")
def conversations_page():
    """Conversations page (spec §8.1). Passes dynamic agents list for the filter dropdown."""
    _FALLBACK_AGENTS = ["data", "general", "knowledge", "scheduling"]
    try:
        resp = requests.get(f"{GO_API}/api/conversations/stats", timeout=10)
        resp.raise_for_status()
        dist = resp.json().get("agent_distribution", {})
        agents = sorted(dist.keys()) if dist else _FALLBACK_AGENTS
    except Exception:
        agents = _FALLBACK_AGENTS
    return render_page("conversations", "_page_conversations.html", agents=agents)


@app.route("/conversations/list")
def conversations_list():
    """Sidebar list fragment — fetches GET /api/conversations, renders the list."""
    q     = request.args.get("q",     "").strip()
    agent = request.args.get("agent", "").strip()
    try:
        page = max(1, int(request.args.get("page", 1) or 1))
    except (ValueError, TypeError):
        page = 1

    params: dict = {"page": page, "limit": 20}
    if q:
        params["q"] = q
    if agent:
        params["agent"] = agent

    try:
        resp = requests.get(f"{GO_API}/api/conversations", params=params, timeout=10)
        resp.raise_for_status()
        data  = resp.json()
        convs = data.get("conversations", [])
        total = data.get("total", 0)
    except Exception:
        return make_response(
            '<p class="text-xs text-slate-400 px-3 py-4">Conversations unavailable.</p>'
        )

    # Annotate each conversation with relative + exact time
    for c in convs:
        rel, exact = _relative_time(c.get("last_activity_at", ""))
        c["_rel_time"]   = rel
        c["_exact_time"] = exact

    if not convs:
        return make_response(
            '<p class="text-xs text-slate-400 px-3 py-4">No conversations yet.</p>'
        )

    return render_template("_conversations_list.html", conversations=convs, total=total)


@app.route("/conversations/stats")
def conversations_stats():
    """Global analytics fragment for the main area landing state."""
    try:
        resp = requests.get(f"{GO_API}/api/conversations/stats", timeout=10)
        resp.raise_for_status()
        stats = resp.json()
    except Exception:
        return make_response(
            '<p class="text-sm text-slate-400 p-6">Statistics unavailable.</p>'
        )

    if stats.get("total_conversations", 0) == 0:
        return make_response(
            '<p class="text-sm text-slate-400 p-6">No conversations yet.</p>'
        )

    # Compute agent percentages (out of total assistant messages).
    # Show every agent present in the data: the 4 canonical agents first (always,
    # even at 0), then any other values (e.g. legacy "advisory", "mcp_data_tool")
    # sorted by name. This matches the "show all present" decision.
    dist  = stats.get("agent_distribution", {})
    total_msgs = sum(dist.values()) or 1
    canonical = ["data", "knowledge", "scheduling", "general"]
    extra = sorted(k for k in dist.keys() if k not in canonical)
    agent_rows = []
    for name in canonical + extra:
        count = dist.get(name, 0)
        pct   = round(count / total_msgs * 100)
        agent_rows.append({"name": name, "count": count, "pct": pct})

    # Compute bar heights for activity chart (max day = 100%)
    activity = stats.get("activity_by_day", [])
    max_day  = max((d["conversations"] for d in activity), default=1) or 1
    for d in activity:
        d["_bar_pct"] = round(d["conversations"] / max_day * 100)

    return render_template(
        "_conversation_stats.html",
        stats=stats,
        agent_rows=agent_rows,
        activity=activity,
    )


@app.route("/conversations/new")
def conversations_new():
    """Empty thread for starting a new conversation."""
    return render_template(
        "_conversation_thread.html",
        conversation_id=0,
        messages=[],
        history="[]",
        agents=[],
        message_count=0,
        started_rel="",
        started_exact="",
        last_rel="",
        last_exact="",
        title="Start a new conversation",
        is_new=True,
    )


@app.route("/conversations/<int:cid>")
def conversation_thread(cid):
    """Full thread for a single conversation."""
    try:
        resp = requests.get(f"{GO_API}/api/conversations/{cid}", timeout=10)
        if resp.status_code == 404:
            return make_response(
                '<p class="text-sm text-slate-400 p-6">Conversation not found.</p>'
            )
        resp.raise_for_status()
        data = resp.json()
    except Exception:
        return make_response(
            '<p class="text-sm text-slate-400 p-6">Conversation unavailable.</p>'
        )

    messages = data.get("messages", [])

    # Render assistant content through _render_reply; user content plain-escaped
    from markupsafe import escape, Markup
    for m in messages:
        if m["role"] == "assistant":
            m["_html"] = _render_reply(m.get("content", ""))
        else:
            raw = str(escape(m.get("content", "")))
            m["_html"] = Markup(raw)

    # Build history JSON for the resume form
    history = json.dumps([
        {"role": m["role"], "content": m["content"]}
        for m in messages
    ])

    started_rel, started_exact = _relative_time(data.get("started_at", ""))
    last_rel,    last_exact    = _relative_time(data.get("last_activity_at", ""))

    return render_template(
        "_conversation_thread.html",
        conversation_id=cid,
        messages=messages,
        history=history,
        agents=data.get("agents", []),
        message_count=data.get("message_count", 0),
        started_rel=started_rel,
        started_exact=started_exact,
        last_rel=last_rel,
        last_exact=last_exact,
        title=data.get("title", ""),
        is_new=False,
    )


@app.route("/conversations/<int:cid>/message", methods=["POST"])
def conversation_message(cid):
    """Resume a conversation — the thread-page equivalent of POST /chat."""
    message = (request.form.get("message") or "").strip()
    if not message:
        return make_response("")

    history_raw = request.form.get("history", "[]")
    try:
        history = json.loads(history_raw)
        if not isinstance(history, list):
            history = []
    except Exception:
        history = []

    try:
        conversation_id = int(request.form.get("conversation_id", str(cid)))
    except (TypeError, ValueError):
        conversation_id = cid

    api_payload = {
        "message": message,
        "history": history,
        "conversation_id": conversation_id,
    }

    try:
        api_resp = requests.post(
            f"{GO_API}/api/chat",
            json=api_payload,
            timeout=330,
        )
        if api_resp.status_code in (500, 503):
            try:
                detail = api_resp.json().get("error", "")
            except Exception:
                detail = ""
            if api_resp.status_code == 503:
                msg_text = detail or "The assistant is currently unavailable. Please try again."
            else:
                msg_text = "The assistant encountered an internal error. Please try again."
            return render_template(
                "_conversation_reply.html",
                message=message,
                reply_html=None,
                error=msg_text,
                history=json.dumps(history),
                conversation_id=conversation_id,
            )
        api_resp.raise_for_status()
        data = api_resp.json()
    except requests.exceptions.Timeout:
        return render_template(
            "_conversation_reply.html",
            message=message,
            reply_html=None,
            error="The assistant took too long to respond. Please try again.",
            history=json.dumps(history),
            conversation_id=conversation_id,
        )
    except Exception:
        return render_template(
            "_conversation_reply.html",
            message=message,
            reply_html=None,
            error="Failed to reach the assistant. Please try again.",
            history=json.dumps(history),
            conversation_id=conversation_id,
        )

    return render_template(
        "_conversation_reply.html",
        message=message,
        reply_html=_render_reply(data.get("reply", "")),
        error=None,
        history=json.dumps(data.get("history", [])),
        conversation_id=data.get("conversation_id", conversation_id),
    )


@app.errorhandler(404)
def not_found(e):
    return "<h1>404 — Page not found</h1>", 404


@app.errorhandler(500)
def server_error(e):
    return "<h1>500 — Internal server error</h1>", 500


if __name__ == "__main__":
    # threaded=True so a long-running chat request doesn't block the dashboard's
    # other HTMX calls (table, fleet-health, alerts).
    app.run(debug=True, threaded=True)
