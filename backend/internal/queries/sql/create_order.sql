INSERT INTO orders (customer_id, branch_id, cart_id, status, fulfillment_type, delivery_address, total_cents)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, customer_id, branch_id, cart_id, status, fulfillment_type, delivery_address, total_cents, created_at, updated_at
