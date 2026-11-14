SELECT id, customer_id, branch_id, cart_id, status, fulfillment_type, delivery_address, total_cents, created_at, updated_at
FROM orders
WHERE customer_id = $1
ORDER BY created_at DESC, id
LIMIT $2 OFFSET $3
