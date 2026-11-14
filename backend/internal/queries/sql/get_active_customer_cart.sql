SELECT id, customer_id, branch_id, status, created_at, updated_at
FROM carts
WHERE customer_id = $1
  AND status = 'active'
ORDER BY updated_at DESC, id
LIMIT 1
