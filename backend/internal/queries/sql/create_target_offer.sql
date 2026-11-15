INSERT INTO target_offers (enquiry_id, target_id, branch_id, source_text, extracted_by, in_stock, strength, quantity, price_cents, currency, pickup_by, payment, needs_prescription, evidence, confidence, decision, decided_by, reasons)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
RETURNING id, enquiry_id, target_id, branch_id, source_text, extracted_by, in_stock, strength, quantity, price_cents, currency, pickup_by, payment, needs_prescription, evidence, confidence, decision, decided_by, reasons, created_at
