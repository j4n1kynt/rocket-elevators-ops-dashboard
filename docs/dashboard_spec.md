# Rocket Elevators Operations Dashboard - Technical Specification

## Overview

The Rocket Elevators Operations Dashboard provides a single-page view of the elevator fleet across Ontario. It enables the operations manager to see fleet status at a glance, identify elevators with overdue inspections and expiring licenses, and look up details about individual elevators—all without scrolling through spreadsheets.

---

## 1. Page Layout

The dashboard consists of four visual areas:

1. **Sidebar**: A navigation panel on the left side of the page. It contains the "Rocket Elevators" title and a menu that currently links to the Dashboard. The sidebar is designed to support additional pages in the future if needed.

2. **Page Header**: A top bar displayed above the main content area. It contains a page title and a subtitle that together identify the scope of the current view.
   - **Title**: Operational Fleet Overview
   - **Subtitle**: Active and by-request licensed devices
   - The title reflects that the dashboard presents an operational subset of the Ontario elevator registry (ACTIVE and BY REQUEST licenses only), not the complete registry. The subtitle reinforces the technical rendering approach.

3. **Summary Cards**: Four metric cards displayed at the top of the main content area. Each card shows a key fleet statistic that answers the operations manager's top questions: How many elevators do we have? How many are active? How many have overdue inspections? Are any licenses expiring soon? On desktop screens, the four cards appear in a single horizontal row. On smaller screens, the cards may wrap to multiple rows while preserving their original order.

4. **Detail Table**: Below the summary cards, a sortable table displays all elevators in the fleet. The operations manager can sort by any column and search by elevator ID or location to look up a specific elevator's details.

---

## 2. Summary Cards

Four metric cards appear at the top of the dashboard in a horizontal row.

### Card 1: Total Elevators

- **Metric**: Total Elevators
- **Display**: A single number showing the total count of elevators in the fleet.
- **Calculation**: `total_elevators` field from `GET /api/fleet/stats`.
- **Data Source**: Go API — `GET /api/fleet/stats`
- **Purpose**: Provides the fleet size at a glance.

### Card 2: Active Elevators

- **Metric**: Active Elevators
- **Display**: A count and percentage (e.g., "847 (92%)").
- **Calculation**: `total` field from `GET /api/elevators?status=ACTIVE&limit=1`, divided by Card 1 total for the percentage.
- **Data Source**: Go API — `GET /api/elevators`
- **Purpose**: Shows what portion of the fleet is operational right now.

### Card 3: Overdue Inspections

- **Metric**: Overdue Inspections (approximated)
- **Display**: A count of elevators without a passing inspection on record.
- **Calculation**: `round((1 - inspection_pass_rate) * total_elevators)` derived from `GET /api/fleet/stats`. This counts elevators that have no inspection record or whose most recent inspection did not pass — the closest available proxy for the time-based overdue definition (latest inspection > 1 year ago or missing).
- **Data Source**: Go API — `GET /api/fleet/stats`
- **Limitation**: The Go API does not expose a time-based "overdue" count. The displayed value is a non-passing inspection proxy, not an exact 365-day staleness count. A dedicated API endpoint would be required for the exact metric.
- **Purpose**: Highlights compliance risk — surfaces elevators that need inspection attention.

### Card 4: Licenses Expiring Soon

- **Metric**: Licenses Expiring in 30 Days
- **Display**: Always `0` in the current implementation.
- **Calculation**: Not available from the Go API. The current API contract (`GET /api/fleet/stats`, `GET /api/elevators`) does not expose date-range queries for license expiry. Value is hardcoded to `0` until a supporting endpoint is added.
- **Data Source**: Go API — not yet exposed
- **Limitation**: Implementing this metric requires either a new API endpoint (e.g., `GET /api/fleet/stats` extended with `expiring_soon_count`) or a date-filter parameter on `GET /api/elevators`.
- **Purpose**: Alerts operations staff to upcoming license renewals that need planning.

### Metric Computation

Summary card values are sourced from the Go API on initial page load. **Card 1 (Total Elevators) updates to reflect the active filter/search state** whenever the table reloads — its value matches the paginated result count returned by the API. Cards 2–4 reflect fleet-wide values set at page load and do not recompute per filter.

| Card | Data source | Updates on filter? |
|---|---|---|
| **Total Elevators** | `GET /api/fleet/stats → total_elevators` | Yes — shows filtered result count |
| **Active Elevators** | `GET /api/elevators?status=ACTIVE&limit=1 → total` | No — fleet-wide value |
| **Overdue Inspections** | `GET /api/fleet/stats → round((1 - inspection_pass_rate) * total_elevators)` | No — fleet-wide value |
| **Licenses Expiring in 30 Days** | Not available from Go API — displays `0` | No |

---

## 3. Detail Table

Below the summary cards, a table displays all elevators in the fleet. The table is sortable: clicking a column header sorts the table by that column. A search box above the table allows filtering by elevator ID or location name.

### Table Columns

The table displays columns sourced from license.csv, inspection.csv, and installed.json. Records are associated using ElevatingDevicesNumber (the common field across all three datasets):

| Dashboard Column | Source Dataset | Source Field | Data Type | Display Format | Purpose |
|---|---|---|---|---|---|
| **Elevator ID** | license.csv | ElevatingDevicesNumber | Text | As stored | Unique identifier for each elevator; used to reference a specific unit |
| **Location** | license.csv | LocationoftheElevatingDevice | Text | As stored | Address or building name where the elevator is installed |
| **License Number** | license.csv | ElevatingDevicesLicenseNumber | Text | As stored | License number assigned to the elevator |
| **Status** | license.csv | LICENSESTATUS | Text | As stored | Current license status (e.g., ACTIVE, BY REQUEST, CANCELLED_NOT_RENEWED) |
| **License Expiration Date** | license.csv | LICENSEEXPIRYDATE | Date | YYYY-MM-DD | Date when the current license expires |
| **Latest Inspection Date** | inspection.csv | Latest_INSPECTION_Date | Date | YYYY-MM-DD | Date of the most recent inspection on record; blank if no inspection record exists |
| **Latest Inspection Outcome** | inspection.csv | InspectionOutcome | Text | As stored | Outcome of the most recent inspection (e.g., PASS/FAIL); blank if no inspection record exists |
| **Elevator Type** | installed.json | Device Type | Text | As stored | Type of elevator (e.g., Passenger Elevator, Freight Elevator); blank if no installed.json match |

### Interactive Behavior

> **Note:** The static prototype used custom JavaScript for client-side sorting and search. The dynamic version replaces this with server-driven updates — no custom JavaScript is required for table interactivity.

#### Filtering

Two dropdown filters appear above the table. Selecting a value immediately updates the table rows to show only matching elevators, without a full page reload.

| Filter | Source field | Behavior |
|---|---|---|
| **Status** | `status` (license.csv.LICENSESTATUS) | Limits rows to the selected status value; shows all statuses when blank |
| **Elevator Type** | `type` (installed.json.Device Type) | Limits rows to the selected equipment type; shows all types when blank |

Both filters apply immediately on selection; no submit button is required.

#### Sorting

Two columns are sortable: **License Expiration Date** and **Latest Inspection Date**. Clicking a column header toggles the sort direction (ascending → descending → ascending). The table updates to reflect the new order without a full page reload.

| Sortable column | Sort directions |
|---|---|
| License Expiration Date | ascending / descending |
| Latest Inspection Date | ascending / descending |

Clicking a column header updates both the table row order and the column header sort indicators to reflect the new direction. All ordering logic is handled by the server; the browser holds no sort state.

#### System Behavior

- The table reflects the combined result of all active filters, search query, and sort state simultaneously.
- All filtering, searching, and sorting logic runs on the server; the client carries no application state.
- Table updates are partial. The page header and sidebar are never affected. Summary cards update alongside the table in a single request using out-of-band swaps; no separate request is needed.

#### Loading Feedback

The dashboard provides visual feedback whenever it is waiting for data from the server.

**Where it appears:**
- The table area shows a loading indicator whenever a filter, search, or sort update is in progress.
- The detail panel shows a loading indicator while the elevator's data is being retrieved.

**When it appears:**
- The indicator becomes visible as soon as the user performs any action that triggers a data update: selecting a filter value, typing in the search box, clicking a sort header, or clicking an elevator row.

**When it disappears:**
- The indicator is hidden as soon as the updated content is fully displayed in the table or detail panel.

**Behavior constraints:**
- The loading indicator does not block the rest of the page. The sidebar, page header, and summary cards remain fully visible while data is loading.
- The user's ability to interact with other controls is not affected by the loading state.


## Data Model

This section defines the unified **Elevator** entity that the dashboard backend will serve and the dashboard UI will display. Field choices here determine valid filtering and sorting options for the interactive dashboard.

### AND-2 Task 2: Elevator Entity Fields

For each field: name, data type, source (dataset + column), and description.

| Field name | Data type | Source (dataset.column) | Description |
|---|---|---|---|
| elevator_id | text | license.csv.ElevatingDevicesNumber | Primary elevator identifier and join key used to unify datasets. |
| location | text | license.csv.LocationoftheElevatingDevice | Full location string for the elevator (address/building descriptor) shown in the table. |
| location_city_region | text | Derived from license.csv.LocationoftheElevatingDevice | City/region extracted from the location string (derived field; used to support the “city/region” requirement and future filtering). |
| equipment_type | text | installed.json.Device Type | Elevator equipment type/category (e.g., Passenger, Freight). Displayed as “Elevator Type”. |
| status | text | license.csv.LICENSESTATUS | Operational/license status (e.g., ACTIVE, BY REQUEST, CANCELLED_NOT_RENEWED). Used for filtering. |
| license_expiry_date | date | license.csv.LICENSEEXPIRYDATE | License expiry date (normalized to YYYY-MM-DD). Used for sorting and renewals monitoring. |
| last_inspection_date | date | inspection.csv.Latest_INSPECTION_Date (derived per elevator) | Most recent inspection date per elevator (normalized to YYYY-MM-DD). Null/blank if none exists. |
| last_inspection_outcome | text | inspection.csv.InspectionOutcome (derived per elevator) | Outcome of the most recent inspection (pass/fail). Null/blank if none exists. |

### Derivations and Join Rules

- **Primary join key:** `license.csv.ElevatingDevicesNumber` is the canonical elevator identifier. It is used to associate records across datasets (e.g., inspections and installed device type).
- **One-to-many handling (inspections):** inspection records may be many per elevator. The dashboard entity uses only the **most recent** inspection to populate `last_inspection_date` and `last_inspection_outcome`.
- **City/region requirement:** the datasets provide a full location string, not separate city/region fields. `location_city_region` is therefore a **derived** field, produced by parsing the location string consistently (or left blank when parsing fails).
- **Display note:** `location_city_region` supports the “city/region” requirement and potential future filtering, but it is not displayed as a separate table column in this initial version (the table displays the full `location` string).

### Data Limitations

Some fields may be blank due to missing cross-dataset matches or missing inspection history:
- Elevators with no inspection record will have blank **Latest Inspection Date** and **Latest Inspection Outcome**, and are treated as overdue in the overdue inspections metric.
- Elevators with no installed.json match will display a blank **Elevator Type**.
- The **location_city_region** field is derived from a free-text location string and may be blank if parsing is ambiguous.

---

## Data Notes

- **Data Sources**: The dashboard integrates data from three datasets:
  - **license.csv**: Device and license information (core dataset; all elevators are listed here).
  - **inspection.csv**: Inspection history including inspection dates.
  - **installed.json**: Device classification information, including elevator type.

- **Record Association**: All three datasets use a common field to identify elevators:
  - license.csv: `ElevatingDevicesNumber`
  - inspection.csv: `ElevatingDevicesNumber`
  - installed.json: `Elevating devices number`

  These fields contain the same elevator identifier and are used to associate records across datasets.

- **Other Available Datasets**: The project also includes altered.json (alteration/modification history), incident.json (incident/accident records), and order.csv (inspection order tracking). These datasets are not used in this initial dashboard version; they may be integrated in future enhancements.

- **Date Format Handling**: Dates in license.csv (LICENSEEXPIRYDATE) and inspection.csv (Latest_INSPECTION_Date) may be stored in various formats (e.g., "DD-MMM-YY", "MM/DD/YYYY"). The dashboard normalizes all dates to YYYY-MM-DD format for consistent display and reliable sorting.

- **Date Calculations**: Date calculations for metrics and filters use today's date as the reference point. The dashboard recalculates metrics each time it is loaded.

- **Field Values**: Values in LICENSESTATUS, LICENSEEXPIRYDATE, Latest_INSPECTION_Date, Device Type, and other fields are used exactly as provided in their respective datasets.

- **Missing Inspection Records**: Not all elevators in license.csv have corresponding inspection records in inspection.csv. Elevators with missing inspection records are treated as having no inspection history and are flagged in the overdue inspections metric.

- **Missing Device Type**: Elevator records that do not have a corresponding entry in installed.json will display a blank Elevator Type in the detail table.

- **Data Quality**: Data quality issues or discrepancies between the datasets will be documented during implementation.

---

## 4. Visual Design

This section defines the presentation and branding of the dashboard. It does not change any layout structure or data definitions from the sections above.

### 4.1 Branding

The sidebar displays the "Rocket Elevators" brand name at the top, above the navigation menu. The brand area should be visually distinct from the rest of the sidebar — larger text weight, slightly more padding, and a bottom border to separate it from the nav links. A simple rocket icon (text character or inline SVG) may appear immediately to the left of the brand name to reinforce identity. The overall treatment should feel like a professional enterprise product, not decorative.

### 4.2 Color Palette

The dashboard uses a restrained, industrial palette appropriate for an operations tool:

| Role | Color |
|---|---|
| Sidebar background | Dark navy or dark slate (e.g., #0f172a or #1e293b) |
| Sidebar text | White or light gray |
| Page background | Light gray (e.g., #f1f5f9) |
| Card and table backgrounds | White |
| Borders | Light gray (e.g., #e2e8f0) |
| Primary text | Dark gray (e.g., #1e293b) |
| Secondary / label text | Medium gray (e.g., #64748b) |
| Active accent | Green (e.g., #16a34a or similar) |
| Warning / overdue accent | Red (e.g., #dc2626 or similar) |
| Expiring soon accent | Amber (e.g., #d97706 or similar) |

No decorative gradients, shadows beyond subtle card elevation, or non-functional color are used.

### 4.3 Summary Card Visual Differentiation

Each summary card emphasizes its metric number over its label. The label appears in small, uppercase, muted text above the number. The number is displayed in a large, bold font.

Three of the four cards use an accent color on their metric value to signal state at a glance:

| Card | Metric color | Rationale |
|---|---|---|
| Total Elevators | Default dark text | Neutral count; no state signal needed |
| Active Elevators | Green accent | Positive operational state |
| Overdue Inspections | Red accent | Compliance risk; requires attention |
| Licenses Expiring in 30 Days | Amber accent **when > 0**, neutral gray when 0 | Time-sensitive but not yet critical; a value of zero means no upcoming renewals, a positive state, so it is not flagged |

### 4.4 Status Column Badges

In the detail table, the LICENSESTATUS value is displayed as a small colored badge rather than plain text, using the same accent color system:

| Status value | Badge color |
|---|---|
| ACTIVE | Green background, dark green text |
| BY REQUEST | Amber background, dark amber text |
| CANCELLED_NOT_RENEWED | Red background, dark red text |

### 4.5 Overdue Inspection Highlighting

In the Latest Inspection Date column, any value that qualifies as overdue (more than one year old, or blank) is displayed in red text. This is consistent with the red accent used for the Overdue Inspections summary card and reinforces the urgency without adding additional UI elements.

### 4.6 Inspection Outcome Visualization

Inspection outcomes are displayed using distinct visual indicators to convey pass/fail status at a glance:

| Outcome value | Visual indicator | Usage |
|---|---|---|
| Passed | Green background or text | Indicates successful inspection |
| Fail | Red background or text | Indicates failed inspection |
| Follow up | Yellow/amber background or text | Indicates inspection requires follow-up |

These indicators must be applied consistently in:
- The detail panel (inspection history table)
- The detail table (if inspection outcome column is visible)

The outcome values displayed must match the actual values present in the inspection.csv dataset.

### 4.7 Visual Hierarchy

- Card metric values are the largest text element on the page (large, bold).
- Card labels are small, uppercase, and muted — secondary to the value.
- Table header labels are small, uppercase, and muted — consistent with card labels.
- Table row text is standard size and dark, with hover highlight on rows for readability.
- No decorative elements (icons, charts, illustrations) are added beyond what is described above, except the risk-distribution **donut** on the Fleet Health panel (Feature 3) — a functional, no-JavaScript data visual, not decoration.

---

## Scope

This specification describes the initial dashboard as requested by the operations manager. It is a single page focused on fleet overview and elevator lookup. The dashboard uses three core datasets: license.csv, inspection.csv, and installed.json. Future additions, such as detailed reporting, alteration tracking, incident analysis, or additional pages, are outside the scope of this version.

---

## AND-103 Task 1: Interaction Specification

---

### Interaction 1: Elevator Detail Panel

#### Outcomes
The user can click on any elevator row in the table and instantly view a complete operational profile of that elevator in a side panel. The panel consolidates inspection history, incident count, alteration count, and current operational status into a single view, eliminating the need to cross-reference multiple sources.

#### Scope Boundaries
- Includes: inspection history, incident count, alteration count, current status.
- Excludes: editing data, performing actions (read-only view).
- The panel only displays data for the selected elevator; it does not modify the dataset.

#### Constraints
- Must load without full page reload.
- Must not block table interaction.
- Must handle missing data (e.g., no incidents or inspections).
- Must maintain performance (fast load under typical dataset size).

#### Prior Decisions
- Data is sourced from the unified dataset created in Task 5.
- Inspection data is already deduplicated (latest inspection per elevator).
- All dynamic updates are handled server-side; no custom JavaScript logic is required in the client.

##### Task Breakdown
- The user can click on any row in the table to select an elevator
- Selecting a row loads that elevator’s data into a detail panel without refreshing the page
- The detail panel appears on the right side of the interface and remains visible while an elevator is selected
- The panel displays:
  - Elevator ID
  - Location
  - Status
  - Inspection history
  - Incident count
  - Alteration count

- Panel layout:
  - Header section: Elevator ID and Status prominently displayed
  - Location section: full location string
  - Inspection history: displayed as a table with Date and Outcome columns, sorted with the most recent inspection first
  - Incident count and alteration count displayed as summary values

- Panel behavior:
  - Opening: selecting a row displays the panel with that elevator’s data
  - Updating: selecting a different row updates the panel content
  - Closing: the panel can be closed by a dedicated close control or by clicking outside the panel

- Edge cases:
  - If the selected elevator is no longer visible due to filtering or searching, the panel closes automatically
  - If an elevator has no inspection or incident data, the panel displays a clear “No data available” message
  
#### Verification Criteria
- Clicking a row opens the panel with correct data.
- Switching rows updates the panel correctly.
- Panel can be closed reliably.
- No page reload occurs.
- Edge cases (empty data, filtered-out selection) are handled gracefully.
- **Interaction Conflicts:**
  - **Filtered removal:** If a search query or filter change removes the currently selected elevator from the visible table, the detail panel closes automatically.
  - **Sort visibility:** If sorting changes the row order but the selected elevator remains visible in the filtered/searched dataset, the panel remains open and continues to display the elevator's data.
  - **Rapid interactions:** If multiple interactions occur in quick succession (e.g., filter + search + sort), only the latest request is processed and reflected; intermediate states are not displayed.

---

### Interaction 2: Filter and Search Interaction

#### Outcomes
The user can filter and search elevators in real time using dropdowns and a search input. The table updates dynamically as the user types, enabling fast discovery of specific elevators by ID or location.

#### Scope Boundaries
- Includes: filtering by status and type, searching by elevator ID and location.
- Excludes: advanced search (regex, fuzzy matching beyond basic contains).
- Interacts only with the table and detail panel state.

#### Constraints
- Must use debounced search (minimum 300–500ms delay).
- Must not trigger a request on every keystroke.
- Must be responsive even with large datasets.
- Must combine filters AND search.

#### Prior Decisions
- Dropdown filters already exist (Module 2).
- Dynamic table updates do not require custom client-side scripting.
- The server accepts filter, search, and sort parameters in combination and returns a consistent result.

#### Task Breakdown
- Add a search input that updates the table after the user pauses typing, without requiring a submit action.
- The system matches the search query against Elevator ID and Location fields (case-insensitive).
- All active filters (status, type) and the search query are applied simultaneously; the table reflects their combined result.
- Define interaction with detail panel:
  - If selected elevator no longer appears → close detail panel
- Update all table-related UI elements (not full page)
- Define search matching behavior:
  - Matching is **case-insensitive** across all search fields.
  - Matching uses **partial match** (contains): if the search query appears anywhere within the field value, the record matches.
  - Search applies to **both** Elevator ID and Location fields.
  - If both fields match the search query, the record appears **once** (no duplicate rows; no prioritization between fields).
  - Empty search string shows all records (no restrictions).

#### Verification Criteria
- Typing in search updates table dynamically.
- Filters and search work together correctly.
- Clearing search restores previous filter state.
- The table does not update on every keystroke; it updates only after the user pauses typing.
- Detail panel reacts correctly to filtered search results.

#### Interaction Conflicts
- **Filtered removal:** If a search query or filter hides the currently selected elevator in the detail panel, the detail panel closes automatically.
- **Search + filter combination:** Applying search and filters in any order produces consistent results; the order of interaction does not affect which records appear.
- **Rapid search/filter changes:** If multiple search or filter changes occur in quick succession, only the latest request state is reflected; intermediate requests that occur during the debounce delay are discarded.

---

### Interaction 3: Sort Behavior

#### Outcomes
The user can sort elevators by specific columns (e.g., expiry date, inspection date) to prioritize operational decisions. Sorting toggles direction and works consistently with filters and search.

#### Scope Boundaries
- Includes: sorting by predefined columns only.
- Excludes: multi-column sorting or custom sorting logic.
- Applies only to visible dataset (after filtering/search).

#### Constraints
- Must preserve filter and search state.
- Must not reload full page.
- Must use server-side sorting.

#### Prior Decisions
- Sorting is applied server-side; the client sends the sort parameters and receives an updated table.
- Sorting is performed on the full filtered dataset.
- Two-column sorting capability exists.

#### Task Breakdown
- Define default sort:
  - License expiry (ascending)
- Define toggle behavior:
  - First click → ascending
  - Second click → descending
  - Subsequent clicks → toggle between ascending and descending
- Ensure sorting retains:
  - current filters
  - current search input
- When sorted, the full table — including column header sort indicators — refreshes to reflect the new sort direction.

- Edge cases:
  - Sorting while detail panel open → panel remains if elevator still visible
  - Sorting + filtering combination must not break state

#### Verification Criteria
- Sorting toggles correctly on repeated clicks.
- Sorting respects active filters and search.
- Table updates without page reload.
- Correct column ordering validated.
- Panel state behaves consistently during sorting.

#### Interaction Conflicts
- **Sorted visibility:** If sorting changes the row order but the selected elevator remains in the filtered/searched dataset, the detail panel stays open and continues to display the elevator's data.
- **Filtered removal during sort:** If sorting is applied and the selected elevator is outside the filtered dataset, the detail panel closes (same as filter/search removal).
- **Multiple rapid sort clicks:** If the user clicks a sort header multiple times in rapid succession, only the final sort state is displayed; intermediate states are not processed or shown.

---

## AND-104 Task 8: Risk Integration

This section extends the dashboard to surface ML risk data from the Go API (`http://localhost:8080`). All new risk data is fetched from the Go API at runtime; no new CSV reads are added to the Flask server.

---

### Feature 1: Risk Level Column in Fleet Table

A new **Risk Level** column is added as the last (9th) column of the fleet table.

- **Display:** Color-coded badge matching the existing status/outcome badge style:
  - `HIGH` → red badge
  - `MEDIUM` → amber badge
  - `LOW` → green badge
  - No prediction available → "—" (muted gray text)
- **Data source:** `risk_level` field from `GET /api/elevators` response (included in every paginated result)
- **Behavior:** Read-only; no filtering or sorting by risk level in this version

---

### Feature 2: Pagination

The fleet table is paginated at **50 rows per page**.

- **Controls:** Previous / Next buttons appear below the table with a result count: "Page N of M — K results"
- **Reset:** Any filter change, search change, or sort action resets to page 1
- **State preservation:** Pagination buttons include all active filter and search parameters
- **No JavaScript:** Pagination controls use HTMX `hx-get` with `hx-include="#controls"` — no client-side state management

---

### Feature 3: Fleet Health Panel

A **fleet health panel** appears between the summary cards and the fleet table. It is loaded via HTMX on page load from `GET /api/fleet/stats`.

**Displayed data:**

| Metric | Description |
|---|---|
| Risk distribution | Count and percentage for each risk level: LOW, MEDIUM, HIGH, UNKNOWN |
| Inspection pass rate | Percentage of elevators with at least one passing inspection |

**Visual design:** Consistent with existing summary cards — white card, muted labels, bold values, accent colors matching the risk badge scheme (green/amber/red/slate). A CSS conic-gradient **donut** (no JavaScript, no chart library) shows the LOW/MEDIUM/HIGH/UNKNOWN split as parts of a whole, with the total device count in its center and a legend listing each count and percentage. Donut and legend colors match the risk badge scheme.

**Error handling:** If the Go API is unreachable, display "Fleet health data unavailable." in place of the panel.

---

### Feature 4: Alerts Section

A **critical alerts section** appears below the fleet table. It is loaded via HTMX on page load from `GET /api/fleet/alerts`.

**Displayed data:** Top 20 alerts (already sorted by `risk_score` DESC by the API), with columns:

| Column | Description |
|---|---|
| Elevator ID | Monospace identifier |
| Location | Full address, wrapped to a maximum of two lines (no native `title` tooltip, which overlapped the row below) |
| Latest Inspection | Most recent inspection date; "No inspection on record" in red when missing (a missing inspection is itself an alert reason) |
| Latest Inspection Outcome | Color badge with graded severity. `Passed`/`All Orders Resolved`=green; `Follow up`=amber; `DC Follow up`=orange; `Follow up Major`=red; `Fail`=red. Because this section is failed-most-recent-inspection only, any other non-pass value (e.g. `Complete`) is shown in amber to signal it still needs action. The literal value from `inspection.csv` is preserved (§4.6) — only the color encodes severity. |
| Risk Score | Formatted as 0.XX, red badge |

**Header:** "Critical Alerts — Showing N of M total (HIGH risk + failed inspection)"

**Error handling:** If the Go API is unreachable, display "Alerts unavailable." in place of the section.

---

### Feature 5: Risk Assessment in Detail Panel

The elevator detail panel gains a **Risk Assessment** section, displayed after the Incidents/Alterations summary and before Inspection History.

**Displayed data when a prediction exists (200):**
- Risk level badge (HIGH/MEDIUM/LOW, same color scheme as table)
- Risk score formatted to 2 decimal places (e.g., "Score: 0.87")
- Model confidence as a percentage (e.g., "Confidence: 87%")
- Predicted failure date if present (e.g., "Predicted failure: 2026-09-14"); omitted if null

**Displayed data when no prediction exists:**
- "No prediction available for this elevator." (muted text)

**Displayed data when API is unreachable:**
- "Risk data unavailable." (muted text)

**Data source:** `GET /api/elevators/{id}/risk` (called on each panel load)

---

## AND-105 Task 2: Relational Data Model

### Overview

Five tables form the relational schema. `elevators` is the central entity; all other tables reference it via `elevator_id`. The join key across all source datasets is `ElevatingDevicesNumber` (license.csv) / `elevating devices number` (incident.json) / `Elevating Devices Number` (altered.json), normalized to `elevator_id INTEGER` in all tables.

---

### Tables

#### elevators
**Source:** `data/license.csv`  
**Primary key:** `elevator_id` — the `ElevatingDevicesNumber` field is the stable government-assigned identifier used as the join key across every dataset; chosen as natural PK over a surrogate to preserve traceability.  
**Purpose:** Central entity. Represents one licensed elevating device. All operational data (inspections, incidents, alterations, predictions) is anchored here.

| Column | Type | Constraints | Source field | Transformation |
|--------|------|-------------|--------------|----------------|
| elevator_id | INTEGER | PK NOT NULL | ElevatingDevicesNumber | Cast to INTEGER |
| location | TEXT | NOT NULL | LocationoftheElevatingDevice | None |
| license_number | TEXT | NOT NULL | ElevatingDevicesLicenseNumber | None |
| status | TEXT | NOT NULL CHECK IN ('ACTIVE','BY REQUEST') | LICENSESTATUS | Uppercase normalized |
| license_expiry_date | DATE | — | LICENSEEXPIRYDATE | Parse "28-Apr-17" → DATE |
| license_holder | TEXT | — | LICENSEHOLDER | None |
| license_holder_address | TEXT | — | LICENSEHOLDERADDRESS | None |
| billing_customer | TEXT | — | BILLINGCUSTOMER | None |
| billing_address | TEXT | — | BILLINGADDRESS | None |
| elevator_type | TEXT | — | Elevator Type (elevator_fleet.csv) | None |

**Indexes:** `status`, `license_expiry_date`

---

#### inspections
**Source:** `data/inspection.csv`  
**Primary key:** `inspection_id` — the `InspectionNumber` field is the unique inspection record identifier assigned by the regulator; stable natural PK.  
**Purpose:** Separate entity because one elevator accumulates many inspection records over its lifetime (1:N). Keeping inspections separate allows querying inspection history, frequency, and outcome trends per elevator without denormalizing the elevators table.  
**Cardinality:** Many inspections → one elevator. `elevator_id` is NOT NULL; an inspection record without an elevator reference is meaningless.  
**Orphan behavior:** FK constraint prevents inserting an inspection for an elevator_id that does not exist in `elevators`.

| Column | Type | Constraints | Source field | Transformation |
|--------|------|-------------|--------------|----------------|
| inspection_id | INTEGER | PK NOT NULL | InspectionNumber | Cast to INTEGER |
| elevator_id | INTEGER | NOT NULL FK → elevators | ElevatingDevicesNumber | Cast to INTEGER |
| service_request_number | INTEGER | — | originatingservicerequestnumber | Cast to INTEGER |
| customer | TEXT | — | InspectionCustomer | None |
| location | TEXT | — | InspectionLocation | None |
| inspection_type | TEXT | — | InspectionType | None |
| earliest_inspection_date | DATE | — | Earliest_INSPECTION_Date | Parse "1/10/2011" → DATE |
| latest_inspection_date | DATE | — | Latest_INSPECTION_Date | Parse "1/10/2011" → DATE |
| outcome | TEXT | — | InspectionOutcome | None |

**Indexes:** `elevator_id`, `latest_inspection_date`, `outcome`

---

#### incidents
**Source:** `data/incident.json`  
**Primary key:** `incident_id` — the `Incident Number` field is the unique government-assigned incident identifier; stable natural PK.  
**Purpose:** Separate entity because one elevator can be involved in many reported incidents (1:N). Incident records capture safety events independently of routine inspections.  
**Cardinality:** Many incidents → one elevator. `elevator_id` is NOT NULL.  
**Orphan behavior:** FK constraint prevents inserting an incident for an elevator_id that does not exist in `elevators`.  
**Design decision:** The source contains ~30 sparse boolean injury-type columns (Burns Severe, Fracture Major Bone, etc.). These are collapsed into a single `injury_severity TEXT` column with values `fatal / permanent / minor / none`, derived from the four summary columns (`fatal injury`, `permanent (serious) injury`, `non-permanent (minor) injury`, `No Injury`). This avoids 30 nullable boolean columns that are almost always NULL.

| Column | Type | Constraints | Source field | Transformation |
|--------|------|-------------|--------------|----------------|
| incident_id | INTEGER | PK NOT NULL | Incident Number | Cast to INTEGER |
| elevator_id | INTEGER | NOT NULL FK → elevators | elevating devices number | Cast to INTEGER |
| creation_date | DATE | — | Creation Date | Parse "14-Jan-11" → DATE |
| date_of_occurrence | DATE | — | Date Of Occurrence | Parse "06-Jan-11" → DATE |
| time_of_occurrence | TIME | — | Time of Occurrence | Parse "2:00:00 PM" → TIME |
| category | TEXT | — | catagory of incident | None (typo preserved from source) |
| incident_summary | TEXT | — | Incident Summary | None |
| root_cause | TEXT | — | Specific Root Cause | None |
| narrative | TEXT | — | Reported occurrence narrative | None |
| fatal_injury | BOOLEAN | NOT NULL DEFAULT FALSE | Fatal Injury Victim | NULL → FALSE |
| injury_severity | TEXT | CHECK IN ('fatal','permanent','minor','none') | fatal/permanent/minor/No Injury columns | Derive from four summary columns |
| task_number | INTEGER | — | Task Number | Cast to INTEGER |

**Indexes:** `elevator_id`, `date_of_occurrence`, `category`

---

#### alterations
**Source:** `data/altered.json`  
**Primary key:** `alteration_id SERIAL` — no stable natural PK exists in the source; `originating service request number` is a reference number but not guaranteed unique across alteration records. Surrogate key used.  
**Purpose:** Separate entity because one elevator can have many alteration requests (1:N). Alterations track physical modifications to devices, distinct from inspections and incidents.  
**Cardinality:** Many alterations → one elevator. `elevator_id` is NOT NULL.  
**Orphan behavior:** FK constraint prevents inserting an alteration for an elevator_id that does not exist in `elevators`. `inspection_number` is nullable — not all alterations result in a linked inspection record.

| Column | Type | Constraints | Source field | Transformation |
|--------|------|-------------|--------------|----------------|
| alteration_id | SERIAL | PK NOT NULL | — | Surrogate; auto-generated |
| service_request_number | INTEGER | — | originating service request number | Cast to INTEGER |
| elevator_id | INTEGER | NOT NULL FK → elevators | Elevating Devices Number | Cast to INTEGER |
| customer | TEXT | — | Alteration Customer | None |
| summary | TEXT | — | Summary | None |
| location | TEXT | — | Alteration  Location | Strip extra whitespace |
| alteration_type | TEXT | — | Alteration Type | None |
| status | TEXT | — | Status of Alteration Request | None |
| contractor_name | TEXT | — | Alteration contractor name | None |
| billing_customer | TEXT | — | Billing Customer | None |
| inspection_number | INTEGER | FK → inspections (nullable) | Inspection number | Cast to INTEGER; NULL if absent |

**Indexes:** `elevator_id`, `status`, `service_request_number`

---

#### predictions
**Source:** `data/predictions.csv`  
**Primary key:** `elevator_id` — one prediction record per elevator (1:1 with elevators). elevator_id is both PK and FK, enforcing the one-to-one relationship and preventing duplicate prediction rows for the same elevator.  
**Purpose:** Separate entity to isolate ML model output from operational data. Predictions are regenerated independently of the source datasets and may not exist for all elevators.  
**Cardinality:** One prediction → one elevator (optional; not all elevators have a prediction).  
**Orphan behavior:** FK constraint prevents inserting a prediction for an elevator_id that does not exist in `elevators`. A missing row means no prediction exists for that elevator (the API returns 503 or 404 accordingly).

| Column | Type | Constraints | Source field | Transformation |
|--------|------|-------------|--------------|----------------|
| elevator_id | INTEGER | PK NOT NULL FK → elevators | elevator_id | Cast to INTEGER |
| risk_score | NUMERIC(5,4) | NOT NULL | risk_score | Cast to NUMERIC |
| risk_level | TEXT | NOT NULL CHECK IN ('LOW','MEDIUM','HIGH') | risk_level | Uppercase |
| risk_explanation | TEXT | — | — | Populated by ML pipeline at generation time |
| model_version | TEXT | NOT NULL | model_version | None |
| prediction_date | DATE | NOT NULL | prediction_date | Parse ISO date → DATE |

**Indexes:** `risk_level`, `prediction_date`

---

### Relationships Summary

| Relationship | Cardinality | Join columns | Orphan behavior |
|---|---|---|---|
| elevators → inspections | 1:N | elevators.elevator_id = inspections.elevator_id | FK prevents orphaned inspections |
| elevators → incidents | 1:N | elevators.elevator_id = incidents.elevator_id | FK prevents orphaned incidents |
| elevators → alterations | 1:N | elevators.elevator_id = alterations.elevator_id | FK prevents orphaned alterations |
| elevators → predictions | 1:1 (optional) | elevators.elevator_id = predictions.elevator_id | Absence of row = no prediction; no cascading delete defined |
| alterations → inspections | N:1 (optional) | alterations.inspection_number = inspections.inspection_id | Nullable FK; alteration may exist without a linked inspection |

---

## 5. Chat Widget (CHAT-2)

A floating Fleet Assistant panel ("OpsBot") gives operations staff a natural-language assistant for **understanding** the fleet — Ontario elevator regulations, device types, inspection rules, risk classification, and maintenance terminology — without leaving the dashboard.

**OpsBot is advisory and educational only.** It has **no live data access** and does not look up individual elevators, fleet counts, or current statuses. Any request for live data is declined with a redirect to the dashboard. This is the behavior validated in `docs/system-prompt-evaluation.md` (EVAL-1), and the chatbot implements that prompt verbatim — no fleet data is injected into the model.

### 5.1 Entry Point

A floating action button (FAB) is fixed to the bottom-right corner of the viewport (`position: fixed; bottom: 1.5rem; right: 1.5rem; z-index: 40`). Clicking it toggles the chat panel open or closed via a CSS `hidden` class — no page navigation, no URL change, no HTMX push-url.

### 5.2 Chat Panel

The panel is a fixed overlay anchored to the bottom-right corner (`z-index: 50`), **380 px wide × 520 px tall**. It does not push or reflow existing content. The dashboard table, summary cards, and detail panel remain interactive behind it.

**Sections (top to bottom):**
1. **Header** — dark navy background, "Fleet Assistant" label (left), close `[×]` button (right), Clear link (resets conversation).
2. **Message list** — `overflow-y: auto` scroll container; messages in chronological order (oldest top, newest bottom).
3. **Input bar** — single-line text field (`maxlength="500"`, placeholder "Ask about the fleet…") + send button.

### 5.3 Message Types

| Type | Alignment | Visual |
|---|---|---|
| User message | Right | Green (`#16a34a`) background, white text |
| Assistant message | Left | White card, `#e2e8f0` border |
| Error message | Left | `#fef2f2` background, dark red text |

### 5.4 Suggested Prompts

When the conversation is empty, three clickable chips appear in the message list above the input. Because OpsBot is advisory (no live data), the chips are educational prompts it can answer from its domain knowledge:

- "What do the risk levels mean?"
- "What is a Customer Shutdown?"
- "When is an inspection overdue?"

Clicking a chip submits that text as the message immediately. Chips are hidden once the first message is sent; they reappear after the conversation is cleared.

### 5.5 Input Behaviour

| Behaviour | Detail |
|---|---|
| After send | Field clears; input and send button are disabled until the reply arrives |
| Send button | Disabled when field is empty or while a reply is pending |
| Max length | 500 characters (`maxlength="500"`) |

### 5.6 Session State

Conversation history is stored client-side in a hidden `<input name="history">` field inside the chat form. Each submission sends the full history to Flask, which forwards it to the Go API. The Go API appends the new turn and returns the updated history. Flask overwrites the client field via an HTMX OOB swap. History is capped at 10 turns (20 messages); the Clear button resets it to `[]`.

### 5.7 Architecture

```
Browser (HTMX)  →  POST /chat  →  Flask :5000
Flask           →  POST /api/chat (JSON)  →  Go API :8080
Go API          →  system prompt + history + user message  (NO DB access)
Go API          →  POST /api/chat  →  Ollama :11434
Go API          ←  {reply, history}
Flask           ←  renders _chat_reply.html fragment (reply bubble + OOB history swap)
Browser         ←  HTMX appends bubble, overwrites history field
```

Flask is the sole intermediary — the browser never calls the Go API or Ollama directly. The Go API does **not** query the database for chat: it forwards the system prompt, conversation history, and user message straight to Ollama.

### 5.8 Go API Endpoint

**Route:** `POST /api/chat`

**Request:** `{"message": "...", "history": [{"role": "user"|"assistant", "content": "..."}]}`

**Response:** `{"reply": "...", "history": [...]}`

**Error codes:** `400` (missing/empty message), `500` (internal failure), `503` (Ollama unreachable)

### 5.8.1 System Prompt & Model

The Go API loads its system prompt from `platform/api/prompts/system_prompt.md` (embedded into the binary at build time) — the hardened "OpsBot" prompt produced and validated in PROMPT-1 / EVAL-1 (`docs/system-prompt-evaluation.md`), used **verbatim**. The prompt's boundaries are in force exactly as validated: no live data access, no specific elevator lookups, no regulatory/procedural advice, no fabrication, no identity override, and emergencies → 911.

**Model:** `mistral:7b` (`mistral:latest`), selected in EVAL-1 §4. Cold start exceeds 300s, so the Go API fires a warm-up request to Ollama on startup and all timeouts on the chat path are set to ~300s+.

### 5.9 No Live Data Access

OpsBot does **not** fetch fleet data. The Go API performs no intent detection and no database query for chat — it sends only the system prompt, conversation history, and the user's message to Ollama. This is deliberate: it matches the EVAL-1-validated behavior, where requests for live data (fleet counts, specific elevator status, lists of HIGH-risk devices) are cleanly declined and the user is redirected to the dashboard (EVAL-1 scenarios S4, S6, S8; question Q4). Live fleet data is the dashboard's job — the table, filters, summary cards, fleet-health panel, and alerts section. OpsBot's job is to help users *understand* what they see there.

### 5.10 Colour Usage

The chat widget reuses the dashboard's existing palette (§4.2). No new colours are introduced.

| Element | Colour |
|---|---|
| FAB + panel header | Dark navy `#0f172a` |
| User message bubble | Green `#16a34a` |
| Assistant message bubble | White, `#e2e8f0` border |
| Error bubble | `#fef2f2`, dark red text |

---

## 6. Multi-Page Navigation (SPA via HTMX)

This section supersedes the original single-page model. The dashboard splits into
three navigable pages inside a persistent shell. Navigation never reloads the full
page — only the main content area swaps, driven by HTMX.

> **Supersedes earlier text.** The original **Scope** section described a single page
> and listed "additional pages" as out of scope. That constraint is lifted for the
> three pages defined here. The data sources, table behavior, detail panel, and visual
> design from sections 1–5 are unchanged; only the page layout is reorganized.

### 6.1 Navigation Model

- A **persistent shell** holds the sidebar, the top bar, and the chat widget. These
  never reload during navigation.
- Each sidebar link uses `hx-get`, `hx-target="#main-content"`, `hx-push-url="true"`,
  and `hx-swap="innerHTML transition:true"`. Clicking a link swaps only the main
  content area and updates the browser address bar.
- The clicked link shows an **active state** (filled background, like the current
  "Dashboard" link) so the user always knows which page they are on.
- **Direct access and refresh (F5):** when a page URL is opened directly or reloaded,
  the server returns the **full shell** with that page's content embedded. The page
  must work as a normal URL, not only as an HTMX fragment.

### 6.2 Page Inventory

| Page | Route | Top bar title | Contents | Data sources |
|---|---|---|---|---|
| **Overview** | `/` | Operational Fleet Overview | Summary cards, fleet health panel, recent critical alerts preview (top 5) | Go API `/api/fleet/stats`, `/api/elevators`, `/api/fleet/alerts` |
| **Elevator Fleet** | `/fleet` | Elevator Fleet | Controls (search + filters), paginated fleet table, detail panel | Go API `/api/elevators`, `/api/elevators/{id}*` |
| **Alerts** | `/alerts` | Critical Alerts | Critical alerts table (HIGH risk + failed inspection) | Go API `/api/fleet/alerts` |

The existing fragment endpoints stay and are reused without change:
`GET /table`, `GET /fleet-health`, `GET /elevator/<id>`, `DELETE /elevator/<id>`.
The page routes above are the **navigable** routes; fragment routes are loaded by the
pages via `hx-trigger="load"` exactly as today.

### 6.3 Layout Shell (`layout.html`)

The shell is a base template with four fixed regions:

1. **Sidebar** — brand block (unchanged from §4.1) + the three nav links from §6.2,
   with active-state styling on the current page.
2. **Top bar** — page title and subtitle. The title changes per page (see §6.2). The
   sidebar and top bar are always visible.
3. **Main content** — a single `<main id="main-content">` container. This is the only
   region that swaps during navigation.
4. **Chat widget** — the floating FAB and panel from §5, anchored to the shell so it
   stays available on every page.

### 6.4 Flask Rendering Contract

Each page route checks the `HX-Request` header:

- **HTMX request** (`request.headers.get("HX-Request")` is truthy): render the page
  **partial only** (the content for `#main-content`).
- **Direct request / refresh** (header absent): render `layout.html` with the page
  partial embedded, the matching top-bar title, and the active sidebar link.

A small helper centralizes this so the three routes stay short. The same context
(active page, title) feeds both the partial and the full-shell render.

### 6.5 Summary Card Behavior Change

The four summary cards now live **only on the Overview page**, while the fleet table
lives on the **Elevator Fleet page**. Because they are on different pages, the cards
no longer update in response to table filters, search, or sort.

- The **Total Elevators** card shows the fleet-wide total (full dataset), not the
  filtered result count. This supersedes the "Updates on filter? — Yes" row for Total
  Elevators in §2.
- The filtered result count stays visible on the Fleet page in the pagination line
  ("Page N of M — K results").
- The out-of-band `#card-total` swap previously sent by `GET /table` is removed (the
  target no longer exists on the Fleet page).

### 6.6 Transitions and Loading

- Page swaps use `hx-swap="innerHTML transition:true"` (View Transitions API) with a
  CSS keyframe fade as a fallback for browsers without that API. The transition is a
  short, calm fade — no slide or zoom.
- `prefers-reduced-motion: reduce` disables the fade.
- While a page is loading, the main content area dims via the `.htmx-request` class
  (lower opacity), consistent with the loading feedback rules in §3. The sidebar, top
  bar, and chat widget stay fully visible and interactive.
- No custom JavaScript is added; all behavior uses HTMX attributes and CSS only.

### 6.7 Overview Interactions

The Overview page adds two navigation shortcuts (both use the same HTMX page-swap
mechanism as the sidebar — `hx-target="#main-content"`, `hx-push-url="true"`):

- **Active Elevators card → filtered Fleet.** Clicking the Active Elevators card opens
  the Fleet page pre-filtered to `status=ACTIVE` (`GET /fleet?status=ACTIVE`). The Fleet
  page reads the `status` query param, pre-selects the Status dropdown, and the initial
  table load reflects the filter. Only the Active card is a drill-down in this version:
  the Go API supports a `status` filter, but not a risk-level or "overdue" filter
  (AND-104 Feature 1: no filtering by risk level), so the other cards are not clickable.
- **Recent alerts preview → Alerts page.** A compact top-5 critical-alerts list fills the
  lower Overview area, loaded via `hx-trigger="load"` from `GET /api/fleet/alerts`. A
  "View all" link opens the full Alerts page. If the API is unreachable, the preview
  shows "Alerts unavailable."
