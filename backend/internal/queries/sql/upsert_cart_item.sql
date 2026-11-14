INSERT INTO cart_items (cart_id, medication_id, prescription_id, quantity, unit_price_cents)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (cart_id, medication_id) DO UPDATE
SET prescription_id = EXCLUDED.prescription_id,
    quantity = EXCLUDED.quantity,
    unit_price_cents = EXCLUDED.unit_price_cents
RETURNING id, cart_id, medication_id, prescription_id, quantity, unit_price_cents, created_at
