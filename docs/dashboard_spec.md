# Rocket Elevators Operations Dashboard - Technical Specification

## Overview

The Rocket Elevators Operations Dashboard provides a single-page view of the elevator fleet across Ontario. It enables the operations manager to see fleet status at a glance, identify elevators with overdue inspections and expiring licenses, and look up details about individual elevators—all without scrolling through spreadsheets.

---

## 1. Page Layout

The dashboard consists of three main sections:

1. **Sidebar**: A navigation panel on the left side of the page. It contains the "Rocket Elevators" title and a menu that currently links to the Dashboard. The sidebar is designed to support additional pages in the future if needed.

2. **Summary Cards**: Four metric cards displayed at the top of the main content area. Each card shows a key fleet statistic that answers the operations manager's top questions: How many elevators do we have? How many are active? How many have overdue inspections? Are any licenses expiring soon? On desktop screens, the four cards appear in a single horizontal row. On smaller screens, the cards may wrap to multiple rows while preserving their original order.

3. **Detail Table**: Below the summary cards, a sortable table displays all elevators in the fleet. The operations manager can sort by any column and search by elevator ID or location to look up a specific elevator's details.

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

### 4.6 Visual Hierarchy

- Card metric values are the largest text element on the page (large, bold).
- Card labels are small, uppercase, and muted — secondary to the value.
- Table header labels are small, uppercase, and muted — consistent with card labels.
- Table row text is standard size and dark, with hover highlight on rows for readability.
- No decorative elements (icons, charts, illustrations) are added beyond what is described above.

---

## Scope

This specification describes the initial dashboard as requested by the operations manager. It is a single page focused on fleet overview and elevator lookup. The dashboard uses three core datasets: license.csv, inspection.csv, and installed.json. Future additions, such as detailed reporting, alteration tracking, incident analysis, or additional pages, are outside the scope of this version.
