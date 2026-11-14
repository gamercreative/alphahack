SELECT id, brand_name, active_ingredient, therapeutic_class, dosage_form, strength, rx_required, controlled, chronic_eligible, unit_price_cents, contraindication_tags, description, created_at
FROM medications
WHERE brand_name ILIKE '%' || $1::text || '%'
   OR active_ingredient ILIKE '%' || $1::text || '%'
   OR therapeutic_class ILIKE '%' || $1::text || '%'
ORDER BY brand_name, id
LIMIT $2 OFFSET $3
