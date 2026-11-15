-- Client medicine schedules and client tickets. A ticket wraps exactly one
-- client-desk enquiry (handled_by = 'agent'). no_bulk: one ticket per cycle
-- with that cycle's quantity; cycles are never merged.
CREATE TABLE medicine_schedules (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    client_id     TEXT        NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    medication_id TEXT        NOT NULL REFERENCES medications(id),
    quantity      INTEGER     NOT NULL CHECK (quantity BETWEEN 1 AND 10),
    every         TEXT        NOT NULL CHECK (every IN ('week','2weeks','month')),
    weekday       INTEGER     NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    pickup_window TEXT        NOT NULL CHECK (pickup_window <> ''),
    no_bulk       BOOLEAN     NOT NULL DEFAULT true,
    status        TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active','paused')),
    next_run_at   TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX medicine_schedules_due_idx ON medicine_schedules (status, next_run_at);
CREATE INDEX medicine_schedules_client_idx ON medicine_schedules (client_id, created_at);

CREATE TABLE client_tickets (
    id                 TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    client_id          TEXT        NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    enquiry_id         TEXT        NOT NULL UNIQUE REFERENCES enquiries(id) ON DELETE CASCADE,
    schedule_id        TEXT        NULL REFERENCES medicine_schedules(id) ON DELETE SET NULL,
    mode               TEXT        NOT NULL CHECK (mode IN ('once','scheduled')),
    pickup_window      TEXT        NOT NULL CHECK (pickup_window <> ''),
    status             TEXT        NOT NULL DEFAULT 'open' CHECK (status IN ('open','reserved','closed')),
    reserved_branch_id TEXT        NULL REFERENCES branches(id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX client_tickets_client_idx ON client_tickets (client_id, created_at DESC);
CREATE INDEX client_tickets_schedule_idx ON client_tickets (schedule_id);
