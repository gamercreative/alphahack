SELECT id, order_id, medication_id, batch_id, quantity, unit_price_cents, created_at
FROM order_items
WHERE order_id = $1
ORDER BY created_at ASC, id
