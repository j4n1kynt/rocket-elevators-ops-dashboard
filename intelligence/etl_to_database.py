#!/usr/bin/env python3
"""
AND-105 Task 3: ETL — Flat files to PostgreSQL
Loads elevators, inspections, incidents, alterations, and predictions
from source files into the relational schema defined in 001_initial_schema.sql.
Re-runnable: uses ON CONFLICT DO NOTHING and idempotency guards.
"""

import csv
import json
import os
import time
from datetime import datetime
from pathlib import Path

import psycopg2
from psycopg2.extras import execute_values


# ---------------------------------------------------------------------------
# Connection
# ---------------------------------------------------------------------------

def get_connection():
    return psycopg2.connect(
        host=os.environ["DB_HOST"],
        port=os.environ.get("DB_PORT", "5432"),
        user=os.environ["DB_USER"],
        password=os.environ["DB_PASSWORD"],
        dbname=os.environ["DB_NAME"],
    )


# ---------------------------------------------------------------------------
# Type helpers
# ---------------------------------------------------------------------------

_DATE_FORMATS = [
    "%d-%b-%y",   # 28-Apr-17  (license.csv, incident.json)
    "%d-%b-%Y",   # 28-Apr-2017
    "%m/%d/%Y",   # 1/10/2011  (inspection.csv)
    "%Y-%m-%d",   # 2026-05-29 (predictions.csv)
]

_TIME_FORMATS = [
    "%I:%M:%S %p",   # 6:30:00 AM
    "%H:%M:%S",      # 14:30:00
]


def parse_date(value):
    if value is None or str(value).strip() in ("", "None", "N/A"):
        return None
    v = str(value).strip()
    for fmt in _DATE_FORMATS:
        try:
            return datetime.strptime(v, fmt).date()
        except ValueError:
            continue
    return None


def parse_time(value):
    if value is None or str(value).strip() in ("", "None"):
        return None
    v = str(value).strip()
    for fmt in _TIME_FORMATS:
        try:
            return datetime.strptime(v, fmt).time()
        except ValueError:
            continue
    return None


def to_int(value):
    if value is None or str(value).strip() in ("", "None"):
        return None
    try:
        return int(float(str(value).strip()))
    except (ValueError, TypeError):
        return None


def to_text(value):
    if value is None:
        return None
    v = str(value).strip()
    return v if v and v.lower() != "none" else None


def to_numeric(value):
    if value is None or str(value).strip() in ("", "None"):
        return None
    try:
        return float(str(value).strip())
    except (ValueError, TypeError):
        return None


def derive_injury_severity(row):
    if row.get("Fatal Injury Victim") or row.get("fatal injury"):
        return "fatal"
    if row.get("permanent (serious) injury"):
        return "permanent"
    if row.get("non-permanent (minor) injury"):
        return "minor"
    return "none"


# ---------------------------------------------------------------------------
# Migration
# ---------------------------------------------------------------------------

def run_migration(conn):
    with conn.cursor() as cur:
        cur.execute(
            "SELECT EXISTS (SELECT 1 FROM information_schema.tables "
            "WHERE table_schema = 'public' AND table_name = 'elevators')"
        )
        if cur.fetchone()[0]:
            print("  schema already exists — skipping migration")
            return
    sql = Path("platform/api/migrations/001_initial_schema.sql").read_text()
    with conn.cursor() as cur:
        cur.execute(sql)
    conn.commit()
    print("  migration applied")


# ---------------------------------------------------------------------------
# Load elevators ← data/license.csv
# ---------------------------------------------------------------------------

def load_elevators(conn):
    skipped, skip_reasons = 0, []

    rows = []
    with open("data/license.csv", newline="", encoding="utf-8") as f:
        for raw in csv.DictReader(f):
            # Skip duplicate header row present in source file
            if raw["ElevatingDevicesNumber"] == "ElevatingDevicesNumber":
                continue

            status = to_text(raw.get("LICENSESTATUS"))
            # Pipeline filter (CLAUDE.md): ACTIVE and BY REQUEST only
            if status not in ("ACTIVE", "BY REQUEST"):
                skipped += 1
                skip_reasons.append(f"elevator {raw['ElevatingDevicesNumber']}: status '{status}' filtered")
                continue

            elevator_id = to_int(raw.get("ElevatingDevicesNumber"))
            location    = to_text(raw.get("LocationoftheElevatingDevice"))
            lic_number  = to_text(raw.get("ElevatingDevicesLicenseNumber"))

            if elevator_id is None:
                skipped += 1
                skip_reasons.append("elevator row: missing elevator_id")
                continue
            if location is None:
                skipped += 1
                skip_reasons.append(f"elevator {elevator_id}: NULL location (NOT NULL constraint)")
                continue
            if lic_number is None:
                skipped += 1
                skip_reasons.append(f"elevator {elevator_id}: NULL license_number (NOT NULL constraint)")
                continue

            rows.append((
                elevator_id,
                location,
                lic_number,
                status,
                parse_date(raw.get("LICENSEEXPIRYDATE")),
                to_text(raw.get("LICENSEHOLDER")),
                to_text(raw.get("LICENSEHOLDERADDRESS")),
                to_text(raw.get("BILLINGCUSTOMER")),
                to_text(raw.get("BILLINGADDRESS")),
                None,  # elevator_type not present in license.csv
            ))

    sql = """
        INSERT INTO elevators (
            elevator_id, location, license_number, status, license_expiry_date,
            license_holder, license_holder_address, billing_customer,
            billing_address, elevator_type
        ) VALUES %s
        ON CONFLICT (elevator_id) DO NOTHING
        RETURNING elevator_id
    """
    with conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM elevators")
        before = cur.fetchone()[0]
        execute_values(cur, sql, rows, fetch=True)
        cur.execute("SELECT COUNT(*) FROM elevators")
        inserted = cur.fetchone()[0] - before
    conn.commit()

    _print_skip_sample(skip_reasons)
    return inserted, skipped, skip_reasons


# ---------------------------------------------------------------------------
# Load inspections ← data/inspection.csv
# ---------------------------------------------------------------------------

def load_inspections(conn, valid_elevator_ids):
    skipped, skip_reasons = 0, []

    rows = []
    with open("data/inspection.csv", newline="", encoding="utf-8") as f:
        for raw in csv.DictReader(f):
            inspection_id = to_int(raw.get("InspectionNumber"))
            elevator_id   = to_int(raw.get("ElevatingDevicesNumber"))
            outcome       = to_text(raw.get("InspectionOutcome"))

            if inspection_id is None:
                skipped += 1
                skip_reasons.append("inspection row: missing inspection_id")
                continue
            if elevator_id not in valid_elevator_ids:
                skipped += 1
                continue
            if outcome is None:
                skipped += 1
                skip_reasons.append(f"inspection {inspection_id}: NULL outcome (NOT NULL constraint)")
                continue

            rows.append((
                inspection_id,
                elevator_id,
                to_int(raw.get("originatingservicerequestnumber")),
                to_text(raw.get("InspectionCustomer")),
                to_text(raw.get("InspectionLocation")),
                to_text(raw.get("InspectionType")),
                parse_date(raw.get("Earliest_INSPECTION_Date")),
                parse_date(raw.get("Latest_INSPECTION_Date")),
                outcome,
            ))

    sql = """
        INSERT INTO inspections (
            inspection_id, elevator_id, service_request_number, customer,
            location, inspection_type, earliest_inspection_date,
            latest_inspection_date, outcome
        ) VALUES %s
        ON CONFLICT (inspection_id) DO NOTHING
    """
    with conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM inspections")
        before = cur.fetchone()[0]
        if rows:
            execute_values(cur, sql, rows)
        cur.execute("SELECT COUNT(*) FROM inspections")
        inserted = cur.fetchone()[0] - before
    conn.commit()

    _print_skip_sample(skip_reasons)
    return inserted, skipped, skip_reasons


# ---------------------------------------------------------------------------
# Load incidents ← data/incident.json
# ---------------------------------------------------------------------------

def load_incidents(conn, valid_elevator_ids):
    skipped, skip_reasons = 0, []

    with open("data/incident.json", encoding="utf-8") as f:
        raw_list = json.load(f)
    if not isinstance(raw_list, list):
        raw_list = list(raw_list.values())[0]

    rows = []
    for raw in raw_list:
        incident_id = to_int(raw.get("Incident Number"))
        elevator_id = to_int(raw.get("elevating devices number"))

        if incident_id is None:
            skipped += 1
            skip_reasons.append("incident row: missing incident_id")
            continue
        if elevator_id not in valid_elevator_ids:
            skipped += 1
            continue

        rows.append((
            incident_id,
            elevator_id,
            parse_date(raw.get("Creation Date")),
            parse_date(raw.get("Date Of Occurrence")),
            parse_time(raw.get("Time of Occurrence")),
            to_text(raw.get("catagory of incident")),
            to_text(raw.get("Incident Summary")),
            to_text(raw.get("Specific Root Cause")),
            to_text(raw.get("Reported occurrence narrative")),
            bool(raw.get("Fatal Injury Victim") or raw.get("fatal injury")),
            derive_injury_severity(raw),
            to_int(raw.get("Task Number")),
        ))

    sql = """
        INSERT INTO incidents (
            incident_id, elevator_id, creation_date, date_of_occurrence,
            time_of_occurrence, category, incident_summary, root_cause,
            narrative, fatal_injury, injury_severity, task_number
        ) VALUES %s
        ON CONFLICT (incident_id) DO NOTHING
    """
    with conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM incidents")
        before = cur.fetchone()[0]
        if rows:
            execute_values(cur, sql, rows)
        cur.execute("SELECT COUNT(*) FROM incidents")
        inserted = cur.fetchone()[0] - before
    conn.commit()

    _print_skip_sample(skip_reasons)
    return inserted, skipped, skip_reasons


# ---------------------------------------------------------------------------
# Load alterations ← data/altered.json
# ---------------------------------------------------------------------------

def load_alterations(conn, valid_elevator_ids):
    skipped, skip_reasons = 0, []

    # Idempotency guard: SERIAL PK has no natural conflict key.
    # Skip loading if the table is already populated.
    with conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM alterations")
        if cur.fetchone()[0] > 0:
            print("  already populated — skipping")
            return 0, 0, []

    with open("data/altered.json", encoding="utf-8") as f:
        raw_list = json.load(f)
    if not isinstance(raw_list, list):
        raw_list = list(raw_list.values())[0]

    with conn.cursor() as cur:
        cur.execute("SELECT inspection_id FROM inspections")
        valid_inspection_ids = {r[0] for r in cur.fetchall()}

    rows = []
    for raw in raw_list:
        elevator_id = to_int(raw.get("Elevating Devices Number"))
        if elevator_id not in valid_elevator_ids:
            skipped += 1
            continue

        insp_num = to_int(raw.get("Inspection number"))
        # Null out inspection_number if the referenced inspection was not loaded
        if insp_num not in valid_inspection_ids:
            insp_num = None

        rows.append((
            to_int(raw.get("originating service request number")),
            elevator_id,
            to_text(raw.get("Alteration Customer")),
            to_text(raw.get("Summary")),
            to_text(raw.get("Alteration  Location")),
            to_text(raw.get("Alteration Type")),
            to_text(raw.get("Status of Alteration Request")),
            to_text(raw.get("Alteration contractor name")),
            to_text(raw.get("Billing Customer")),
            insp_num,
        ))

    sql = """
        INSERT INTO alterations (
            service_request_number, elevator_id, customer, summary, location,
            alteration_type, status, contractor_name, billing_customer,
            inspection_number
        ) VALUES %s
    """
    with conn.cursor() as cur:
        if rows:
            execute_values(cur, sql, rows)
        inserted = len(rows)
    conn.commit()

    _print_skip_sample(skip_reasons)
    return inserted, skipped, skip_reasons


# ---------------------------------------------------------------------------
# Load predictions ← data/predictions.csv
# ---------------------------------------------------------------------------

def load_predictions(conn, valid_elevator_ids):
    skipped, skip_reasons = 0, []

    rows = []
    with open("data/predictions.csv", newline="", encoding="utf-8") as f:
        for raw in csv.DictReader(f):
            elevator_id    = to_int(raw.get("elevator_id"))
            risk_score     = to_numeric(raw.get("risk_score"))
            risk_level     = to_text(raw.get("risk_level"))
            model_version  = to_text(raw.get("model_version"))
            prediction_date = parse_date(raw.get("prediction_date"))

            if elevator_id not in valid_elevator_ids:
                skipped += 1
                continue
            if None in (risk_score, risk_level, model_version, prediction_date):
                skipped += 1
                skip_reasons.append(f"prediction {elevator_id}: missing required field")
                continue

            rows.append((
                elevator_id,
                risk_score,
                risk_level,
                None,   # risk_explanation populated by ML pipeline (Task 6)
                model_version,
                prediction_date,
            ))

    sql = """
        INSERT INTO predictions (
            elevator_id, risk_score, risk_level, risk_explanation,
            model_version, prediction_date
        ) VALUES %s
        ON CONFLICT (elevator_id) DO NOTHING
    """
    with conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM predictions")
        before = cur.fetchone()[0]
        if rows:
            execute_values(cur, sql, rows)
        cur.execute("SELECT COUNT(*) FROM predictions")
        inserted = cur.fetchone()[0] - before
    conn.commit()

    _print_skip_sample(skip_reasons)
    return inserted, skipped, skip_reasons


# ---------------------------------------------------------------------------
# Utilities
# ---------------------------------------------------------------------------

def _print_skip_sample(reasons, limit=5):
    for msg in reasons[:limit]:
        print(f"  WARNING: {msg}")
    if len(reasons) > limit:
        print(f"  ... {len(reasons) - limit} more warnings not shown")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    t0 = time.time()
    print("=== AND-105 Task 3: ETL — Flat files → PostgreSQL ===\n")

    conn = get_connection()
    try:
        print("[1/6] Running migration...")
        run_migration(conn)

        print("\n[2/6] Loading elevators (data/license.csv)...")
        e_ins, e_skip, _ = load_elevators(conn)
        print(f"  inserted: {e_ins:>6} | skipped: {e_skip:>6}")

        with conn.cursor() as cur:
            cur.execute("SELECT elevator_id FROM elevators")
            valid_ids = {r[0] for r in cur.fetchall()}
        print(f"  {len(valid_ids)} elevator IDs available as FK targets")

        print("\n[3/6] Loading inspections (data/inspection.csv)...")
        i_ins, i_skip, _ = load_inspections(conn, valid_ids)
        print(f"  inserted: {i_ins:>6} | skipped: {i_skip:>6}")

        print("\n[4/6] Loading incidents (data/incident.json)...")
        ic_ins, ic_skip, _ = load_incidents(conn, valid_ids)
        print(f"  inserted: {ic_ins:>6} | skipped: {ic_skip:>6}")

        print("\n[5/6] Loading alterations (data/altered.json)...")
        a_ins, a_skip, _ = load_alterations(conn, valid_ids)
        print(f"  inserted: {a_ins:>6} | skipped: {a_skip:>6}")

        print("\n[6/6] Loading predictions (data/predictions.csv)...")
        p_ins, p_skip, _ = load_predictions(conn, valid_ids)
        print(f"  inserted: {p_ins:>6} | skipped: {p_skip:>6}")

    finally:
        conn.close()

    elapsed = time.time() - t0
    total_ins  = e_ins + i_ins + ic_ins + a_ins + p_ins
    total_skip = e_skip + i_skip + ic_skip + a_skip + p_skip

    print("\n=== Summary ===")
    print(f"  {'table':<14} {'inserted':>8}  {'skipped':>8}")
    print(f"  {'-'*34}")
    print(f"  {'elevators':<14} {e_ins:>8}  {e_skip:>8}")
    print(f"  {'inspections':<14} {i_ins:>8}  {i_skip:>8}")
    print(f"  {'incidents':<14} {ic_ins:>8}  {ic_skip:>8}")
    print(f"  {'alterations':<14} {a_ins:>8}  {a_skip:>8}")
    print(f"  {'predictions':<14} {p_ins:>8}  {p_skip:>8}")
    print(f"  {'-'*34}")
    print(f"  {'TOTAL':<14} {total_ins:>8}  {total_skip:>8}")
    print(f"\n  elapsed: {elapsed:.2f}s")


if __name__ == "__main__":
    main()
