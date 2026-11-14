SELECT id, customer_id, branch_id, status, created_at, updated_at
FROM carts
WHERE id = $1
