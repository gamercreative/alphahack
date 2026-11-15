-- Government reference: official product list (code, registration, official price)
-- and the documents staff upload as its source. The agent may read the list
-- (information only); only staff add rows or documents. The seed rows are demo
-- data, labelled as such in `source`, never real MoPH data.
CREATE TABLE reference_documents (
    id                   TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name                 TEXT        NOT NULL CHECK (name <> ''),
    content_type         TEXT        NOT NULL CHECK (content_type IN ('application/pdf', 'image/png', 'image/jpeg')),
    size_bytes           BIGINT      NOT NULL CHECK (size_bytes >= 0),
    sha256               TEXT        NOT NULL CHECK (sha256 <> ''),
    uploaded_by_staff_id TEXT        NOT NULL REFERENCES staff(id),
    storage_path         TEXT        NOT NULL CHECK (storage_path <> ''),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX reference_documents_created_idx ON reference_documents (created_at DESC, id);

CREATE TABLE gov_products (
    code                 TEXT PRIMARY KEY CHECK (code ~ '^[0-9]{13}$'),
    reg_no               TEXT        NOT NULL CHECK (reg_no <> ''),
    brand_name           TEXT        NOT NULL CHECK (brand_name <> ''),
    ingredient           TEXT        NOT NULL CHECK (ingredient <> ''),
    strength             TEXT        NOT NULL CHECK (strength <> ''),
    dosage_form          TEXT        NOT NULL CHECK (dosage_form <> ''),
    official_price_cents BIGINT      NOT NULL CHECK (official_price_cents >= 0),
    source               TEXT        NOT NULL CHECK (source <> ''),
    document_id          TEXT        NULL REFERENCES reference_documents(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ingredient, strength, dosage_form, brand_name)
);
CREATE INDEX gov_products_ingredient_idx ON gov_products (lower(ingredient));

INSERT INTO gov_products (code, reg_no, brand_name, ingredient, strength, dosage_form, official_price_cents, source) VALUES
    ('6280000000001', 'DEMO-0001', 'Augmentin',       'Amoxicillin/Clavulanate', '1000mg', 'tablet',  1200, 'Demo seed, not real MoPH data'),
    ('6280000000002', 'DEMO-0002', 'Augmentin',       'Amoxicillin/Clavulanate', '625mg',  'tablet',  1450, 'Demo seed, not real MoPH data'),
    ('6280000000003', 'DEMO-0003', 'Augmentin',       'Amoxicillin/Clavulanate', '500mg',  'tablet',   700, 'Demo seed, not real MoPH data'),
    ('6280000000004', 'DEMO-0004', 'Brufen',          'Ibuprofen',               '400mg',  'tablet',   600, 'Demo seed, not real MoPH data'),
    ('6280000000005', 'DEMO-0005', 'Brufen',          'Ibuprofen',               '200mg',  'tablet',   350, 'Demo seed, not real MoPH data'),
    ('6280000000006', 'DEMO-0006', 'Concor',          'Bisoprolol',              '5mg',    'tablet',   900, 'Demo seed, not real MoPH data'),
    ('6280000000007', 'DEMO-0007', 'Concor',          'Bisoprolol',              '2.5mg',  'tablet',   520, 'Demo seed, not real MoPH data'),
    ('6280000000008', 'DEMO-0008', 'Glucophage',      'Metformin',               '1000mg', 'tablet',   650, 'Demo seed, not real MoPH data'),
    ('6280000000009', 'DEMO-0009', 'Glucophage',      'Metformin',               '500mg',  'tablet',   380, 'Demo seed, not real MoPH data'),
    ('6280000000010', 'DEMO-0010', 'Lexotanil',       'Bromazepam',              '3mg',    'tablet',   500, 'Demo seed, not real MoPH data'),
    ('6280000000011', 'DEMO-0011', 'Lipitor',         'Atorvastatin',            '20mg',   'tablet',  2400, 'Demo seed, not real MoPH data'),
    ('6280000000012', 'DEMO-0012', 'Lipitor',         'Atorvastatin',            '10mg',   'tablet',  1300, 'Demo seed, not real MoPH data'),
    ('6280000000013', 'DEMO-0013', 'Nexium',          'Esomeprazole',            '20mg',   'tablet',  1450, 'Demo seed, not real MoPH data'),
    ('6280000000014', 'DEMO-0014', 'Panadol Advance', 'Paracetamol',             '500mg',  'tablet',   450, 'Demo seed, not real MoPH data'),
    ('6280000000015', 'DEMO-0015', 'Panadol',         'Paracetamol',             '250mg',  'tablet',   260, 'Demo seed, not real MoPH data'),
    ('6280000000016', 'DEMO-0016', 'Ventolin',        'Salbutamol',              '100mcg', 'inhaler',  750, 'Demo seed, not real MoPH data'),
    ('6280000000017', 'DEMO-0017', 'Zithromax',       'Azithromycin',            '500mg',  'tablet',  1100, 'Demo seed, not real MoPH data'),
    ('6280000000018', 'DEMO-0018', 'Zithromax',       'Azithromycin',            '250mg',  'tablet',   600, 'Demo seed, not real MoPH data');
