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
| **Status** | license.csv | LICENSESTATUS | Text | As stored | Current license status (e.g., Active, Inactive) |
| **License Expiration Date** | license.csv | LICENSEEXPIRYDATE | Date | YYYY-MM-DD | Date when the current license expires |
| **Latest Inspection Date** | inspection.csv | Latest_INSPECTION_Date | Date | YYYY-MM-DD | Date of the most recent inspection on record; blank if no inspection record exists |
| **Elevator Type** | installed.json | Device Type | Text | As stored | Type of elevator (e.g., Passenger Elevator, Freight Elevator) |

### Data Limitations

None of the core fields required by the operations manager are missing. All fields in the detail table exist in the available datasets.

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

## Scope

This specification describes the initial dashboard as requested by the operations manager. It is a single page focused on fleet overview and elevator lookup. The dashboard uses three core datasets: license.csv, inspection.csv, and installed.json. Future additions, such as detailed reporting, alteration tracking, incident analysis, or additional pages, are outside the scope of this version.
