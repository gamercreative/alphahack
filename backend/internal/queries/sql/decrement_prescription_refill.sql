UPDATE prescriptions
SET remaining_refills = remaining_refills - 1
WHERE id = $1
  AND status = 'verified'
  AND remaining_refills > 0
  AND expires_at > now()
RETURNING id, customer_id, medication_id, doctor_name, dosage_instructions, allowed_refills, remaining_refills, status, notes, expires_at, created_at
