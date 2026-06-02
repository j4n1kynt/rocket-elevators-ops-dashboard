-- AND-105 Task 2: Relational Data Model
-- Migration 001: Initial schema
-- All tables reference elevators as the central entity.

-- ---------------------------------------------------------------------
-- elevators
-- Source: data/license.csv
-- Central entity. All other tables reference this via elevator_id.
-- ---------------------------------------------------------------------
CREATE TABLE elevators (
    elevator_id             INTEGER     PRIMARY KEY,
    location                TEXT        NOT NULL,
    license_number          TEXT        NOT NULL,
    status                  TEXT        NOT NULL CHECK (status IN ('ACTIVE', 'BY REQUEST')),
    license_expiry_date     DATE,
    license_holder          TEXT,
    license_holder_address  TEXT,
    billing_customer        TEXT,
    billing_address         TEXT,
    elevator_type           TEXT
);

CREATE INDEX idx_elevators_status             ON elevators (status);
CREATE INDEX idx_elevators_license_expiry     ON elevators (license_expiry_date);

-- ---------------------------------------------------------------------
-- inspections
-- Source: data/inspection.csv
-- One elevator has many inspections over its lifetime (1:N).
-- inspection_id is the natural PK from the source (InspectionNumber).
-- ON DELETE CASCADE: inspections are meaningless without their elevator.
-- outcome is NOT NULL: verified across all 143,181 source rows.
-- ---------------------------------------------------------------------
CREATE TABLE inspections (
    inspection_id               INTEGER     PRIMARY KEY,
    elevator_id                 INTEGER     NOT NULL REFERENCES elevators (elevator_id) ON DELETE CASCADE,
    service_request_number      INTEGER,
    customer                    TEXT,
    location                    TEXT,
    inspection_type             TEXT,
    earliest_inspection_date    DATE,
    latest_inspection_date      DATE,
    outcome                     TEXT        NOT NULL
);

CREATE INDEX idx_inspections_elevator_id       ON inspections (elevator_id);
CREATE INDEX idx_inspections_latest_date       ON inspections (latest_inspection_date);
CREATE INDEX idx_inspections_outcome           ON inspections (outcome);

-- ---------------------------------------------------------------------
-- incidents
-- Source: data/incident.json
-- One elevator can have many reported incidents (1:N).
-- The many individual injury-type boolean columns from the source are
-- collapsed into injury_severity TEXT to avoid 30+ sparse boolean columns.
-- ON DELETE CASCADE: incidents are meaningless without their elevator.
-- injury_severity defaults to 'none' when no injury data is present.
-- ---------------------------------------------------------------------
CREATE TABLE incidents (
    incident_id             INTEGER     PRIMARY KEY,
    elevator_id             INTEGER     NOT NULL REFERENCES elevators (elevator_id) ON DELETE CASCADE,
    creation_date           DATE,
    date_of_occurrence      DATE,
    time_of_occurrence      TIME,
    category                TEXT,
    incident_summary        TEXT,
    root_cause              TEXT,
    narrative               TEXT,
    fatal_injury            BOOLEAN     NOT NULL DEFAULT FALSE,
    injury_severity         TEXT        NOT NULL DEFAULT 'none' CHECK (injury_severity IN ('fatal', 'permanent', 'minor', 'none')),
    task_number             INTEGER
);

CREATE INDEX idx_incidents_elevator_id         ON incidents (elevator_id);
CREATE INDEX idx_incidents_date_of_occurrence  ON incidents (date_of_occurrence);
CREATE INDEX idx_incidents_category            ON incidents (category);

-- ---------------------------------------------------------------------
-- alterations
-- Source: data/altered.json
-- One elevator can have many alteration requests (1:N).
-- No stable natural PK exists in the source; SERIAL surrogate key used.
-- service_request_number is retained as a searchable reference.
-- ON DELETE CASCADE on elevator_id: alteration records without an
--   elevator have no operational meaning.
-- ON DELETE SET NULL on inspection_number: the alteration record
--   remains valid if the linked inspection is removed.
-- ---------------------------------------------------------------------
CREATE TABLE alterations (
    alteration_id           SERIAL      PRIMARY KEY,
    service_request_number  INTEGER,
    elevator_id             INTEGER     NOT NULL REFERENCES elevators (elevator_id) ON DELETE CASCADE,
    customer                TEXT,
    summary                 TEXT,
    location                TEXT,
    alteration_type         TEXT,
    status                  TEXT,
    contractor_name         TEXT,
    billing_customer        TEXT,
    inspection_number       INTEGER     REFERENCES inspections (inspection_id) ON DELETE SET NULL
);

CREATE INDEX idx_alterations_elevator_id       ON alterations (elevator_id);
CREATE INDEX idx_alterations_status            ON alterations (status);
CREATE INDEX idx_alterations_service_request   ON alterations (service_request_number);

-- ---------------------------------------------------------------------
-- predictions
-- Source: data/predictions.csv
-- One prediction record per elevator, representing the latest model
-- output. elevator_id is both PK and FK (1:1 with elevators).
-- ON DELETE CASCADE: a prediction without its elevator is meaningless.
-- risk_explanation is required for audit and dashboard display.
-- ---------------------------------------------------------------------
CREATE TABLE predictions (
    elevator_id         INTEGER         PRIMARY KEY REFERENCES elevators (elevator_id) ON DELETE CASCADE,
    risk_score          NUMERIC(5,4)    NOT NULL,
    risk_level          TEXT            NOT NULL CHECK (risk_level IN ('LOW', 'MEDIUM', 'HIGH')),
    risk_explanation    TEXT,
    model_version       TEXT            NOT NULL,
    prediction_date     DATE            NOT NULL
);

CREATE INDEX idx_predictions_elevator_id       ON predictions (elevator_id);
CREATE INDEX idx_predictions_risk_level        ON predictions (risk_level);
CREATE INDEX idx_predictions_prediction_date   ON predictions (prediction_date);
