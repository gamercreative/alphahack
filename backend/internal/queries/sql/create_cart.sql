INSERT INTO carts (customer_id, branch_id)
VALUES ($1, $2)
RETURNING id, customer_id, branch_id, status, created_at, updated_at
