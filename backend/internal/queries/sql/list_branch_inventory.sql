SELECT id, branch_id, medication_id, batch_number, expiry_date, quantity_on_hand, quantity_reserved, is_recalled, recall_reason, updated_at
FROM inventory_batches
WHERE branch_id = $1
  AND ($2::text = '' OR medication_id = $2)
  AND is_recalled = false
  AND expiry_date > now()
  AND quantity_on_hand - quantity_reserved > 0
ORDER BY expiry_date ASC, id
LIMIT $3 OFFSET $4
