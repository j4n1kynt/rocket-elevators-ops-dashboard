-- Migration 003: duplicate pending-inspection guard
-- Prevents a confirmed scheduling action from creating a second identical
-- pending inspection for the same elevator, date, and type (double-submit /
-- replay protection — see docs/schedule_inspection.md §6).
--
-- Scoped to inspection_id >= 9,000,000 so it only constrains chatbot-created
-- rows (mcp_inspection_id_seq starts at 9,000,000) and can never conflict with
-- imported source data, which may legitimately contain duplicate pending rows.

CREATE UNIQUE INDEX IF NOT EXISTS uq_pending_inspection
    ON inspections (elevator_id, earliest_inspection_date, inspection_type)
    WHERE outcome = 'Pending' AND inspection_id >= 9000000;
