SELECT id, client_id, medication_id, quantity, every, weekday, pickup_window, no_bulk, status, next_run_at, created_at
FROM medicine_schedules
WHERE status = 'active' AND next_run_at <= now()
ORDER BY next_run_at, id
LIMIT $1
FOR UPDATE SKIP LOCKED
