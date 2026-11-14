-- 0002_pharmacy_base: B2C pharmacy domain.
-- branches, medications, inventory_batches, customers, prescriptions, carts,
-- cart_items, orders, order_items, refill_subscriptions.
-- Money is BIGINT cents. Tag lists are JSONB arrays of strings.

CREATE TABLE branches (
    id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name              TEXT        NOT NULL CHECK (name <> ''),
    city              TEXT        NOT NULL CHECK (city <> ''),
    address           TEXT        NOT NULL DEFAULT '',
    phone             TEXT        NOT NULL DEFAULT '',
    supports_delivery BOOLEAN     NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE medications (
    id                    TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    brand_name            TEXT        NOT NULL CHECK (brand_name <> ''),
    active_ingredient     TEXT        NOT NULL CHECK (active_ingredient <> ''),
    therapeutic_class     TEXT        NOT NULL DEFAULT '',
    dosage_form           TEXT        NOT NULL
                          CHECK (dosage_form IN ('tablet', 'capsule', 'syrup', 'inhaler', 'injection', 'cream', 'drops')),
    strength              TEXT        NOT NULL CHECK (strength <> ''),
    rx_required           BOOLEAN     NOT NULL DEFAULT false,
    controlled            BOOLEAN     NOT NULL DEFAULT false,
    chronic_eligible      BOOLEAN     NOT NULL DEFAULT false,
    unit_price_cents      BIGINT      NOT NULL CHECK (unit_price_cents >= 0),
    contraindication_tags JSONB       NOT NULL DEFAULT '[]'::jsonb,
    description           TEXT        NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX medications_active_ingredient_idx ON medications (active_ingredient);
CREATE INDEX medications_brand_name_idx ON medications (brand_name);

CREATE TABLE inventory_batches (
    id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    branch_id         TEXT        NOT NULL REFERENCES branches (id) ON DELETE CASCADE,
    medication_id     TEXT        NOT NULL REFERENCES medications (id) ON DELETE CASCADE,
    batch_number      TEXT        NOT NULL CHECK (batch_number <> ''),
    expiry_date       TIMESTAMPTZ NOT NULL,
    quantity_on_hand  INTEGER     NOT NULL DEFAULT 0 CHECK (quantity_on_hand >= 0),
    quantity_reserved INTEGER     NOT NULL DEFAULT 0
                      CHECK (quantity_reserved >= 0 AND quantity_reserved <= quantity_on_hand),
    is_recalled       BOOLEAN     NOT NULL DEFAULT false,
    recall_reason     TEXT        NOT NULL DEFAULT '',
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX inventory_batches_branch_med_batch_uidx
    ON inventory_batches (branch_id, medication_id, batch_number);
CREATE INDEX inventory_batches_fefo_idx
    ON inventory_batches (branch_id, medication_id, expiry_date ASC);

CREATE TABLE customers (
    id                 TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    full_name          TEXT        NOT NULL CHECK (full_name <> ''),
    phone              TEXT        NOT NULL UNIQUE CHECK (phone <> ''),
    email              TEXT        NOT NULL DEFAULT '',
    address            TEXT        NOT NULL DEFAULT '',
    allergies          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    chronic_conditions JSONB       NOT NULL DEFAULT '[]'::jsonb,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE prescriptions (
    id                  TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    customer_id         TEXT        NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    medication_id       TEXT        NOT NULL REFERENCES medications (id) ON DELETE RESTRICT,
    doctor_name         TEXT        NOT NULL CHECK (doctor_name <> ''),
    dosage_instructions TEXT        NOT NULL DEFAULT '',
    allowed_refills     INTEGER     NOT NULL DEFAULT 1 CHECK (allowed_refills >= 0),
    remaining_refills   INTEGER     NOT NULL DEFAULT 1
                        CHECK (remaining_refills >= 0 AND remaining_refills <= allowed_refills),
    status              TEXT        NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'verified', 'rejected', 'expired')),
    notes               TEXT        NOT NULL DEFAULT '',
    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX prescriptions_customer_status_idx ON prescriptions (customer_id, status);

CREATE TABLE carts (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    customer_id TEXT        NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    branch_id   TEXT        NOT NULL REFERENCES branches (id) ON DELETE CASCADE,
    status      TEXT        NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'checked_out', 'abandoned')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX carts_customer_status_idx ON carts (customer_id, status);

CREATE TABLE cart_items (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    cart_id          TEXT        NOT NULL REFERENCES carts (id) ON DELETE CASCADE,
    medication_id    TEXT        NOT NULL REFERENCES medications (id) ON DELETE RESTRICT,
    prescription_id  TEXT        REFERENCES prescriptions (id) ON DELETE SET NULL,
    quantity         INTEGER     NOT NULL CHECK (quantity > 0),
    unit_price_cents BIGINT      NOT NULL CHECK (unit_price_cents >= 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX cart_items_cart_med_uidx ON cart_items (cart_id, medication_id);

CREATE TABLE orders (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    customer_id      TEXT        NOT NULL REFERENCES customers (id) ON DELETE RESTRICT,
    branch_id        TEXT        NOT NULL REFERENCES branches (id) ON DELETE RESTRICT,
    cart_id          TEXT        REFERENCES carts (id) ON DELETE SET NULL,
    status           TEXT        NOT NULL DEFAULT 'confirmed'
                     CHECK (status IN ('pending_rx', 'confirmed', 'ready_for_pickup', 'out_for_delivery', 'completed', 'cancelled')),
    fulfillment_type TEXT        NOT NULL CHECK (fulfillment_type IN ('pickup', 'delivery')),
    delivery_address TEXT        NOT NULL DEFAULT '',
    total_cents      BIGINT      NOT NULL CHECK (total_cents >= 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX orders_customer_created_idx ON orders (customer_id, created_at DESC);

CREATE TABLE order_items (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    order_id         TEXT        NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    medication_id    TEXT        NOT NULL REFERENCES medications (id) ON DELETE RESTRICT,
    batch_id         TEXT        NOT NULL REFERENCES inventory_batches (id) ON DELETE RESTRICT,
    quantity         INTEGER     NOT NULL CHECK (quantity > 0),
    unit_price_cents BIGINT      NOT NULL CHECK (unit_price_cents >= 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX order_items_order_idx ON order_items (order_id);

CREATE TABLE refill_subscriptions (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    customer_id     TEXT        NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    medication_id   TEXT        NOT NULL REFERENCES medications (id) ON DELETE RESTRICT,
    branch_id       TEXT        NOT NULL REFERENCES branches (id) ON DELETE RESTRICT,
    prescription_id TEXT        REFERENCES prescriptions (id) ON DELETE SET NULL,
    interval_days   INTEGER     NOT NULL CHECK (interval_days >= 7 AND interval_days <= 90),
    next_refill_at  TIMESTAMPTZ NOT NULL,
    status          TEXT        NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'paused', 'cancelled')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX refill_subscriptions_customer_idx ON refill_subscriptions (customer_id, next_refill_at);
