-- Reverts 0010_package_photos. Files under UPLOAD_DIR/photos are left on disk.
DROP TABLE IF EXISTS package_photos;
