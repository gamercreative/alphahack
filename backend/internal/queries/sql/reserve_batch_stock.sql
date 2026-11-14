UPDATE inventory_batches
SET quantity_reserved = quantity_reserved + $2::integer, updated_at = now()
WHERE id = $1
  AND $2::integer > 0
  AND is_recalled = false
  AND expiry_date > now()
  AND quantity_on_hand - quantity_reserved >= $2::integer
RETURNING id, branch_id, medication_id, batch_number, expiry_date, quantity_on_hand, quantity_reserved, is_recalled, recall_reason, updated_at
