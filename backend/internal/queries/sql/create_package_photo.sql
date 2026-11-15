INSERT INTO package_photos (id, target_id, branch_id, storage_path, content_type, size_bytes, typed_code, typed_expiry_month, status, gov_match, decided_by, note)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'rules', $11)
RETURNING id, target_id, branch_id, storage_path, content_type, size_bytes, typed_code, typed_expiry_month, status, gov_match, decided_by, note, reviewed_by_staff_id, agent_code, agent_expiry_month, agent_confidence, read_at, created_at
