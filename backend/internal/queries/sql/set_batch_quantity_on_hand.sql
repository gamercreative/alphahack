UPDATE inventory_batches
SET quantity_on_hand = $2, updated_at = now()
WHERE id = $1
RETURNING id, branch_id, medication_id, batch_number, expiry_date, quantity_on_hand, quantity_reserved, is_recalled, recall_reason, updated_at
