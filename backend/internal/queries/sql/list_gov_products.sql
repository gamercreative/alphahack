SELECT code, reg_no, brand_name, ingredient, strength, dosage_form, official_price_cents, source, document_id, created_at
FROM gov_products
WHERE ($1::text = '' OR brand_name ILIKE '%' || $1::text || '%' ESCAPE '\' OR ingredient ILIKE '%' || $1::text || '%' ESCAPE '\')
  AND ($2::text = '' OR lower(ingredient) = lower($2::text))
  AND ($3::text = '' OR lower(strength) = lower($3::text))
  AND ($4::text = '' OR lower(dosage_form) = lower($4::text))
ORDER BY ingredient, brand_name, dosage_form, strength, code
LIMIT $5 OFFSET $6
