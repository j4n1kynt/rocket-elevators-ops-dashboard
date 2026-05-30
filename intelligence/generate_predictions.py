# CLI runner — mirrors intelligence/generate_predictions.ipynb.
# Run from project root: py -3 intelligence/generate_predictions.py
import pandas as pd
from sklearn.ensemble import RandomForestClassifier
from sklearn.impute import SimpleImputer
from sklearn.pipeline import Pipeline
from datetime import date

FEATURES = [
    "prior_inspection_count", "prior_fail_count", "prior_pass_count",
    "prior_pass_rate", "days_since_last_inspection", "prior_inspection_frequency",
    "last_inspection_outcome", "prior_order_count", "max_prior_riskscore",
    "mean_prior_riskscore", "prior_overdue_order_count", "prior_unresolved_order_count",
    "distinct_directive_count",
    "device_type_Freight_Elevator_E", "device_type_Freight_Elevator_P",
    "device_type_LULA_Elevator", "device_type_Material_Lift_ATD",
    "device_type_Observation_Elevator", "device_type_Passenger_Elevator",
    "device_type_Sidewalk_Elevator", "device_type_Special_Installation",
    "device_type_Temporary_Elevator",
    "insp_type_Followup", "insp_type_Other", "insp_type_Periodic",
]
MODEL_VERSION = "v4.1"
SPLIT_DATE    = "2015-12-14"
COL_ORDER     = ["elevator_id", "risk_score", "risk_level", "model_version", "prediction_date"]


def assign_risk_level(score: float) -> str:
    if score >= 0.7:
        return "HIGH"
    if score >= 0.4:
        return "MEDIUM"
    return "LOW"


df = pd.read_csv("data/feature_matrix.csv")
df["Latest_INSPECTION_Date"] = pd.to_datetime(df["Latest_INSPECTION_Date"])

train = df[df["Latest_INSPECTION_Date"] < SPLIT_DATE]
X_train, y_train = train[FEATURES], train["target"]

print(f"Training on {len(train):,} rows (before {SPLIT_DATE})...")
pipeline = Pipeline([
    ("imputer", SimpleImputer(strategy="median")),
    ("clf", RandomForestClassifier(
        n_estimators=100, class_weight="balanced",
        random_state=42, n_jobs=-1,
    )),
])
pipeline.fit(X_train, y_train)

latest = (
    df.sort_values("Latest_INSPECTION_Date")
      .groupby("ElevatingDevicesNumber", sort=False)
      .last()
      .reset_index()
)
proba = pipeline.predict_proba(latest[FEATURES])[:, 1]

today = date.today().isoformat()
predictions = pd.DataFrame({
    "elevator_id":     latest["ElevatingDevicesNumber"].astype(str),
    "risk_score":      proba.round(4),
    "risk_level":      [assign_risk_level(s) for s in proba],
    "model_version":   MODEL_VERSION,
    "prediction_date": today,
})

# Validation
fm_ids  = set(df["ElevatingDevicesNumber"].astype(str).unique())
missing = fm_ids - set(predictions["elevator_id"].unique())
assert len(missing) == 0, f"Missing predictions for {len(missing)} elevator(s)"
assert predictions["risk_score"].between(0, 1).all(), "risk_score out of [0, 1]"
dist = predictions["risk_level"].value_counts(normalize=True)
assert dist.max() < 0.95, f"Degenerate distribution: {dist.to_dict()}"

counts = predictions["risk_level"].value_counts()
print(f"\nTotal elevators       : {len(predictions):,}")
for level in ["HIGH", "MEDIUM", "LOW"]:
    n = counts.get(level, 0)
    print(f"  {level:<8}: {n:>6,}  ({n / len(predictions) * 100:.1f}%)")
print(f"Min / Max / Mean score: "
      f"{predictions['risk_score'].min():.4f} / "
      f"{predictions['risk_score'].max():.4f} / "
      f"{predictions['risk_score'].mean():.4f}")

predictions[COL_ORDER].to_csv("data/predictions.csv", index=False)
print(f"\nSaved → data/predictions.csv")
