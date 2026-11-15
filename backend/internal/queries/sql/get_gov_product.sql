SELECT code, reg_no, brand_name, ingredient, strength, dosage_form, official_price_cents, source, document_id, created_at
FROM gov_products
WHERE code = $1
