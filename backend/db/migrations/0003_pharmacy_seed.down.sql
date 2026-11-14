-- Reverts 0003_pharmacy_seed: deletes only the deterministic seed rows
-- (children before parents, because orders/order_items use ON DELETE RESTRICT).
DELETE FROM order_items          WHERE id LIKE 'order-item-nadia-%';
DELETE FROM orders               WHERE id IN ('order-nadia-1');
DELETE FROM refill_subscriptions WHERE id IN ('refill-nadia-glucophage');
DELETE FROM cart_items           WHERE id IN ('cart-item-rami-panadol', 'cart-item-rami-nexium');
DELETE FROM carts                WHERE id IN ('cart-rami-1');
DELETE FROM prescriptions        WHERE id IN ('rx-nadia-glucophage', 'rx-nadia-concor', 'rx-karim-augmentin');
DELETE FROM customers            WHERE id IN ('cust-nadia', 'cust-karim', 'cust-rami');
DELETE FROM inventory_batches    WHERE id LIKE 'batch-byblos-%' OR id LIKE 'batch-hamra-%' OR id LIKE 'batch-baabda-%';
DELETE FROM medications          WHERE id IN ('med-panadol-advance', 'med-brufen-400', 'med-nexium-20',
                                              'med-augmentin-1g', 'med-zithromax-500', 'med-glucophage-1000',
                                              'med-concor-5', 'med-lipitor-20', 'med-ventolin-inh', 'med-lexotanil-3');
DELETE FROM branches             WHERE id IN ('branch-byblos', 'branch-hamra', 'branch-baabda');
