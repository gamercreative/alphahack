SELECT id, display_name, phone, pin_hash, created_at
FROM clients
WHERE id = $1
