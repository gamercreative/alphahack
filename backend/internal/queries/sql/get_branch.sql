SELECT id, name, city, address, phone, supports_delivery, created_at
FROM branches
WHERE id = $1
