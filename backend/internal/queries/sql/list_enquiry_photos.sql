SELECT p.id, p.target_id, p.branch_id, p.storage_path, p.content_type, p.size_bytes, p.typed_code, p.typed_expiry_month, p.status, p.gov_match, p.decided_by, p.note, p.reviewed_by_staff_id, p.agent_code, p.agent_expiry_month, p.agent_confidence, p.read_at, p.created_at
FROM package_photos p
JOIN enquiry_targets t ON t.id = p.target_id
WHERE t.enquiry_id = $1
ORDER BY p.created_at, p.id
