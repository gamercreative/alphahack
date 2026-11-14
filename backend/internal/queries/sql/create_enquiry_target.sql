INSERT INTO enquiry_targets (enquiry_id, branch_id, status)
VALUES ($1, $2, $3)
RETURNING id, enquiry_id, branch_id, status, reply_quantity, reply_text, replied_by_staff_id, replied_at, follow_ups, created_at
