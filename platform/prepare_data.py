# Task 3 Part A — Build elevator_fleet.csv for the ops dashboard
# Run from repo root: python platform/prepare_data.py

import pandas as pd
from pathlib import Path

# ── Paths ────────────────────────────────────────────────────────────────────
ROOT = Path(__file__).parent.parent
DATA = ROOT / "data"
OUT  = ROOT / "platform" / "elevator_fleet.csv"

# ── 1. Load license data and filter to active licenses (Task 6c) ─────────────
license_df = pd.read_csv(DATA / "license.csv", dtype=str)
license_df.columns = license_df.columns.str.strip()
license_df["ElevatingDevicesNumber"] = license_df["ElevatingDevicesNumber"].str.strip()

# Keep only ACTIVE and BY REQUEST rows — same filter used in Module 1 Task 6c
license_df = license_df[
    license_df["LICENSESTATUS"].str.strip().isin(["ACTIVE", "BY REQUEST"])
].copy()

# Normalize license expiry date: DD-MMM-YY → YYYY-MM-DD
license_df["LICENSEEXPIRYDATE"] = pd.to_datetime(
    license_df["LICENSEEXPIRYDATE"], format="%d-%b-%y", errors="coerce"
).dt.strftime("%Y-%m-%d")

# ── 2. Load inspection data — keep only the most recent record per elevator ──
insp_df = pd.read_csv(DATA / "inspection.csv", dtype=str)
insp_df.columns = insp_df.columns.str.strip()
insp_df["ElevatingDevicesNumber"] = insp_df["ElevatingDevicesNumber"].str.strip()

# Parse Latest_INSPECTION_Date so we can sort and deduplicate
insp_df["_latest_dt"] = pd.to_datetime(
    insp_df["Latest_INSPECTION_Date"], errors="coerce"
)

# Sort descending and keep one row per elevator (the most recent inspection)
insp_df = (
    insp_df
    .sort_values("_latest_dt", ascending=False)
    .drop_duplicates(subset="ElevatingDevicesNumber", keep="first")
)

# Normalize the date column to YYYY-MM-DD string for the output CSV
insp_df["Latest_INSPECTION_Date"] = insp_df["_latest_dt"].dt.strftime("%Y-%m-%d")
insp_df = insp_df.drop(columns=["_latest_dt"])

# ── 3. Load installed device data ────────────────────────────────────────────
installed_df = pd.read_json(DATA / "installed.json", dtype=str)
installed_df.columns = installed_df.columns.str.strip()
installed_df["Elevating devices number"] = (
    installed_df["Elevating devices number"].str.strip()
)

# ── 4. Join — left merges from the filtered license dataframe ────────────────

# Attach latest inspection info
merged = license_df.merge(
    insp_df[["ElevatingDevicesNumber", "Latest_INSPECTION_Date", "InspectionOutcome"]],
    on="ElevatingDevicesNumber",
    how="left",
)

# Attach device type from installed.json
merged = merged.merge(
    installed_df[["Elevating devices number", "Device Type"]],
    left_on="ElevatingDevicesNumber",
    right_on="Elevating devices number",
    how="left",
)

# ── 5. Build the final output with the exact column order from the spec ───────
output = merged[[
    "ElevatingDevicesNumber",
    "LocationoftheElevatingDevice",
    "ElevatingDevicesLicenseNumber",
    "LICENSESTATUS",
    "LICENSEEXPIRYDATE",
    "Latest_INSPECTION_Date",
    "InspectionOutcome",
    "Device Type",
]].rename(columns={
    "ElevatingDevicesNumber":        "Elevator ID",
    "LocationoftheElevatingDevice":  "Location",
    "ElevatingDevicesLicenseNumber": "License Number",
    "LICENSESTATUS":                 "Status",
    "LICENSEEXPIRYDATE":             "License Expiration Date",
    "Latest_INSPECTION_Date":        "Latest Inspection Date",
    "InspectionOutcome":             "Latest Inspection Outcome",
    "Device Type":                   "Elevator Type",
})

# ── 6. Write output ───────────────────────────────────────────────────────────
output.to_csv(OUT, index=False)
print(f"Wrote {len(output):,} rows -> {OUT}")
