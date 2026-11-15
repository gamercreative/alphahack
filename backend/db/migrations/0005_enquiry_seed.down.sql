-- Reverts 0005_enquiry_seed.
DELETE FROM agents WHERE id = '00000000-0000-4000-8000-000000000001';
DELETE FROM staff WHERE id IN ('staff-hamra','staff-achrafieh','staff-baabda','staff-verdun','staff-byblos');
DELETE FROM inventory_batches WHERE id IN ('batch-achrafieh-augmentin-1g','batch-baabda-augmentin-625','batch-verdun-augmentin-1g','batch-verdun-panadol');
DELETE FROM medications WHERE id = 'med-augmentin-625';
DELETE FROM branches WHERE id IN ('branch-achrafieh','branch-verdun');
UPDATE branches SET lat = NULL, lng = NULL;
