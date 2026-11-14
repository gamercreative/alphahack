INSERT INTO order_items (order_id, medication_id, batch_id, quantity, unit_price_cents)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, order_id, medication_id, batch_id, quantity, unit_price_cents, created_at
