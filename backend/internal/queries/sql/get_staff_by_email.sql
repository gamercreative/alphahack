SELECT id, branch_id, full_name, email, pin_hash, role, created_at
FROM staff
WHERE lower(email) = lower($1)
