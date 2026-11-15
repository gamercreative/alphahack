INSERT INTO medicine_schedules (client_id, medication_id, quantity, every, weekday, pickup_window, next_run_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, client_id, medication_id, quantity, every, weekday, pickup_window, no_bulk, status, next_run_at, created_at
