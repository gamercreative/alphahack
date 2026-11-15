SELECT id, display_name, phone, pin_hash, created_at
FROM clients
WHERE phone = $1
