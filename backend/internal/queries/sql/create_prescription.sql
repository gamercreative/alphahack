INSERT INTO prescriptions (customer_id, medication_id, doctor_name, dosage_instructions,
                           allowed_refills, remaining_refills, status, notes, expires_at)
VALUES ($1, $2, $3, $4, $5::integer, $5::integer, $6, $7, $8)
RETURNING id, customer_id, medication_id, doctor_name, dosage_instructions, allowed_refills, remaining_refills, status, notes, expires_at, created_at
