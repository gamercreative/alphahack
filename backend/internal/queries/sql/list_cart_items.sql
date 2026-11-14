SELECT id, cart_id, medication_id, prescription_id, quantity, unit_price_cents, created_at
FROM cart_items
WHERE cart_id = $1
ORDER BY created_at ASC, id
