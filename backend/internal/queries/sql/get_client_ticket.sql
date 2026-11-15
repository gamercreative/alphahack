SELECT id, client_id, enquiry_id, schedule_id, mode, pickup_window, status, reserved_branch_id, created_at
FROM client_tickets
WHERE id = $1
