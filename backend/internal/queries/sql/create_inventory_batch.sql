INSERT INTO inventory_batches (branch_id, medication_id, batch_number, expiry_date, quantity_on_hand)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, branch_id, medication_id, batch_number, expiry_date, quantity_on_hand, quantity_reserved, is_recalled, recall_reason, updated_at
