SELECT s.id, s.branch_id, s.full_name, s.email, s.pin_hash, s.role, s.created_at FROM staff s JOIN staff_sessions ss ON ss.staff_id = s.id WHERE ss.token_hash = $1 AND ss.expires_at > now()
