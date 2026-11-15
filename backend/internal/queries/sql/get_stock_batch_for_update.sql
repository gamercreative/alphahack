SELECT id, branch_id, medication_id, batch_number, expiry_date, quantity_on_hand, quantity_reserved, is_recalled, recall_reason, updated_at
FROM inventory_batches
WHERE branch_id = $1
  AND medication_id = $2
  AND is_recalled = false
  AND expiry_date > now()
ORDER BY expiry_date ASC, id
LIMIT 1
FOR UPDATE
