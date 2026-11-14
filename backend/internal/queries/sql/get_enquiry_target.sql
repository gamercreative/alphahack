SELECT id, enquiry_id, branch_id, status, reply_quantity, reply_text, replied_by_staff_id, replied_at, follow_ups, created_at
FROM enquiry_targets
WHERE id = $1
