SELECT id, enquiry_id, target_id, branch_id, source_text, extracted_by, in_stock, strength, quantity, price_cents, currency, pickup_by, payment, needs_prescription, evidence, confidence, decision, decided_by, reasons, created_at
FROM target_offers
WHERE enquiry_id = $1
ORDER BY created_at, id
