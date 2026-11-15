-- 0005_enquiry_seed: demo data for Wassit. Every staff PIN is 1234 (sha256 hex below).
-- Demo roles: A = Hamra (requester), B = Achrafieh, C = Baabda, D = Verdun.

INSERT INTO branches (id, name, city, address, phone, supports_delivery) VALUES
    ('branch-achrafieh', 'Cornilia Pharmacy Achrafieh', 'Beirut - Achrafieh', 'Sassine Square, Achrafieh, Beirut', '+961 1 200 321', true),
    ('branch-verdun',    'Cornilia Pharmacy Verdun',    'Beirut - Verdun',    'Rue Verdun, near Dunes, Beirut',    '+961 1 800 654', true)
ON CONFLICT (id) DO NOTHING;

UPDATE branches SET lat = 33.8959, lng = 35.4787 WHERE id = 'branch-hamra';
UPDATE branches SET lat = 33.8886, lng = 35.5200 WHERE id = 'branch-achrafieh';
UPDATE branches SET lat = 33.8339, lng = 35.5442 WHERE id = 'branch-baabda';
UPDATE branches SET lat = 33.8790, lng = 35.4830 WHERE id = 'branch-verdun';
UPDATE branches SET lat = 34.1230, lng = 35.6519 WHERE id = 'branch-byblos';

-- A second Augmentin strength so "need augmentin" is genuinely ambiguous.
INSERT INTO medications (id, brand_name, active_ingredient, therapeutic_class, dosage_form, strength,
                         rx_required, controlled, chronic_eligible, unit_price_cents, contraindication_tags, description) VALUES
    ('med-augmentin-625', 'Augmentin', 'Amoxicillin/Clavulanate', 'Antibiotic (Penicillin)', 'tablet', '625mg',
     true, false, false, 1450, '[]'::jsonb, 'Amoxicillin 500mg + clavulanic acid 125mg.')
ON CONFLICT (id) DO NOTHING;

INSERT INTO inventory_batches (id, branch_id, medication_id, batch_number, expiry_date,
                               quantity_on_hand, quantity_reserved, is_recalled, recall_reason) VALUES
    ('batch-achrafieh-augmentin-1g', 'branch-achrafieh', 'med-augmentin-1g',  'AUG-2709A', '2027-09-30T00:00:00Z', 6, 0, false, ''),
    ('batch-baabda-augmentin-625',   'branch-baabda',    'med-augmentin-625', 'AUG-2705C', '2027-05-31T00:00:00Z', 8, 0, false, ''),
    ('batch-verdun-augmentin-1g',    'branch-verdun',    'med-augmentin-1g',  'AUG-2711V', '2027-11-30T00:00:00Z', 3, 0, false, ''),
    ('batch-verdun-panadol',         'branch-verdun',    'med-panadol-advance','PAN-2805V', '2028-05-31T00:00:00Z', 50, 0, false, '')
ON CONFLICT (id) DO NOTHING;

INSERT INTO staff (id, branch_id, full_name, email, pin_hash) VALUES
    ('staff-hamra',     'branch-hamra',     'Pharmacist Hamra (A)',     'a@wassit.demo', '03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4'),
    ('staff-achrafieh', 'branch-achrafieh', 'Pharmacist Achrafieh (B)', 'b@wassit.demo', '03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4'),
    ('staff-baabda',    'branch-baabda',    'Pharmacist Baabda (C)',    'c@wassit.demo', '03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4'),
    ('staff-verdun',    'branch-verdun',    'Pharmacist Verdun (D)',    'd@wassit.demo', '03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4'),
    ('staff-byblos',    'branch-byblos',    'Pharmacist Byblos (E)',    'e@wassit.demo', '03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4')
ON CONFLICT (id) DO NOTHING;

INSERT INTO agents (id, name, system_prompt, model) VALUES
    ('00000000-0000-4000-8000-000000000001', 'Wassit B2B Pharmacy Coordinator',
     'You are Wassit, an admin coordinator for pharmacists helping coordinate B2B medicine availability enquiries and inventory shield policies across participating branches. Never diagnose, prescribe, or suggest clinical substitutions.',
     'qwen3.5:35b-a3b')
ON CONFLICT (id) DO NOTHING;
