SELECT id, seq, enquiry_id, actor_type, actor_id, type, payload, inverse, undone_at, undone_by_event_id, created_at
FROM enquiry_events
WHERE id = $1
