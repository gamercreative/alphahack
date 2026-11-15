SELECT id, client_id, enquiry_id, schedule_id, mode, pickup_window, status, reserved_branch_id, created_at
FROM client_tickets
WHERE client_id = $1
  AND ($2::text = '' OR status = $2::text)
ORDER BY created_at DESC, id
LIMIT $3 OFFSET $4
