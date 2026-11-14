SELECT id, customer_id, medication_id, branch_id, prescription_id, interval_days, next_refill_at, status, created_at
FROM refill_subscriptions
WHERE customer_id = $1
ORDER BY next_refill_at ASC, id
