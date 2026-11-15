SELECT id, target_id, branch_id, storage_path, content_type, size_bytes, typed_code, typed_expiry_month, status, gov_match, decided_by, note, reviewed_by_staff_id, agent_code, agent_expiry_month, agent_confidence, read_at, created_at
FROM package_photos
WHERE id = $1
