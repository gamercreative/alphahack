UPDATE carts
SET status = $2, updated_at = now()
WHERE id = $1
RETURNING id, customer_id, branch_id, status, created_at, updated_at
