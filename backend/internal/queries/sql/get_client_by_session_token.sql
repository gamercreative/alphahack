SELECT c.id, c.display_name, c.phone, c.pin_hash, c.created_at
FROM clients c
JOIN client_sessions cs ON cs.client_id = c.id
WHERE cs.token_hash = $1 AND cs.expires_at > now()
