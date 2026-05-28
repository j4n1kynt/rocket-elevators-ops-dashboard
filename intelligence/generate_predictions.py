# Run from project root: py -3 intelligence/generate_predictions.py
import pandas as pd
import numpy as np
from sklearn.ensemble import RandomForestClassifier
from sklearn.impute import SimpleImputer
from sklearn.pipeline import Pipeline
from datetime import date

FEATURES = [
    "prior_inspection_count", "prior_fail_count", "prior_pass_count",
    "prior_pass_rate", "days_since_last_inspection", "prior_inspection_frequency",
    "last_inspection_outcome", "prior_order_count", "max_prior_riskscore",
    "mean_prior_riskscore", "prior_overdue_order_count", "prior_unresolved_order_count",
    "distinct_directive_count", "device_type_encoded",
    "insp_type_Followup", "insp_type_Other", "insp_type_Periodic",
]
MODEL_VERSION = "random_forest_v1"
SPLIT_DATE    = "2015-12-14"


def risk_level(score: float) -> str:
    if score >= 0.7:
        return "HIGH"
    if score >= 0.4:
        return "MEDIUM"
    return "LOW"


df = pd.read_csv("data/feature_matrix.csv")
df["Latest_INSPECTION_Date"] = pd.to_datetime(df["Latest_INSPECTION_Date"])

# Temporal train/test split — identical to ml_pipeline.ipynb
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
print("Model trained.")

# Latest row per elevator = current feature snapshot
latest = (
    df.sort_values("Latest_INSPECTION_Date")
      .groupby("ElevatingDevicesNumber", sort=False)
      .last()
      .reset_index()
)
proba = pipeline.predict_proba(latest[FEATURES])[:, 1]  # P(NOT PASS)

today = date.today().isoformat()
out = pd.DataFrame({
    "elevator_id":            latest["ElevatingDevicesNumber"].astype(str),
    "risk_score":             proba.round(4),
    "risk_level":             [risk_level(s) for s in proba],
    "predicted_failure_date": "",  # not modeled; Go API treats blank as null
    "confidence":             np.maximum(proba, 1 - proba).round(4),
    "model_version":          MODEL_VERSION,
    "generated_at":           today,
})

# Validation
assert out["risk_score"].between(0, 1).all(), "risk_score out of [0, 1]"
print(f"\nPredictions generated : {len(out):,} elevators")
print(f"Score range           : {out['risk_score'].min():.4f} – {out['risk_score'].max():.4f}")
print(f"Risk distribution     :\n{out['risk_level'].value_counts().to_string()}")

out.to_csv("data/predictions.csv", index=False)
print("\nSaved → data/predictions.csv")
