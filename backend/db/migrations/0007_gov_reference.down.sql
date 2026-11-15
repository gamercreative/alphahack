-- Reverts 0007_gov_reference. Uploaded files under UPLOAD_DIR/reference are left on disk.
DROP TABLE IF EXISTS gov_products;
DROP TABLE IF EXISTS reference_documents;
