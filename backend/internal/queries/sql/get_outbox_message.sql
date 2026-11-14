SELECT id, enquiry_id, target_id, kind, body, status, send_after, sent_at, created_by_actor, created_at
FROM outbox_messages
WHERE id = $1
