INSERT INTO client_tickets (client_id, enquiry_id, schedule_id, mode, pickup_window)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, client_id, enquiry_id, schedule_id, mode, pickup_window, status, reserved_branch_id, created_at
