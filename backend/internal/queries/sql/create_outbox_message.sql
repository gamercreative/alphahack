INSERT INTO outbox_messages (enquiry_id, target_id, kind, body, send_after, created_by_actor)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, enquiry_id, target_id, kind, body, status, send_after, sent_at, created_by_actor, created_at
