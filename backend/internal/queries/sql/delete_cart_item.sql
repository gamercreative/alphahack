DELETE FROM cart_items
WHERE cart_id = $1 AND medication_id = $2
