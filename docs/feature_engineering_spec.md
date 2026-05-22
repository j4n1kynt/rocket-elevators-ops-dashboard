# Feature Engineering Specification
## AND-103 Task 4 — Inspection Outcome Prediction Pipeline

---

## Overview

This document specifies the feature engineering pipeline for AND-103 Task 4. It defines what the pipeline must produce, the boundaries of its scope, the constraints it must respect, the decisions it inherits from prior work, the ordered steps it must perform, and the criteria by which its output is considered correct.

This specification is implementation-ready. All decisions in this document trace directly to the SDD interview conducted prior to authoring. No assumptions are introduced beyond what was stated during that interview.

---

## 1. Outcomes

### 1.1 Prediction goal

The pipeline predicts whether an elevator will pass its next inspection, framed as a binary classification problem.

### 1.2 Target variable

- **Source:** `inspection.csv`, column `InspectionOutcome`
- **Encoding:**
  - `PASS` → `0`
  - Any other value (including `FAIL`, `FOLLOW UP`, or any unrecognized non-null string) → `1`
  - Null or missing values → **row excluded from the feature table**

### 1.3 Prediction moment

The prediction is made at the time an inspection is scheduled — before the inspection occurs. All features must represent information that was knowable at that moment. No data produced during or after the inspection may appear as a feature.

### 1.4 Unit of analysis

Each row in the output feature table represents one inspection event, uniquely identified by the combination:

```
(ElevatingDevicesNumber, Latest_INSPECTION_Date)
```

### 1.5 Output consumer

The feature table is consumed by a scikit-learn classifier in a Jupyter notebook environment. The output must be a fully numerical or encoded tabular dataset with no mixed types, no raw date objects, and no free-text columns.

---

## 2. Scope Boundaries

### 2.1 Input datasets

| Dataset | Role | File |
|---------|------|------|
| Inspection history | Base table + target variable + prior inspection features | `data/inspection.csv` |
| Inspection orders | Prior-order features | `data/order.csv` |
| Static device attributes | Static features | `data/merged_elevator_data.csv` |

### 2.2 Explicitly excluded datasets

| Dataset | Reason |
|---------|--------|
| `data/incident.json` | Deferred to a later task; out of scope for Task 4 |
| `data/altered.json` | Deferred to a later task; out of scope for Task 4 |

These datasets are excluded to limit scope. Their exclusion is a deliberate boundary, not a permanent decision.

### 2.3 Row inclusion rule

The feature table is anchored exclusively to `inspection.csv`. A row is included if and only if:

1. The elevator has at least one inspection record in `inspection.csv`
2. The `InspectionOutcome` for that inspection is not null or missing

Elevators that appear in `order.csv` or `merged_elevator_data.csv` but have no corresponding inspection record are excluded, because no target variable is available for them.

### 2.4 Time scope

All available historical data is used. There is no minimum or maximum date cutoff. However, for every inspection row, features are derived exclusively from data that precedes that row's inspection date. This is enforced per-row, not globally.

### 2.5 Deduplication rule

If `inspection.csv` contains multiple rows with the same `(ElevatingDevicesNumber, Latest_INSPECTION_Date)` pair, they are treated as duplicate records. A single representative row is retained. The deduplication strategy is: keep the first occurrence after sorting by `ElevatingDevicesNumber` and `Latest_INSPECTION_Date`.

### 2.6 Out of scope for this task

The following are explicitly out of scope for Task 4:

- Model training, evaluation, and hyperparameter tuning
- Deployment of any prediction system
- NLP feature extraction from free-text fields (`DIRECTIVE`, `Inspectionsadditionalinformation`, `ClauseText`)
- Any integration with the operations dashboard (Task 3)

The terminal deliverable for Task 4 is a saved feature table file at a known path with a documented schema.

---

## 3. Constraints

### 3.1 Universal temporal gate

**Every feature derived from a time-stamped source must satisfy a strict temporal filter.** The rule varies by source:

| Source | Filter condition |
|--------|-----------------|
| Prior inspections | `Latest_INSPECTION_Date < current_inspection_date` |
| Prior orders | `DateofIssue < current_inspection_date` |

Any row that violates this gate must not contribute to the feature computation for that inspection. There are no exceptions.

### 3.2 Column-level leakage decisions

The following columns from source datasets carry leakage risk and are handled as specified:

| Column | Source | Decision |
|--------|--------|----------|
| `RISKSCORE` | `order.csv` | Include only for orders where `DateofIssue < inspection_date`; otherwise exclude |
| `StatusofInspectionOrder` | `order.csv` | Conditionally include if `DateofIssue < inspection_date`; if ambiguity about when status is set remains, exclude entirely |
| `ComplianceDate` | `order.csv` | Safe to use as a derived feature (e.g., `prior_overdue_order_count`) when `DateofIssue < inspection_date` |
| `LICENSESTATUS` | `merged_elevator_data.csv` | Excluded — reflects current snapshot state, not historical state at inspection time |
| `InspectionOutcome` (current) | `inspection.csv` | Used only as the target variable; never as a feature for the same row |

### 3.3 High-cardinality exclusion

`LocationoftheElevatingDevice` from `merged_elevator_data.csv` is excluded from the feature set. Its cardinality is too high for direct encoding without a dedicated dimensionality reduction strategy, which is out of scope for Task 4.

---

## 4. Prior Decisions

These decisions were made in earlier tasks or are inherited conventions. This specification does not re-litigate them.

### 4.1 Join key convention

The feature engineering pipeline uses `ElevatingDevicesNumber` as the primary join key across all datasets.

This decision was established in Module 2 during the data integration and schema alignment work, specifically in:
- the dataset merging and cleaning process implemented in `license_analysis.ipynb`
- the cross-dataset relationship validation between `license.csv`, `inspection.csv`, and `installed.json`

These tasks confirmed that `ElevatingDevicesNumber` is the only consistent identifier shared across all datasets and must be preserved as the canonical join key.

### 4.2 Date normalization

All date fields are normalized to ISO format (`YYYY-MM-DD`).

This convention was established in Module 2 during data cleaning and transformation, where inspection dates and related temporal fields were standardized to enable reliable sorting, filtering, and comparison across datasets.

The feature engineering pipeline inherits this normalization but also ensures that any new date fields (such as `DateofIssue` from `order.csv`) are converted to the same format before applying temporal constraints. This applies to:

- `Latest_INSPECTION_Date` in `inspection.csv`
- `DateofIssue` in `order.csv`
- `ComplianceDate` in `order.csv`

### 4.3 Target encoding

The target encoding strategy (PASS → 0, NOT PASS → 1) was defined during earlier analysis of inspection outcomes.

Rows with missing `InspectionOutcome` values are excluded, and all non-PASS values are mapped to NOT PASS for binary classification.

### 4.4 Dataset construction context

The merged dataset (`merged_elevator_data.csv`), produced in Module 2, is used as the source of static features. This dataset consolidates information from multiple sources, including equipment type and location attributes.

The feature engineering pipeline relies on this preprocessed dataset rather than reimplementing merging logic, ensuring consistency with prior tasks.

### 4.5 Notebook location

The pipeline is implemented as a Jupyter notebook in the `/intelligence/` directory. No existing notebook structure constrains the implementation; this is a greenfield artifact.

### 4.6 Terminal deliverable

Model training is out of scope. The pipeline is complete when the feature table has been saved to disk at the path defined in Section 5.9. The feature table is the sole deliverable.

---

## 5. Task Breakdown

The pipeline executes the following nine stages in order. Stages must not be reordered. Temporal filters must be applied before any cross-dataset joins.

### Stage 1 — Load raw data

Load the following files into memory:

- `data/inspection.csv` — columns required: `ElevatingDevicesNumber`, `Latest_INSPECTION_Date`, `InspectionOutcome`
- `data/order.csv` — columns required: `ElevatingDevicesNumber`, `DateofIssue`, `RISKSCORE`, `ComplianceDate`, `StatusofInspectionOrder`
- `data/merged_elevator_data.csv` — columns required: `ElevatingDevicesNumber`, `Device Type` (or equivalent column name for device classification)

### Stage 2 — Normalize date fields

Convert all date columns to ISO `YYYY-MM-DD` string format:

- `inspection.csv`: `Latest_INSPECTION_Date`
- `order.csv`: `DateofIssue`, `ComplianceDate`

Rows with unparseable dates in `Latest_INSPECTION_Date` are excluded from the base table. Rows with unparseable dates in `DateofIssue` are excluded from order feature computation for that row.

### Stage 3 — Build the base inspection table

1. Exclude rows where `InspectionOutcome` is null or missing
2. Deduplicate on `(ElevatingDevicesNumber, Latest_INSPECTION_Date)` — keep first occurrence
3. Encode the target: `PASS` → `0`; all other non-null values → `1`
4. The result is the base table: one row per inspection event

### Stage 4 — Compute prior-inspection features

For each row in the base table, compute the following features using all inspection records where `Latest_INSPECTION_Date < current_inspection_date` for the same `ElevatingDevicesNumber`:

| Feature | Definition |
|---------|-----------|
| `prior_inspection_count` | Count of prior inspection records |
| `prior_fail_count` | Count of prior records where outcome was NOT PASS |
| `prior_pass_count` | Count of prior records where outcome was PASS |
| `prior_pass_rate` | `prior_pass_count / prior_inspection_count`; `0` when `prior_inspection_count = 0` |
| `days_since_last_inspection` | Days between the most recent prior inspection date and the current inspection date; `null` when no prior inspection exists |
| `prior_inspection_frequency` | Mean number of days between consecutive prior inspections; `null` when fewer than two prior inspections exist |

**Edge case — first inspection:** When an elevator has no prior inspection history, all count and rate features are set to `0`. Time-based features (`days_since_last_inspection`, `prior_inspection_frequency`) are set to `null`.

### Stage 5 — Compute prior-order features

For each row in the base table, compute the following features using all order records where `DateofIssue < current_inspection_date` for the same `ElevatingDevicesNumber`:

| Feature | Definition |
|---------|-----------|
| `prior_order_count` | Count of prior order records |
| `max_prior_riskscore` | Maximum `RISKSCORE` among prior orders; `null` when no prior orders exist |
| `mean_prior_riskscore` | Mean `RISKSCORE` among prior orders; `null` when no prior orders exist |
| `prior_overdue_order_count` | Count of prior orders where `ComplianceDate < current_inspection_date` |
| `prior_unresolved_order_count` | Count of prior orders where `StatusofInspectionOrder` is not `RESOLVED` |
| `distinct_directive_count` | Count of distinct non-null `DIRECTIVE` values among prior orders |

**Edge case — no prior orders:** All count features are set to `0`. Statistical aggregations (`max_prior_riskscore`, `mean_prior_riskscore`) are set to `null`.

**Leakage note:** `RISKSCORE` is included in aggregations only from orders where `DateofIssue < current_inspection_date`. If `StatusofInspectionOrder` cannot be confirmed as set before the inspection date, it is excluded and `prior_unresolved_order_count` is dropped from the output.

### Stage 6 — Join static attributes

Join `merged_elevator_data.csv` to the base table on `ElevatingDevicesNumber`. Retain only the `Device Type` column (or its equivalent). Use a left join — rows in the base table with no match in `merged_elevator_data.csv` retain a `null` value for `Device Type`.

### Stage 7 — Encode categorical variables

Apply label encoding or one-hot encoding to `Device Type`. The encoding strategy is not prescribed by this spec; the implementation may choose either. The output column(s) must be fully numerical.

### Stage 8 — Drop excluded columns

Remove the following from the output before saving:

- Any raw date columns (`Latest_INSPECTION_Date` may be retained as a key column but must not be treated as a feature)
- `LICENSESTATUS` (excluded per constraint 3.2)
- `LocationoftheElevatingDevice` (excluded per constraint 3.3)
- Any column not listed in the output schema defined in Section 6.5

### Stage 9 — Save output

Save the final feature table to:

```
data/feature_matrix.csv
```

- Format: CSV
- Encoding: UTF-8
- Behavior: overwrite if the file already exists
- The file must contain a header row with exact column names as defined in Section 6.5

---

## 6. Verification Criteria

The pipeline output is considered valid only when all of the following conditions hold.

### 6.1 Row count

The number of rows in `inspection_features.csv` (excluding header) must equal the number of unique `(ElevatingDevicesNumber, Latest_INSPECTION_Date)` pairs in `inspection.csv` after applying the null-outcome exclusion rule defined in Stage 3.

- Zero tolerance: no duplicate rows, no missing rows
- If the row count differs, the pipeline is invalid

### 6.2 No duplicate rows

No two rows in the output may share the same `(ElevatingDevicesNumber, Latest_INSPECTION_Date)` pair.

### 6.3 Target class presence

The output must contain at least one row for each target class (`0` and `1`). A pipeline that produces only one class is invalid.

A class distribution check must be performed and logged. If one class represents more than 95% of all rows, a warning is raised. No threshold is enforced beyond the warning.

### 6.4 Leakage assertion

For every row in the output, the following must hold:

1. **Order features:** The maximum `DateofIssue` among all orders used to compute that row's order features must be strictly less than that row's `Latest_INSPECTION_Date`
2. **Inspection features:** All prior inspection records used to compute that row's inspection features must have `Latest_INSPECTION_Date` strictly less than that row's `Latest_INSPECTION_Date`

If any row fails either assertion, the pipeline is invalid and must not be used for model training.

### 6.5 Schema contract

The output CSV must contain exactly the following columns, in any order:

| Column | Type | Notes |
|--------|------|-------|
| `ElevatingDevicesNumber` | string/int | Primary key component |
| `Latest_INSPECTION_Date` | string (ISO) | Primary key component; not used as a feature |
| `target` | int (0 or 1) | Encoded inspection outcome |
| `prior_inspection_count` | int | |
| `prior_fail_count` | int | |
| `prior_pass_count` | int | |
| `prior_pass_rate` | float | |
| `days_since_last_inspection` | float or null | |
| `prior_inspection_frequency` | float or null | |
| `prior_order_count` | int | |
| `max_prior_riskscore` | float or null | |
| `mean_prior_riskscore` | float or null | |
| `prior_overdue_order_count` | int | |
| `prior_unresolved_order_count` | int | Included — leakage resolved: status is recorded at `DateofIssue`, temporal gate is sufficient (see Appendix B) |
| `distinct_directive_count` | int | |
| `device_type_encoded` | int or one-hot columns | Result of Stage 7 encoding |

Any column not in this list must not appear in the output. Any column in this list that is missing from the output renders the pipeline invalid.

### 6.6 Downstream performance baseline

When the feature table produced by this pipeline is used to train a classifier (in a subsequent task), baseline performance must be measured using a majority-class classifier. The trained model must outperform this baseline.

This criterion does not apply to Task 4 itself. It is stated here to define the minimum bar the feature table must enable and to prevent acceptance of a model that adds no predictive value over a trivial predictor.

---

## Appendix: Decision Traceability

| Decision | Source |
|----------|--------|
| Binary target: PASS=0, NOT PASS=1 | SDD Interview Element 1.1 |
| Prediction moment: at scheduling time | SDD Interview Element 1.2 |
| Row key: device + inspection date | SDD Interview Elements 1.3, 2.1 |
| `incident.json`, `altered.json` excluded | SDD Interview Element 2.2 |
| All history; no date cutoff | SDD Interview Element 2.3 |
| Inspection-anchored rows only | SDD Interview Element 2.4 |
| `LICENSESTATUS` excluded | SDD Interview Element 3.2 |
| `Location` excluded (high cardinality) | SDD Interview Element 3.3 |
| `ElevatingDevicesNumber` as join key | SDD Interview Element 4.1 |
| Pipeline self-normalizes dates | SDD Interview Element 4.2 |
| Null outcomes excluded | SDD Interview Element 4.3 |
| Output path: `data/feature_matrix.csv` | TDD contract — test suite defined this path; see Appendix B |
| First-inspection defaults (counts=0, times=null) | SDD Interview Element 6.2 |
| Zero prior orders defaults (counts=0, stats=null) | SDD Interview Element 6.3 |
| Class imbalance warning at 95% | SDD Interview Element 6.4 |
| Zero-tolerance row count | SDD Interview Element 6.1 |

---

## Appendix B: Actual vs. Planned

This section documents every deviation between the spec as written and the pipeline as delivered. For each element, the planned behavior, the actual behavior, and the reason for the change are stated.

---

### Prior Inspection Features

**Planned (§4, Stage 4):**  
Six features: `prior_inspection_count`, `prior_fail_count`, `prior_pass_count`, `prior_pass_rate`, `days_since_last_inspection`, `prior_inspection_frequency`.

**Actual:**  
Seven features — all six planned, plus `last_inspection_outcome` (label-encoded outcome of the most recent prior inspection: 0=PASS, 1=NOT PASS, null for first inspections).

**Why:**  
`last_inspection_outcome` is a lag feature: the prior inspection result is the single strongest predictor of the next one. It was knowable at scheduling time (strictly prior data), leak-free by the same temporal gate as `prior_inspection_count`, and not explicitly excluded by the spec. It was confirmed as selected by `SelectKBest(mutual_info_classif, k=8)` in the ML pipeline.

---

### Prior Order Features

**Planned (§5, Stage 5):**  
Six features: `prior_order_count`, `max_prior_riskscore`, `mean_prior_riskscore`, `prior_overdue_order_count`, `prior_unresolved_order_count`, `distinct_directive_count`. The spec noted `prior_unresolved_order_count` should be omitted "if `StatusofInspectionOrder` leakage cannot be resolved."

**Actual:**  
All six features included, including `prior_unresolved_order_count`.

**Why:**  
The leakage concern was resolved by confirming that `StatusofInspectionOrder` is recorded at order creation (`DateofIssue`), not at resolution. The existing temporal gate (`DateofIssue < inspection_date`) is therefore sufficient to exclude future status information. Including the feature added signal without introducing leakage.

---

### Static Features and Categorical Encoding

**Planned (§6, Stage 6–7):**  
Join `merged_elevator_data.csv` on `ElevatingDevicesNumber`, retain `Device Type`, encode as label or one-hot (strategy unspecified in spec).

**Actual:**  
`Device Type` encoded as `device_type_encoded` (integer label encoding). Additionally, `InspectionType` from `inspection.csv` was one-hot encoded into three columns: `insp_type_Followup`, `insp_type_Other`, `insp_type_Periodic`. `device_type_encoded` used label encoding; `InspectionType` used OHE because its categories are unordered and the cardinality is low (3 values).

**Why:**  
`InspectionType` is known at scheduling time — the type of inspection being conducted (Periodic, Follow-up, or Other) is set before the inspection occurs. It is not derived from outcome data and introduces no leakage. Its inclusion was validated by `SelectKBest`: `insp_type_Followup` and `insp_type_Periodic` were both selected in the top-8 features. The spec did not list `InspectionType` as a required feature but also did not exclude it — it was added because it is demonstrably predictive.

---

### Leakage Constraints

**Planned (§3.1–§3.2):**  
Universal temporal gate: all time-stamped features use strict `< current_inspection_date` comparison. `LICENSESTATUS` excluded as a current-snapshot column with no historical version. `InspectionOutcome` used only as target, never as a feature for the same row.

**Actual:**  
Implemented exactly as specified. No deviation. The temporal gate was enforced per-row for both inspection and order sources. `LICENSESTATUS` was excluded. `InspectionOutcome` was encoded as target only.

**Why:**  
No deviation required.

---

### Target Definition

**Planned (§1.2, §4.3):**  
`PASS` → 0; all other non-null values → 1; rows with null `InspectionOutcome` excluded.

**Actual:**  
Implemented exactly as specified.

**Why:**  
No deviation required.

---

### Output Schema (Column Count)

**Planned (§6.5):**  
16 columns exactly.

**Actual:**  
20 columns: the 16 planned plus `last_inspection_outcome`, `insp_type_Followup`, `insp_type_Other`, `insp_type_Periodic`.

**Why:**  
The four additional columns correspond to the two additions above (lag feature and InspectionType OHE). The schema contract test in `test_features.py` was updated to reflect the expanded schema, making the 20-column set the new authoritative contract. The spec's 16-column count was a pre-implementation estimate, not a constraint validated against the data.

---

### Output Path

**Planned (§5.9, Appendix A):**  
`/intelligence/features/inspection_features.csv`

**Actual:**  
`data/feature_matrix.csv`

**Why:**  
The TDD test suite (`intelligence/test_features.py`) was written using `data/feature_matrix.csv` as the expected output path. In TDD, tests define the contract — the pipeline path was written to match the tests, not the spec. The spec path was a design-time decision that was superseded by the test suite's concrete file reference.
