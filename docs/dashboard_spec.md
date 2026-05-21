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
- **Calculation**: Count all unique records in the license.csv dataset (using ElevatingDevicesNumber as the unique identifier).
- **Data Source**: license.csv
- **Purpose**: Provides the fleet size at a glance.

### Card 2: Active Elevators

- **Metric**: Active Elevators
- **Display**: A count and percentage (e.g., "847 (92%)").
- **Calculation**: Count all records in license.csv where LICENSESTATUS equals "ACTIVE", then divide by the total count to calculate the percentage.
- **Data Source**: license.csv
- **Purpose**: Shows what portion of the fleet is operational right now.

### Card 3: Overdue Inspections

- **Metric**: Overdue Inspections
- **Display**: A count of elevators with overdue inspections.
- **Calculation**: Count all elevators (from license.csv) where the Latest_INSPECTION_Date (from inspection.csv, joined via ElevatingDevicesNumber) is more than one year before today. For example, if today is May 5, 2026, any elevator with a Latest_INSPECTION_Date before May 5, 2025 is considered overdue. Elevators with no inspection record in inspection.csv are also considered overdue.
- **Data Sources**: license.csv (all elevators) + inspection.csv (inspection dates, joined on ElevatingDevicesNumber)
- **Purpose**: Highlights compliance risk. Ontario regulations require annual inspections; this metric surfaces elevators that need attention.

### Card 4: Licenses Expiring Soon

- **Metric**: Licenses Expiring in 30 Days
- **Display**: A count of elevators with licenses expiring in the next 30 days.
- **Calculation**: Count all records in license.csv where the LICENSEEXPIRYDATE falls between today and 30 days from today (inclusive).
- **Data Source**: license.csv
- **Purpose**: Alerts operations staff to upcoming license renewals that need planning.

### Metric Computation

Summary card values are updated whenever the table updates. Each metric reflects the currently filtered and searched dataset. On initial page load, all cards reflect the full fleet. When a filter or search is active, all four cards recompute to match the visible rows. Sorting does not change which rows are visible; cards still update to remain consistent.

| Card | Calculation |
|---|---|
| **Total Elevators** | Count of all records in elevator_fleet.csv. |
| **Active Elevators** | Count of records where `Status = "ACTIVE"`, displayed as both a raw count and a percentage of Total Elevators (e.g., "847 (92%)"). |
| **Overdue Inspections** | Count of records where `Latest Inspection Date` is blank **or** more than 365 days before today. |
| **Licenses Expiring in 30 Days** | Count of records where `License Expiration Date` falls between today and 30 days from today (inclusive). |

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
| Licenses Expiring in 30 Days | Amber accent | Time-sensitive but not yet critical |

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
- No decorative elements (icons, charts, illustrations) are added beyond what is described above.

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
