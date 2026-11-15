INSERT INTO gov_products (code, reg_no, brand_name, ingredient, strength, dosage_form, official_price_cents, source, document_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING code, reg_no, brand_name, ingredient, strength, dosage_form, official_price_cents, source, document_id, created_at
