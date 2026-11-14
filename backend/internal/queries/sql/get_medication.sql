SELECT id, brand_name, active_ingredient, therapeutic_class, dosage_form, strength, rx_required, controlled, chronic_eligible, unit_price_cents, contraindication_tags, description, created_at
FROM medications
WHERE id = $1
