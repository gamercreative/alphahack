SELECT id, name, content_type, size_bytes, sha256, uploaded_by_staff_id, storage_path, created_at
FROM reference_documents
ORDER BY created_at DESC, id
LIMIT $1 OFFSET $2
