SELECT id, customer_id, branch_id, cart_id, status, fulfillment_type, delivery_address, total_cents, created_at, updated_at
FROM orders
WHERE id = $1
