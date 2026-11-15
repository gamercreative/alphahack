UPDATE package_photos
SET agent_code = $2, agent_expiry_month = $3, agent_confidence = $4, read_at = now(),
    status = $5, gov_match = $6, note = $7
WHERE id = $1 AND decided_by = 'rules'
RETURNING id, target_id, branch_id, storage_path, content_type, size_bytes, typed_code, typed_expiry_month, status, gov_match, decided_by, note, reviewed_by_staff_id, agent_code, agent_expiry_month, agent_confidence, read_at, created_at
