UPDATE refill_subscriptions
SET status = $2
WHERE id = $1
RETURNING id, customer_id, medication_id, branch_id, prescription_id, interval_days, next_refill_at, status, created_at
