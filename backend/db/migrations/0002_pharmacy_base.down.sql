-- Reverts 0002_pharmacy_base (children before parents).
DROP TABLE IF EXISTS refill_subscriptions;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS cart_items;
DROP TABLE IF EXISTS carts;
DROP TABLE IF EXISTS prescriptions;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS inventory_batches;
DROP TABLE IF EXISTS medications;
DROP TABLE IF EXISTS branches;
