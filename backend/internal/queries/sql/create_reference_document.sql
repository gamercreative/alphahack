INSERT INTO reference_documents (id, name, content_type, size_bytes, sha256, uploaded_by_staff_id, storage_path)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, name, content_type, size_bytes, sha256, uploaded_by_staff_id, storage_path, created_at
