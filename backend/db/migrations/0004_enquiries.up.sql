-- 0004_enquiries: pharmacist-to-pharmacist availability enquiries (Wassit).
-- branches gain location + participation; staff log in with a PIN; every state
-- change is an enquiry_events row; outbound messages go through outbox_messages.

ALTER TABLE branches
    ADD COLUMN lat           DOUBLE PRECISION,
    ADD COLUMN lng           DOUBLE PRECISION,
    ADD COLUMN participating BOOLEAN NOT NULL DEFAULT true;

CREATE TABLE staff (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    branch_id  TEXT        NOT NULL REFERENCES branches(id),
    full_name  TEXT        NOT NULL CHECK (full_name <> ''),
    email      TEXT        NOT NULL UNIQUE CHECK (email <> ''),
    pin_hash   TEXT        NOT NULL,
    role       TEXT        NOT NULL DEFAULT 'pharmacist',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE staff_sessions (
    token_hash TEXT PRIMARY KEY,
    staff_id   TEXT        NOT NULL REFERENCES staff(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE enquiries (
    id                     TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    requester_branch_id    TEXT        NOT NULL REFERENCES branches(id),
    requester_staff_id     TEXT        NOT NULL REFERENCES staff(id),
    raw_request            TEXT        NOT NULL CHECK (raw_request <> ''),
    medication_id          TEXT        REFERENCES medications(id),
    quantity               INTEGER     NOT NULL CHECK (quantity > 0),
    status                 TEXT        NOT NULL DEFAULT 'open'
                           CHECK (status IN ('open','awaiting_clarification','awaiting_approval','waiting','closed','cancelled')),
    outcome                TEXT        NOT NULL DEFAULT ''
                           CHECK (outcome IN ('','available','unavailable','unable_to_confirm','cancelled')),
    outcome_branch_id      TEXT        REFERENCES branches(id),
    handled_by             TEXT        NOT NULL CHECK (handled_by IN ('manual','agent')),
    clarification_question TEXT        NOT NULL DEFAULT '',
    clarification_answer   TEXT        NOT NULL DEFAULT '',
    wake_trigger           TEXT        NOT NULL DEFAULT '',
    next_wake_at           TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at              TIMESTAMPTZ
);
CREATE INDEX enquiries_requester_idx ON enquiries (requester_branch_id, created_at DESC);

CREATE TABLE enquiry_targets (
    id                  TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    enquiry_id          TEXT        NOT NULL REFERENCES enquiries(id) ON DELETE CASCADE,
    branch_id           TEXT        NOT NULL REFERENCES branches(id),
    status              TEXT        NOT NULL
                        CHECK (status IN ('proposed','approved','sent','available','unavailable','alternative_offered','no_response','cancelled')),
    reply_quantity      INTEGER,
    reply_text          TEXT        NOT NULL DEFAULT '',
    replied_by_staff_id TEXT        REFERENCES staff(id),
    replied_at          TIMESTAMPTZ,
    follow_ups          INTEGER     NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (enquiry_id, branch_id)
);
CREATE INDEX enquiry_targets_branch_idx ON enquiry_targets (branch_id);

CREATE TABLE outbox_messages (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    enquiry_id       TEXT        NOT NULL REFERENCES enquiries(id) ON DELETE CASCADE,
    target_id        TEXT        REFERENCES enquiry_targets(id) ON DELETE CASCADE,
    kind             TEXT        NOT NULL CHECK (kind IN ('enquiry','follow_up','retraction','notification')),
    body             TEXT        NOT NULL DEFAULT '',
    status           TEXT        NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','sent','cancelled')),
    send_after       TIMESTAMPTZ NOT NULL,
    sent_at          TIMESTAMPTZ,
    created_by_actor TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX outbox_due_idx ON outbox_messages (send_after) WHERE status = 'queued';

CREATE TABLE enquiry_events (
    id                 TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    seq                BIGSERIAL   NOT NULL UNIQUE,
    enquiry_id         TEXT        NOT NULL REFERENCES enquiries(id) ON DELETE CASCADE,
    actor_type         TEXT        NOT NULL CHECK (actor_type IN ('staff','agent','system')),
    actor_id           TEXT        NOT NULL DEFAULT '',
    type               TEXT        NOT NULL,
    payload            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    inverse            JSONB,
    undone_at          TIMESTAMPTZ,
    undone_by_event_id TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX enquiry_events_enquiry_idx ON enquiry_events (enquiry_id, seq);

CREATE TABLE enquiry_agent_runs (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    enquiry_id    TEXT        NOT NULL REFERENCES enquiries(id) ON DELETE CASCADE,
    trigger       TEXT        NOT NULL DEFAULT '',
    model         TEXT        NOT NULL DEFAULT '',
    input_tokens  INTEGER     NOT NULL DEFAULT 0,
    output_tokens INTEGER     NOT NULL DEFAULT 0,
    steps         INTEGER     NOT NULL DEFAULT 0,
    cost_micros   BIGINT      NOT NULL DEFAULT 0,
    status        TEXT        NOT NULL DEFAULT 'running' CHECK (status IN ('running','completed','failed')),
    error         TEXT        NOT NULL DEFAULT '',
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at   TIMESTAMPTZ
);

CREATE TABLE enquiry_agent_tool_calls (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    seq         BIGSERIAL   NOT NULL UNIQUE,
    run_id      TEXT        NOT NULL REFERENCES enquiry_agent_runs(id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    input       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    output      TEXT        NOT NULL DEFAULT '',
    is_error    BOOLEAN     NOT NULL DEFAULT false,
    duration_ms INTEGER     NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
