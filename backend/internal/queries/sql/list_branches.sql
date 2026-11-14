SELECT id, name, city, address, phone, supports_delivery, created_at
FROM branches
ORDER BY name, id
LIMIT $1 OFFSET $2
