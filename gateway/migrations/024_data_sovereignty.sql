-- 024_data_sovereignty.sql
-- Adds geographic data sovereignty columns to the tenants table.
-- Safe defaults preserve existing us-central1 behavior so no existing row breaks.

ALTER TABLE tenants ADD COLUMN data_region TEXT NOT NULL DEFAULT 'us-central1';
ALTER TABLE tenants ADD COLUMN allowed_regions TEXT NOT NULL DEFAULT '["us-central1"]';
