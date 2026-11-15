UPDATE medicine_schedules
SET status = $2, next_run_at = $3
WHERE id = $1
RETURNING id, client_id, medication_id, quantity, every, weekday, pickup_window, no_bulk, status, next_run_at, created_at
