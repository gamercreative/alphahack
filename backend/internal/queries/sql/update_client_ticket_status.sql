UPDATE client_tickets
SET status = $2, reserved_branch_id = $3
WHERE id = $1
RETURNING id, client_id, enquiry_id, schedule_id, mode, pickup_window, status, reserved_branch_id, created_at
