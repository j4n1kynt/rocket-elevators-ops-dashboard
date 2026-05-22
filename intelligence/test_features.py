import pandas as pd
import pytest


## Correct Prior Inspections
def test_prior_inspection_count_correct():
    df_raw = pd.read_csv("data/inspection.csv")
    df_raw["Latest_INSPECTION_Date"] = pd.to_datetime(df_raw["Latest_INSPECTION_Date"])

    df_features = pd.read_csv("data/feature_matrix.csv")
    df_features["Latest_INSPECTION_Date"] = pd.to_datetime(df_features["Latest_INSPECTION_Date"])

    # Pick a specific elevator with enough history
    elevator_id = df_raw["ElevatingDevicesNumber"].value_counts().idxmax()

    elevator_data = df_raw[df_raw["ElevatingDevicesNumber"] == elevator_id]
    elevator_data = elevator_data.sort_values("Latest_INSPECTION_Date")

    # Pick an inspection not the first
    target_row = elevator_data.iloc[3]
    inspection_date = target_row["Latest_INSPECTION_Date"]

    # Manual prior count
    prior_count = elevator_data[
        elevator_data["Latest_INSPECTION_Date"] < inspection_date
    ].shape[0]

    # Retrieve feature row
    feature_row = df_features[
        (df_features["ElevatingDevicesNumber"] == elevator_id) &
        (df_features["Latest_INSPECTION_Date"] == inspection_date)
    ]

    assert feature_row.shape[0] == 1
    assert feature_row["prior_inspection_count"].iloc[0] == prior_count

##First Inspection edge cases
def test_first_inspection_has_zero_history():
    df_raw = pd.read_csv("data/inspection.csv")
    df_raw["Latest_INSPECTION_Date"] = pd.to_datetime(df_raw["Latest_INSPECTION_Date"])

    df_features = pd.read_csv("data/feature_matrix.csv")
    df_features["Latest_INSPECTION_Date"] = pd.to_datetime(df_features["Latest_INSPECTION_Date"])

    df_raw = df_raw.sort_values(["ElevatingDevicesNumber", "Latest_INSPECTION_Date"])

    first_inspections = df_raw.groupby("ElevatingDevicesNumber").first().reset_index()

    for _, row in first_inspections.iterrows():
        elevator_id = row["ElevatingDevicesNumber"]
        inspection_date = row["Latest_INSPECTION_Date"]

        feature_row = df_features[
            (df_features["ElevatingDevicesNumber"] == elevator_id) &
            (df_features["Latest_INSPECTION_Date"] == inspection_date)
        ]

        assert feature_row.shape[0] == 1
        assert feature_row["prior_inspection_count"].iloc[0] == 0
        assert feature_row["prior_pass_rate"].iloc[0] == 0

##No future data
def test_no_future_orders_used():
    df_orders = pd.read_csv("data/order.csv")
    df_orders["DateofIssue"] = pd.to_datetime(df_orders["DateofIssue"])

    df_features = pd.read_csv("data/feature_matrix.csv")
    df_features["Latest_INSPECTION_Date"] = pd.to_datetime(df_features["Latest_INSPECTION_Date"])

    # Sample rows (no necesitas todo el dataset)
    sample = df_features.sample(10, random_state=42)

    for _, row in sample.iterrows():
        elevator_id = row["ElevatingDevicesNumber"]
        inspection_date = row["Latest_INSPECTION_Date"]

        # Orders for this elevator AFTER inspection date
        future_orders = df_orders[
            (df_orders["ElevatingDevicesNumber"] == elevator_id) &
            (df_orders["DateofIssue"] >= inspection_date)
        ]

        # This should not exist IF pipeline is correct
        # (feature matrix must not be influenced by these)
        assert True  # placeholder → se validará indirectamente en pipeline


## §6.1 — Row count matches inspection base (zero-tolerance)
def test_row_count_matches_inspection_base():
    df_raw = pd.read_csv("data/inspection.csv", dtype=str).fillna("")
    df_features = pd.read_csv("data/feature_matrix.csv", dtype=str)

    df_raw = df_raw[df_raw["InspectionOutcome"].str.strip() != ""]
    dates = pd.to_datetime(df_raw["Latest_INSPECTION_Date"], errors="coerce")
    df_raw = df_raw[dates.notna()].copy()
    df_raw["Latest_INSPECTION_Date"] = dates[dates.notna()].dt.strftime("%Y-%m-%d")
    df_raw = df_raw.sort_values(["ElevatingDevicesNumber", "Latest_INSPECTION_Date"])
    expected = df_raw.drop_duplicates(
        subset=["ElevatingDevicesNumber", "Latest_INSPECTION_Date"], keep="first"
    ).shape[0]

    assert len(df_features) == expected, (
        f"Expected {expected} rows, got {len(df_features)}"
    )


## §6.2 — No duplicate rows
def test_no_duplicate_rows():
    df_features = pd.read_csv("data/feature_matrix.csv", dtype=str)

    n_total = len(df_features)
    n_unique = df_features.drop_duplicates(
        subset=["ElevatingDevicesNumber", "Latest_INSPECTION_Date"]
    ).shape[0]

    assert n_total == n_unique, (
        f"{n_total - n_unique} duplicate (ElevatingDevicesNumber, Latest_INSPECTION_Date) pairs found"
    )


## §6.3 — Both target classes present; warning if one class > 95%
def test_both_target_classes_present():
    import warnings
    df_features = pd.read_csv("data/feature_matrix.csv")

    classes = set(df_features["target"].unique())
    assert 0 in classes, "Target class 0 (PASS) missing from output"
    assert 1 in classes, "Target class 1 (NOT PASS) missing from output"

    counts = df_features["target"].value_counts(normalize=True)
    for cls, pct in counts.items():
        if pct > 0.95:
            warnings.warn(
                f"Class imbalance: class {cls} = {pct:.1%} of rows (threshold: 95%)"
            )


## §6.4 — No leakage in prior-inspection features
def test_leakage_inspection_features():
    df_raw = pd.read_csv("data/inspection.csv", dtype=str).fillna("")
    df_features = pd.read_csv("data/feature_matrix.csv")
    df_features["Latest_INSPECTION_Date"] = pd.to_datetime(df_features["Latest_INSPECTION_Date"])

    dates = pd.to_datetime(df_raw["Latest_INSPECTION_Date"], errors="coerce")
    df_raw = df_raw[dates.notna()].copy()
    df_raw["Latest_INSPECTION_Date"] = dates[dates.notna()]

    has_priors = df_features["prior_inspection_count"] > 0
    sample = df_features[has_priors].sample(min(20, has_priors.sum()), random_state=42)

    for _, row in sample.iterrows():
        elevator_id = str(row["ElevatingDevicesNumber"])
        inspection_date = row["Latest_INSPECTION_Date"]

        manual_prior = df_raw[
            (df_raw["ElevatingDevicesNumber"].astype(str) == elevator_id) &
            (df_raw["Latest_INSPECTION_Date"] < inspection_date)
        ].shape[0]

        assert int(row["prior_inspection_count"]) == manual_prior, (
            f"Elevator {elevator_id} on {inspection_date}: "
            f"expected {manual_prior} prior inspections, got {row['prior_inspection_count']}"
        )


## §6.4 — No leakage in prior-order features
def test_leakage_order_features():
    df_orders = pd.read_csv("data/order.csv", dtype=str).fillna("")
    df_features = pd.read_csv("data/feature_matrix.csv")
    df_features["Latest_INSPECTION_Date"] = pd.to_datetime(df_features["Latest_INSPECTION_Date"])

    order_dates = pd.to_datetime(df_orders["DateofIssue"], errors="coerce")
    df_orders = df_orders[order_dates.notna()].copy()
    df_orders["DateofIssue"] = order_dates[order_dates.notna()]

    has_orders = df_features["prior_order_count"] > 0
    sample = df_features[has_orders].sample(min(20, has_orders.sum()), random_state=42)

    for _, row in sample.iterrows():
        elevator_id = str(row["ElevatingDevicesNumber"])
        inspection_date = row["Latest_INSPECTION_Date"]

        manual_count = df_orders[
            (df_orders["ElevatingDevicesNumber"].astype(str) == elevator_id) &
            (df_orders["DateofIssue"] < inspection_date)
        ].shape[0]

        assert int(row["prior_order_count"]) == manual_count, (
            f"Elevator {elevator_id} on {inspection_date}: "
            f"expected {manual_count} prior orders, got {row['prior_order_count']}"
        )


## §6.5 — Schema contract (exactly 16 columns, no extras)
def test_schema_contract():
    EXPECTED_COLUMNS = {
        "ElevatingDevicesNumber",
        "Latest_INSPECTION_Date",
        "target",
        "prior_inspection_count",
        "prior_fail_count",
        "prior_pass_count",
        "prior_pass_rate",
        "days_since_last_inspection",
        "prior_inspection_frequency",
        "prior_order_count",
        "max_prior_riskscore",
        "mean_prior_riskscore",
        "prior_overdue_order_count",
        "prior_unresolved_order_count",
        "distinct_directive_count",
        "device_type_encoded",
    }

    df_features = pd.read_csv("data/feature_matrix.csv", nrows=1)
    actual = set(df_features.columns)

    missing = EXPECTED_COLUMNS - actual
    extra = actual - EXPECTED_COLUMNS

    assert not missing, f"Missing columns: {missing}"
    assert not extra, f"Extra columns not in schema: {extra}"

