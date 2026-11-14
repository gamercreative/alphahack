UPDATE customers
SET full_name = $2, email = $3, address = $4,
    allergies = $5::jsonb, chronic_conditions = $6::jsonb,
    updated_at = now()
WHERE id = $1
RETURNING id, full_name, phone, email, address, allergies, chronic_conditions, created_at, updated_at
