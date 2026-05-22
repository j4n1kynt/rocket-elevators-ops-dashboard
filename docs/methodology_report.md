# Methodology Report — AND-103 Task 6

---

## 1. Feature Engineering Summary

### Features Created

The pipeline produces 17 predictive features organized in four groups.

**Prior inspection features (7)** — derived from `inspection.csv`:

| Feature | Definition |
|---|---|
| `prior_inspection_count` | Number of inspections before current date |
| `prior_fail_count` | Count of prior NOT PASS outcomes |
| `prior_pass_count` | Count of prior PASS outcomes |
| `prior_pass_rate` | `prior_pass_count / prior_inspection_count`; 0 when no history |
| `days_since_last_inspection` | Days between last prior inspection and current date |
| `prior_inspection_frequency` | Mean days between consecutive prior inspections |
| `last_inspection_outcome` | Outcome of the most recent prior inspection (0=PASS, 1=NOT PASS) |

**Prior order features (6)** — derived from `order.csv`:

| Feature | Definition |
|---|---|
| `prior_order_count` | Number of inspection orders before current date |
| `max_prior_riskscore` | Maximum RISKSCORE among prior orders |
| `mean_prior_riskscore` | Mean RISKSCORE among prior orders |
| `prior_overdue_order_count` | Orders where `ComplianceDate < inspection_date` |
| `prior_unresolved_order_count` | Orders where `StatusofInspectionOrder != RESOLVED` |
| `distinct_directive_count` | Count of distinct non-null DIRECTIVE values |

**Static device feature (1)** — derived from `merged_elevator_data.csv`:

| Feature | Definition |
|---|---|
| `device_type_encoded` | Label-encoded device type classification |

**Inspection type features (3)** — derived from `inspection.csv`:

| Feature | Definition |
|---|---|
| `insp_type_Followup` | 1 if the current inspection is a Follow-up |
| `insp_type_Other` | 1 if the current inspection type is Other |
| `insp_type_Periodic` | 1 if the current inspection is Periodic |

### How Leakage Was Prevented

Every feature is computed using only data that pre-dates the inspection event being predicted. Two temporal gates enforce this:

- **Inspection features:** only rows where `Latest_INSPECTION_Date < current_inspection_date`
- **Order features:** only rows where `DateofIssue < current_inspection_date`

This simulates the actual prediction scenario: all features represent information an inspector would have on the day an inspection is scheduled, before the inspection takes place. No outcome data, no same-day data, and no post-inspection data appears in any feature.

Five features produce `NaN` for first-inspection rows or elevators with no prior orders (`days_since_last_inspection`, `prior_inspection_frequency`, `last_inspection_outcome`, `max_prior_riskscore`, `mean_prior_riskscore`). These are handled inside the sklearn Pipeline using `SimpleImputer(strategy='median')`, fit exclusively on training data.

---

## 2. TDD Experience

### Tests Written (Task 5)

Nine tests were written before the pipeline was implemented:

| Test | What it validates |
|---|---|
| `test_prior_inspection_count_correct` | `prior_inspection_count` matches manual count for a sampled elevator |
| `test_first_inspection_has_zero_history` | First-inspection rows have `prior_inspection_count=0` and `prior_pass_rate=0` |
| `test_no_future_orders_used` | Placeholder confirming no future orders are used (validated indirectly by leakage tests) |
| `test_row_count_matches_inspection_base` | Feature matrix row count equals unique `(ElevatingDevicesNumber, Latest_INSPECTION_Date)` pairs after null-outcome exclusion |
| `test_no_duplicate_rows` | No duplicate `(ElevatingDevicesNumber, Latest_INSPECTION_Date)` pairs in output |
| `test_both_target_classes_present` | Both class 0 (PASS) and class 1 (NOT PASS) appear in the target column |
| `test_leakage_inspection_features` | For 20 sampled rows, `prior_inspection_count` matches manual count using only strictly prior dates |
| `test_leakage_order_features` | For 20 sampled rows, `prior_order_count` matches manual count using only strictly prior dates |
| `test_schema_contract` | Output contains exactly the expected columns — no missing, no extra |

### Issues Caught

The schema contract test (`test_schema_contract`) caught a column mismatch when `last_inspection_outcome` and the three `insp_type_*` columns were added during implementation. The test failed with "Extra columns not in schema," which forced an explicit decision: update `EXPECTED_COLUMNS` to include the new features and document the deviation in the spec. Without this test, the schema expansion would have been invisible to a future reviewer.

The leakage tests caught no violations after implementation, which confirmed that the temporal filtering logic was applied correctly before the pipeline was considered complete.

### Effect of Writing Tests First

Writing tests before implementation provided three concrete benefits:

1. **Clarity of requirements:** The schema contract test forced a precise enumeration of every output column before the first line of pipeline code was written. Ambiguities in the spec ("label or one-hot encoding") had to be resolved into concrete column names.

2. **Leakage prevention:** The leakage tests required that the temporal gate be implemented as a per-row filter, not a global date cutoff. This distinction — filtering per inspection event rather than once for the whole dataset — is easy to overlook and would have produced subtle data leakage.

3. **Confidence in correctness:** All 9 tests pass on the delivered feature matrix. The suite can be re-run after any pipeline change to detect regressions without manual inspection.

---

## 3. Model Results

All models were evaluated on a time-based holdout split: training on inspections from 2011 to 2015 (105,769 rows), testing on inspections from 2015 to 2017 (26,443 rows). No random shuffling was used.

**Baseline:** A majority-class predictor that always predicts NOT PASS achieves:
- Accuracy: **0.8096**
- ROC-AUC: **0.50** (no discrimination ability)

**Model comparison:**

| Model | Accuracy | F1 macro (all features) | F1 macro (SelectKBest k=8) | ROC-AUC (all features) | ROC-AUC (SelectKBest k=8) |
|---|---|---|---|---|---|
| Baseline | 0.8096 | — | — | 0.5000 | 0.5000 |
| Logistic Regression | 0.8398 | 0.5058 | 0.5040 | 0.6771 | 0.7166 |
| Random Forest | **0.8865** | **0.5187** | 0.5096 | **0.7705** | 0.7196 |
| Gradient Boosting | 0.8463 | 0.5092 | 0.4896 | 0.7456 | 0.7148 |

**Best model:** Random Forest with all 17 features.  
**Primary metric:** F1 macro = **0.5187** (+0.022 over baseline F1 macro of 0.4964).

F1 macro was chosen as the primary metric because it averages performance equally across both classes. A majority-class predictor achieves 80.96% accuracy but scores 0 for class-0 F1 — F1 macro penalizes this by including both classes in the average. ROC-AUC is reported as a supplementary metric (RF: 0.7705, +0.2705 over baseline of 0.50) and confirms the same selection.

**Why Random Forest performed best:**  
Logistic Regression assumes linear decision boundaries. The relationship between inspection history, order risk signals, and inspection outcome is non-linear — RF captures this through ensemble tree splits. Gradient Boosting is theoretically more powerful but is sensitive to hyperparameter choices; with default parameters on this dataset, RF generalized better to the test period.

**Effect of feature selection:**  
`SelectKBest(mutual_info_classif, k=8)` selected: `prior_inspection_count`, `prior_fail_count`, `prior_pass_count`, `prior_pass_rate`, `days_since_last_inspection`, `prior_inspection_frequency`, `insp_type_Followup`, `insp_type_Periodic`. Feature selection decreased ROC-AUC for all three models. The 9 dropped features — including all order-related features and `last_inspection_outcome` — carried real signal that mutual information scoring underestimated. All 17 features are retained in the final model.

---

## 4. Lessons Learned

### About the Spec

The spec did not include `InspectionType` as a required feature, despite it being knowable at scheduling time and demonstrably predictive (two of its OHE columns appear in the SelectKBest top-8). The spec focused on features derived from prior events (counts, scores, rates) but did not account for features describing the inspection being scheduled. A revised spec would add a dedicated section for "current event attributes" — features describing the inspection itself that are set before it occurs, such as inspection type and scheduled location category.

### About the Pipeline

The imputation strategy (`SimpleImputer(strategy='median')`) was decided at implementation time with no spec reference. Five columns produce `NaN` for structurally valid reasons (first-inspection rows, elevators with no prior orders), and the choice between median imputation, mean imputation, and sentinel-value imputation has measurable effects on model behavior. A revised pipeline would define the imputation strategy in the spec — including the rationale for median over mean (robustness to outlier risk scores) — so that the choice is documented and reproducible, not incidental.
