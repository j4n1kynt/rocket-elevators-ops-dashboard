# OpsBot — Additional Supported Queries

**FEATURE-1 | Updated:** 2026-06-18

Four additional chat queries supported beyond the five core PBI scenarios. All retrieve live data from PostgreSQL via the MCP server.

---

## 1. Incident history for a specific elevator

**Example prompts:**
- "What incidents were reported for elevator 10234?"
- "Show me the incident history for elevator 55907"
- "Has elevator 88712 had any incidents?"

**MCP tool:** `get_elevator_incidents`

**Why an operations team member needs this:**
Before sending an inspector to a site, the team needs to know whether that elevator has a history of entrapments, mechanical failures, or injuries — not just whether it failed its last inspection. A building owner calling in a complaint about a specific device may describe symptoms that match a prior incident. This query surfaces that history so the team can arrive prepared, triage correctly, and identify repeat-offender devices.

**What it returns:**
Up to 20 incidents for the elevator, newest first: incident ID, date of occurrence, category, summary, root cause, injury severity, and whether the incident involved a fatality.

---

## 2. Risk score and explanation for a specific elevator

**Example prompts:**
- "What is the risk level of elevator 10234?"
- "Why is elevator 55907 rated HIGH risk?"
- "Show me the risk assessment for elevator 88712"

**MCP tool:** `get_elevator_risk`

**Why an operations team member needs this:**
The dashboard shows a risk level badge, but not the reasoning behind it. When a field inspector, building manager, or compliance officer asks *why* a device is HIGH risk, the operations team needs a plain-language answer — not just a score. This query returns the ML risk score plus the AI-generated explanation produced by `generate_explanations.py`, which describes the specific factors (number of failed inspections, incident history, time since last inspection) that drove the classification. Only the top 5,000 highest-risk elevators have explanations; devices outside that set return a prediction without explanation.

> **Tip:** Include the phrase "risk level" in your prompt. The classifier requires it plus an elevator ID to route to this tool.

---

## 3. Annual incident count

**Example prompts:**
- "How many incidents were reported last year?"
- "What is the incident count for last year?"
- "Give me the annual incident summary"

**MCP tool:** `get_incident_count_last_year`

**Why an operations team member needs this:**
Regulatory reporting and internal safety reviews require year-over-year incident totals. An operations director presenting to the board or filing with the TSSA needs the total incident count, broken down by fatal incidents and injury-involving incidents, without pulling a manual report from the database. This query returns all three figures for the previous calendar year in a single response, ready to be cited directly.

**What it returns:**
Total incidents, fatal incidents, and injury incidents for the previous calendar year, plus the year being reported on.

---

## 4. Fleet-wide risk and health summary

**Example prompts:**
- "How many elevators are at each risk level?"
- "Give me a fleet risk summary"
- "What is the overall inspection pass rate?"

**MCP tool:** `get_fleet_stats`

**Why an operations team member needs this:**
An operations manager or director starting their day wants a single-sentence health check on the fleet — how many devices are HIGH risk, what fraction passed their last inspection, and how the equipment types break down — without navigating to the dashboard. This is also useful for preparing a verbal briefing or a quick status update for an executive who is not logged into the system.

**What it returns:**
Total active elevators, count of devices at each risk level (HIGH / MEDIUM / LOW / unscored), overall inspection pass rate as a percentage, and the count of each elevator type in the fleet.

> **Tip:** The classifier requires a keyword like "risk level", "risk summary", or "pass rate" plus a fleet-scope phrase (no elevator ID) to route to this tool. Generic prompts like "how is the fleet doing?" fall back to advisory.
