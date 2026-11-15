UPDATE package_photos
SET status = $2, note = $3, reviewed_by_staff_id = $4, decided_by = 'human'
WHERE id = $1
RETURNING id, target_id, branch_id, storage_path, content_type, size_bytes, typed_code, typed_expiry_month, status, gov_match, decided_by, note, reviewed_by_staff_id, agent_code, agent_expiry_month, agent_confidence, read_at, created_at
