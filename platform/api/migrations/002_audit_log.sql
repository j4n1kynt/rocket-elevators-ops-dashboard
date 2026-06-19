-- Migration 002: Scheduling audit log
-- Records every successful inspection scheduling action taken via the chatbot.
-- No FK constraints on elevator_id or inspection_id — audit rows are immutable
-- and must not cascade-delete when source records are removed.

CREATE TABLE IF NOT EXISTS scheduling_audit_log (
    log_id          BIGSERIAL   PRIMARY KEY,
    elevator_id     INTEGER     NOT NULL,
    inspection_id   INTEGER,
    inspection_date DATE        NOT NULL,
    inspection_type TEXT        NOT NULL,
    reason          TEXT,
    outcome         TEXT        NOT NULL CHECK (outcome IN ('success', 'error')),
    error_message   TEXT,
    performed_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_elevator_id   ON scheduling_audit_log (elevator_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_performed_at  ON scheduling_audit_log (performed_at);
