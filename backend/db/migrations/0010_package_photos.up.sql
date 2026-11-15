-- Package photos: evidence, not proof (CONTRACT rule 10). The target pharmacist
-- types the product code and expiry next to the photo; Go compares them with the
-- government reference. A photo alone never proves a box is genuine.
CREATE TABLE package_photos (
    id                   TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    target_id            TEXT        NOT NULL REFERENCES enquiry_targets(id) ON DELETE CASCADE,
    branch_id            TEXT        NOT NULL REFERENCES branches(id),
    storage_path         TEXT        NOT NULL CHECK (storage_path <> ''),
    content_type         TEXT        NOT NULL CHECK (content_type IN ('image/jpeg','image/png')),
    size_bytes           BIGINT      NOT NULL CHECK (size_bytes >= 0),
    typed_code           TEXT        NOT NULL DEFAULT '',
    typed_expiry_month   TEXT        NOT NULL DEFAULT '',
    status               TEXT        NOT NULL CHECK (status IN ('verified','flagged','mismatch','rejected')),
    gov_match            BOOLEAN     NOT NULL DEFAULT false,
    decided_by           TEXT        NOT NULL CHECK (decided_by IN ('rules','human')),
    note                 TEXT        NOT NULL DEFAULT '',
    reviewed_by_staff_id TEXT        NULL REFERENCES staff(id),
    -- The agent's reading of the same photo (POST /api/v1/photos/{id}/agent-reading).
    -- Go compares it with what the pharmacist typed; the model never decides.
    agent_code           TEXT        NOT NULL DEFAULT '',
    agent_expiry_month   TEXT        NOT NULL DEFAULT '',
    agent_confidence     DOUBLE PRECISION NULL CHECK (agent_confidence IS NULL OR (agent_confidence >= 0 AND agent_confidence <= 1)),
    read_at              TIMESTAMPTZ NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX package_photos_target_idx ON package_photos (target_id, created_at);
