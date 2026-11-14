INSERT INTO refill_subscriptions (customer_id, medication_id, branch_id, prescription_id, interval_days, next_refill_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, customer_id, medication_id, branch_id, prescription_id, interval_days, next_refill_at, status, created_at
