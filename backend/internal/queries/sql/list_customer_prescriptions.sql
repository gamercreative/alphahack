SELECT id, customer_id, medication_id, doctor_name, dosage_instructions, allowed_refills, remaining_refills, status, notes, expires_at, created_at
FROM prescriptions
WHERE customer_id = $1
  AND ($2::text = '' OR status = $2)
ORDER BY created_at DESC, id
LIMIT $3 OFFSET $4
