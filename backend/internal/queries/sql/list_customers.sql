SELECT id, full_name, phone, email, address, allergies, chronic_conditions, created_at, updated_at
FROM customers
ORDER BY full_name, id
LIMIT $1 OFFSET $2
