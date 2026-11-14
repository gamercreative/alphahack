SELECT id, customer_id, medication_id, doctor_name, dosage_instructions, allowed_refills, remaining_refills, status, notes, expires_at, created_at
FROM prescriptions
WHERE id = $1
