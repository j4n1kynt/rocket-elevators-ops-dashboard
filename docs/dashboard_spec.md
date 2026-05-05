# Rocket Elevators Operations Dashboard - Technical Specification

## Overview

The Rocket Elevators Operations Dashboard provides a single-page view of the elevator fleet across Ontario. It enables the operations manager to see fleet status at a glance, identify elevators with overdue inspections, and look up details about individual elevators—all without scrolling through spreadsheets.

---

## 1. Page Layout

The dashboard consists of three main sections:

1. **Sidebar**: A navigation panel on the left side of the page. It contains the "Rocket Elevators" title and a menu that currently links to the Dashboard. The sidebar is designed to support additional pages in the future if needed.

2. **Summary Cards**: Four metric cards displayed at the top of the main content area. Each card shows a key fleet statistic that answers the operations manager's top questions: How many elevators do we have? How many are active? How many have overdue inspections? Are any licenses expiring soon?

3. **Detail Table**: Below the summary cards, a sortable table displays all elevators in the fleet. The operations manager can sort by any column and search by elevator ID or location to look up a specific elevator's details.

---

## 2. Summary Cards

Four metric cards appear at the top of the dashboard in a horizontal row.

### Card 1: Total Elevators

- **Metric**: Total Elevators
- **Display**: A single number showing the total count of elevators in the fleet.
- **Calculation**: Count all elevators in the dataset.
- **Purpose**: Provides the fleet size at a glance.

### Card 2: Active Elevators

- **Metric**: Active Elevators
- **Display**: A count and percentage (e.g., "847 (92%)").
- **Calculation**: Count all elevators with status "Active" and divide by the total count to calculate the percentage.
- **Purpose**: Shows what portion of the fleet is operational right now.

### Card 3: Overdue Inspections

- **Metric**: Overdue Inspections
- **Display**: A count of elevators with overdue inspections.
- **Calculation**: Count all elevators where the last inspection date is more than one year before today. (For example, if today is May 5, 2026, any elevator with a last inspection date before May 5, 2025 is overdue.)
- **Purpose**: Highlights compliance risk. Ontario regulations require annual inspections; this card surfaces elevators that need attention.

### Card 4: Licenses Expiring Soon

- **Metric**: Licenses Expiring in 30 Days
- **Display**: A count of elevators with licenses expiring in the next 30 days.
- **Calculation**: Count all elevators where the license expiration date falls between today and 30 days from today (inclusive).
- **Purpose**: Alerts operations staff to upcoming license renewals that need planning.

---

## 3. Detail Table

Below the summary cards, a table displays all elevators in the fleet. The table is sortable: clicking a column header sorts the table by that column. A search box above the table allows filtering by elevator ID or location name.

### Table Columns

| Column | Data Type | Display Format | Purpose |
|---|---|---|---|
| **Elevator ID** | Text | As stored in the dataset | Unique identifier for each elevator; used to reference a specific unit |
| **Location** | Text | As stored in the dataset | Building name and city; helps locate the physical elevator |
| **Type** | Text | As stored in the dataset | Elevator type (e.g., hydraulic, traction); relevant for maintenance |
| **Status** | Text | As stored in the dataset | Current state of the elevator (e.g., active, inactive) |
| **License Expiration Date** | Date | YYYY-MM-DD format | Date when the current license expires |
| **Last Inspection Date** | Date | YYYY-MM-DD format | Date of the most recent safety inspection |

---

## Data Notes

- The exact field names and values in the source data will be confirmed during data exploration.
- Calculations assume the dataset contains at least the fields listed in the table above.
- Date calculations use today's date as the reference point; the dashboard recalculates metrics each time it is loaded.
- Any data quality issues or missing fields will be addressed during the data exploration phase.

---

## Scope

This specification describes the initial dashboard as requested by the operations manager. It is a single page focused on fleet overview and elevator lookup. Future additions, such as detailed reporting or additional pages, are outside the scope of this version.
