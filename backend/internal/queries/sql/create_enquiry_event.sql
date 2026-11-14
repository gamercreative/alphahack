INSERT INTO enquiry_events (enquiry_id, actor_type, actor_id, type, payload, inverse)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, seq, enquiry_id, actor_type, actor_id, type, payload, inverse, undone_at, undone_by_event_id, created_at
