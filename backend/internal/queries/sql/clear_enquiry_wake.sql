UPDATE enquiries
SET wake_trigger = '',
    next_wake_at = NULL
WHERE id = $1
