-- 0003_pharmacy_seed: deterministic demo data for the B2C pharmacy agent.
-- Every row has a fixed id and is inserted with ON CONFLICT (id) DO NOTHING,
-- so re-running is harmless. Prices are USD cents.
--
-- Edge cases built in for agent tests:
--   * FEFO: several batches of the same medication per branch with different expiries.
--   * Recalled batch: batch-byblos-ventolin-r (must never be sold).
--   * Expired batch:  batch-byblos-lipitor-x (expiry 2025-12-01, must never be sold).
--   * Stock-out: med-concor-5 has 0 available in branch-byblos, 25 in branch-hamra.
--   * Allergies: cust-nadia (penicillin) vs med-augmentin-1g; cust-karim (nsaid) vs med-brufen-400.

-- ---- branches ----------------------------------------------------------------
INSERT INTO branches (id, name, city, address, phone, supports_delivery) VALUES
    ('branch-byblos', 'Cornilia Pharmacy Byblos', 'Byblos', 'Main Road, near Old Souk, Jbeil', '+961 9 540 123', true),
    ('branch-hamra',  'Cornilia Pharmacy Hamra',  'Beirut - Hamra', 'Hamra Street, Bliss crossing, Beirut', '+961 1 350 456', true),
    ('branch-baabda', 'Cornilia Pharmacy Baabda', 'Baabda', 'Baabda Main Road, near Serail', '+961 5 920 789', false)
ON CONFLICT (id) DO NOTHING;

-- ---- medications -------------------------------------------------------------
INSERT INTO medications (id, brand_name, active_ingredient, therapeutic_class, dosage_form, strength,
                         rx_required, controlled, chronic_eligible, unit_price_cents, contraindication_tags, description) VALUES
    ('med-panadol-advance', 'Panadol Advance', 'Paracetamol', 'Analgesic / Antipyretic', 'tablet', '500mg',
        false, false, false, 450, '["liver_disease"]'::jsonb,
        'Pain and fever relief. Max 4 g paracetamol per day.'),
    ('med-brufen-400', 'Brufen', 'Ibuprofen', 'NSAID', 'tablet', '400mg',
        false, false, false, 600, '["nsaid", "peptic_ulcer", "asthma"]'::jsonb,
        'Anti-inflammatory pain relief. Take with food.'),
    ('med-nexium-20', 'Nexium', 'Esomeprazole', 'Proton Pump Inhibitor', 'tablet', '20mg',
        false, false, false, 1450, '[]'::jsonb,
        'Heartburn and acid reflux relief.'),
    ('med-augmentin-1g', 'Augmentin', 'Amoxicillin/Clavulanate', 'Antibiotic (Penicillin)', 'tablet', '1000mg',
        true, false, false, 1200, '["penicillin"]'::jsonb,
        'Broad-spectrum penicillin antibiotic. Prescription required.'),
    ('med-zithromax-500', 'Zithromax', 'Azithromycin', 'Antibiotic (Macrolide)', 'tablet', '500mg',
        true, false, false, 1100, '["macrolide"]'::jsonb,
        'Macrolide antibiotic. Prescription required.'),
    ('med-glucophage-1000', 'Glucophage', 'Metformin', 'Antidiabetic (Biguanide)', 'tablet', '1000mg',
        true, false, true, 650, '["severe_renal_impairment"]'::jsonb,
        'Type 2 diabetes first-line therapy. Chronic refill eligible.'),
    ('med-concor-5', 'Concor', 'Bisoprolol', 'Beta Blocker', 'tablet', '5mg',
        true, false, true, 900, '["severe_asthma", "bradycardia"]'::jsonb,
        'Hypertension and heart failure. Chronic refill eligible.'),
    ('med-lipitor-20', 'Lipitor', 'Atorvastatin', 'Statin', 'tablet', '20mg',
        true, false, true, 2400, '["active_liver_disease", "pregnancy"]'::jsonb,
        'Cholesterol lowering. Chronic refill eligible.'),
    ('med-ventolin-inh', 'Ventolin', 'Salbutamol', 'Bronchodilator (SABA)', 'inhaler', '100mcg',
        false, false, true, 750, '[]'::jsonb,
        'Rescue inhaler for asthma. Chronic refill eligible.'),
    ('med-lexotanil-3', 'Lexotanil', 'Bromazepam', 'Benzodiazepine', 'tablet', '3mg',
        true, true, false, 500, '["respiratory_insufficiency", "myasthenia_gravis"]'::jsonb,
        'Controlled anxiolytic. Original prescription required; pharmacist review only.')
ON CONFLICT (id) DO NOTHING;

-- ---- inventory batches -------------------------------------------------------
INSERT INTO inventory_batches (id, branch_id, medication_id, batch_number, expiry_date,
                               quantity_on_hand, quantity_reserved, is_recalled, recall_reason) VALUES
    -- Byblos
    ('batch-byblos-panadol-a',    'branch-byblos', 'med-panadol-advance', 'PAN-2701A', '2027-03-31T00:00:00Z', 40, 0, false, ''),
    ('batch-byblos-panadol-b',    'branch-byblos', 'med-panadol-advance', 'PAN-2801B', '2028-01-31T00:00:00Z', 60, 0, false, ''),
    ('batch-byblos-brufen-a',     'branch-byblos', 'med-brufen-400',      'BRU-2706A', '2027-06-30T00:00:00Z', 30, 0, false, ''),
    ('batch-byblos-nexium-a',     'branch-byblos', 'med-nexium-20',       'NEX-2709A', '2027-09-30T00:00:00Z', 20, 0, false, ''),
    ('batch-byblos-augmentin-a',  'branch-byblos', 'med-augmentin-1g',    'AUG-2705A', '2027-05-31T00:00:00Z', 15, 0, false, ''),
    ('batch-byblos-glucophage-a', 'branch-byblos', 'med-glucophage-1000', 'GLU-2706A', '2027-06-30T00:00:00Z', 50, 0, false, ''),
    ('batch-byblos-glucophage-b', 'branch-byblos', 'med-glucophage-1000', 'GLU-2802B', '2028-02-29T00:00:00Z', 40, 0, false, ''),
    ('batch-byblos-concor-a',     'branch-byblos', 'med-concor-5',        'CON-2704A', '2027-04-30T00:00:00Z',  0, 0, false, ''),
    ('batch-byblos-ventolin-r',   'branch-byblos', 'med-ventolin-inh',    'VEN-2703R', '2027-03-31T00:00:00Z', 25, 0, true,  'MoPH Batch Quality Hold #2026-11'),
    ('batch-byblos-ventolin-b',   'branch-byblos', 'med-ventolin-inh',    'VEN-2711B', '2027-11-30T00:00:00Z', 10, 0, false, ''),
    ('batch-byblos-lipitor-x',    'branch-byblos', 'med-lipitor-20',      'LIP-2512X', '2025-12-01T00:00:00Z', 12, 0, false, ''),
    ('batch-byblos-lexotanil-a',  'branch-byblos', 'med-lexotanil-3',     'LEX-2708A', '2027-08-31T00:00:00Z', 10, 0, false, ''),
    -- Hamra
    ('batch-hamra-panadol-a',     'branch-hamra',  'med-panadol-advance', 'PAN-2704H', '2027-04-30T00:00:00Z', 100, 0, false, ''),
    ('batch-hamra-brufen-a',      'branch-hamra',  'med-brufen-400',      'BRU-2710H', '2027-10-31T00:00:00Z', 25, 0, false, ''),
    ('batch-hamra-augmentin-a',   'branch-hamra',  'med-augmentin-1g',    'AUG-2712H', '2027-12-31T00:00:00Z', 20, 0, false, ''),
    ('batch-hamra-zithromax-a',   'branch-hamra',  'med-zithromax-500',   'ZIT-2709H', '2027-09-30T00:00:00Z', 18, 0, false, ''),
    ('batch-hamra-glucophage-a',  'branch-hamra',  'med-glucophage-1000', 'GLU-2708H', '2027-08-31T00:00:00Z', 30, 0, false, ''),
    ('batch-hamra-concor-a',      'branch-hamra',  'med-concor-5',        'CON-2709H', '2027-09-30T00:00:00Z', 25, 0, false, ''),
    ('batch-hamra-lipitor-a',     'branch-hamra',  'med-lipitor-20',      'LIP-2803H', '2028-03-31T00:00:00Z', 22, 0, false, ''),
    ('batch-hamra-ventolin-a',    'branch-hamra',  'med-ventolin-inh',    'VEN-2802H', '2028-02-29T00:00:00Z', 15, 0, false, ''),
    -- Baabda
    ('batch-baabda-panadol-a',    'branch-baabda', 'med-panadol-advance', 'PAN-2707D', '2027-07-31T00:00:00Z', 35, 0, false, ''),
    ('batch-baabda-nexium-a',     'branch-baabda', 'med-nexium-20',       'NEX-2801D', '2028-01-31T00:00:00Z', 15, 0, false, ''),
    ('batch-baabda-zithromax-a',  'branch-baabda', 'med-zithromax-500',   'ZIT-2706D', '2027-06-30T00:00:00Z',  6, 0, false, ''),
    ('batch-baabda-glucophage-a', 'branch-baabda', 'med-glucophage-1000', 'GLU-2705D', '2027-05-31T00:00:00Z', 12, 0, false, ''),
    ('batch-baabda-lipitor-a',    'branch-baabda', 'med-lipitor-20',      'LIP-2710D', '2027-10-31T00:00:00Z', 10, 0, false, ''),
    ('batch-baabda-ventolin-a',   'branch-baabda', 'med-ventolin-inh',    'VEN-2709D', '2027-09-30T00:00:00Z',  8, 0, false, '')
ON CONFLICT (id) DO NOTHING;

-- ---- customers ---------------------------------------------------------------
INSERT INTO customers (id, full_name, phone, email, address, allergies, chronic_conditions) VALUES
    ('cust-nadia', 'Nadia Khoury', '+961 3 111 222', 'nadia.khoury@example.com',
        'Rue des Croisés, Byblos', '["penicillin"]'::jsonb, '["hypertension", "type_2_diabetes"]'::jsonb),
    ('cust-karim', 'Karim Haddad', '+961 70 333 444', 'karim.haddad@example.com',
        'Makdessi Street, Hamra, Beirut', '["nsaid"]'::jsonb, '["asthma"]'::jsonb),
    ('cust-rami', 'Rami Saad', '+961 71 555 666', 'rami.saad@example.com',
        'Old Port Road, Byblos', '[]'::jsonb, '[]'::jsonb)
ON CONFLICT (id) DO NOTHING;

-- ---- prescriptions -----------------------------------------------------------
INSERT INTO prescriptions (id, customer_id, medication_id, doctor_name, dosage_instructions,
                           allowed_refills, remaining_refills, status, notes, expires_at) VALUES
    ('rx-nadia-glucophage', 'cust-nadia', 'med-glucophage-1000', 'Dr. Elie Mansour',
        '1 tablet twice daily with meals', 6, 5, 'verified', 'Chronic therapy, reviewed 2026-10.', '2027-05-01T00:00:00Z'),
    ('rx-nadia-concor', 'cust-nadia', 'med-concor-5', 'Dr. Elie Mansour',
        '1 tablet every morning', 6, 6, 'verified', 'Chronic therapy, reviewed 2026-10.', '2027-05-01T00:00:00Z'),
    ('rx-karim-augmentin', 'cust-karim', 'med-augmentin-1g', 'Dr. Rana Fakhoury',
        '1 tablet every 12 hours for 7 days', 1, 1, 'pending', 'Uploaded photo awaiting pharmacist verification.', '2026-12-14T00:00:00Z')
ON CONFLICT (id) DO NOTHING;

-- ---- carts -------------------------------------------------------------------
INSERT INTO carts (id, customer_id, branch_id, status) VALUES
    ('cart-rami-1', 'cust-rami', 'branch-byblos', 'active')
ON CONFLICT (id) DO NOTHING;

INSERT INTO cart_items (id, cart_id, medication_id, prescription_id, quantity, unit_price_cents) VALUES
    ('cart-item-rami-panadol', 'cart-rami-1', 'med-panadol-advance', NULL, 2, 450),
    ('cart-item-rami-nexium',  'cart-rami-1', 'med-nexium-20',       NULL, 1, 1450)
ON CONFLICT (id) DO NOTHING;

-- ---- orders (history) --------------------------------------------------------
INSERT INTO orders (id, customer_id, branch_id, cart_id, status, fulfillment_type, delivery_address, total_cents,
                    created_at, updated_at) VALUES
    ('order-nadia-1', 'cust-nadia', 'branch-byblos', NULL, 'completed', 'delivery', 'Rue des Croisés, Byblos', 3750,
        '2026-10-28T09:30:00Z', '2026-10-28T14:00:00Z')
ON CONFLICT (id) DO NOTHING;

INSERT INTO order_items (id, order_id, medication_id, batch_id, quantity, unit_price_cents, created_at) VALUES
    ('order-item-nadia-1-glucophage', 'order-nadia-1', 'med-glucophage-1000', 'batch-byblos-glucophage-a', 3, 650, '2026-10-28T09:30:00Z'),
    ('order-item-nadia-1-concor',     'order-nadia-1', 'med-concor-5',        'batch-byblos-concor-a',     2, 900, '2026-10-28T09:30:00Z')
ON CONFLICT (id) DO NOTHING;

-- ---- refill subscriptions ----------------------------------------------------
INSERT INTO refill_subscriptions (id, customer_id, medication_id, branch_id, prescription_id,
                                  interval_days, next_refill_at, status) VALUES
    ('refill-nadia-glucophage', 'cust-nadia', 'med-glucophage-1000', 'branch-byblos', 'rx-nadia-glucophage',
        30, '2026-11-27T08:00:00Z', 'active')
ON CONFLICT (id) DO NOTHING;
