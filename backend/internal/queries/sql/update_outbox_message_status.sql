UPDATE outbox_messages SET status = $2, sent_at = $3 WHERE id = $1 RETURNING id, enquiry_id, target_id, kind, body, status, send_after, sent_at, created_by_actor, created_at
