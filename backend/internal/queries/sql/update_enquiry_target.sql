UPDATE enquiry_targets
SET status = $2,
    reply_quantity = $3,
    reply_text = $4,
    replied_by_staff_id = $5,
    replied_at = $6,
    follow_ups = $7
WHERE id = $1
RETURNING id, enquiry_id, branch_id, status, reply_quantity, reply_text, replied_by_staff_id, replied_at, follow_ups, created_at
