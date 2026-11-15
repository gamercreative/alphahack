-- Structured offers extracted from a pharmacist's free-text reply.
-- The agent (or requester staff) fills the fields with quoted evidence; Go rules
-- (services/enq_offers.go) decide matches | escalated | rejected. An offer never
-- changes a target's status or closes an enquiry (CONTRACT rules 1, 2 and 6).
CREATE TABLE target_offers (
    id                 TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    enquiry_id         TEXT        NOT NULL REFERENCES enquiries(id) ON DELETE CASCADE,
    target_id          TEXT        NOT NULL REFERENCES enquiry_targets(id) ON DELETE CASCADE,
    branch_id          TEXT        NOT NULL REFERENCES branches(id),
    source_text        TEXT        NOT NULL,
    extracted_by       TEXT        NOT NULL CHECK (extracted_by IN ('staff','agent')),
    in_stock           BOOLEAN,
    strength           TEXT        NOT NULL DEFAULT '',
    quantity           INTEGER     CHECK (quantity IS NULL OR quantity >= 0),
    price_cents        BIGINT      CHECK (price_cents IS NULL OR price_cents >= 0),
    currency           TEXT        NOT NULL DEFAULT '',
    pickup_by          TEXT        NOT NULL DEFAULT '',
    payment            TEXT        NOT NULL DEFAULT '' CHECK (payment IN ('','cash','card','any')),
    needs_prescription BOOLEAN,
    evidence           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    confidence         DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    decision           TEXT        NOT NULL CHECK (decision IN ('matches','escalated','rejected')),
    decided_by         TEXT        NOT NULL DEFAULT 'rules' CHECK (decided_by IN ('rules','human')),
    reasons            JSONB       NOT NULL DEFAULT '[]'::jsonb,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX target_offers_enquiry_idx ON target_offers (enquiry_id, created_at);
