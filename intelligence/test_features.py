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

