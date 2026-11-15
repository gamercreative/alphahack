-- DOSYN clients: a minimal account (display name, phone, PIN) that can open
-- tickets. Never linked to the B2C `customers` table, so the agent can never
-- reach allergy or chronic-condition fields. Pharmacies see client tickets as
-- coming from the 'DOSYN client desk' branch, never the client.
CREATE TABLE clients (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    display_name TEXT        NOT NULL CHECK (display_name <> ''),
    phone        TEXT        NOT NULL UNIQUE CHECK (phone <> ''),
    pin_hash     TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE client_sessions (
    token_hash TEXT PRIMARY KEY,
    client_id  TEXT        NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL
);

ALTER TABLE enquiry_events DROP CONSTRAINT enquiry_events_actor_type_check;
ALTER TABLE enquiry_events ADD CONSTRAINT enquiry_events_actor_type_check
    CHECK (actor_type IN ('staff','agent','system','client'));

-- The desk branch is the requester of every client ticket. participating = false
-- keeps it out of candidate rankings; no lat/lng.
INSERT INTO branches (id, name, city, address, phone, supports_delivery, participating) VALUES
    ('branch-client-desk', 'DOSYN client desk', 'Beirut', 'DOSYN client desk (online)', '', false, false)
ON CONFLICT (id) DO NOTHING;

-- Desk reviewer (PIN 1234) and the demo client Maya (PIN 1234).
INSERT INTO staff (id, branch_id, full_name, email, pin_hash, role) VALUES
    ('staff-client-desk', 'branch-client-desk', 'DOSYN desk reviewer', 'desk@wassit.demo',
     '03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4', 'reviewer')
ON CONFLICT (id) DO NOTHING;

INSERT INTO clients (id, display_name, phone, pin_hash) VALUES
    ('client-maya', 'Maya', '+96170000000', '03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4')
ON CONFLICT (id) DO NOTHING;
