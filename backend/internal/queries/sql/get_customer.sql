SELECT id, full_name, phone, email, address, allergies, chronic_conditions, created_at, updated_at
FROM customers
WHERE id = $1
