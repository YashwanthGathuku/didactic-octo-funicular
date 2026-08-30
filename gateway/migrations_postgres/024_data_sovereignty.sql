-- 024_data_sovereignty.sql -- PostgreSQL dialect
-- Adds geographic data sovereignty columns to the tenants table.
-- Safe defaults preserve existing us-central1 behavior so no existing row breaks.

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS data_region TEXT NOT NULL DEFAULT 'us-central1';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS allowed_regions JSONB NOT NULL DEFAULT '["us-central1"]'::jsonb;
