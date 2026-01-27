-- Migration: 003_add_retention_update_tracking.sql
-- Description: Add tracking fields to data_retention_properties table for API update tracking
-- Created: 2025-01-XX

-- Add tracking columns to data_retention_properties table
ALTER TABLE data_retention_properties
ADD COLUMN IF NOT EXISTS last_api_update_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN IF NOT EXISTS last_api_update_status VARCHAR(50) DEFAULT 'pending',
ADD COLUMN IF NOT EXISTS last_api_update_error TEXT,
ADD COLUMN IF NOT EXISTS api_update_retry_count INTEGER NOT NULL DEFAULT 0;

-- Add constraint for status values
ALTER TABLE data_retention_properties
DROP CONSTRAINT IF EXISTS chk_api_update_status;

ALTER TABLE data_retention_properties
ADD CONSTRAINT chk_api_update_status CHECK (last_api_update_status IN ('pending', 'succeeded', 'failed'));

-- Add index for querying failed updates
CREATE INDEX IF NOT EXISTS idx_data_retention_properties_api_update_status 
ON data_retention_properties(last_api_update_status) 
WHERE last_api_update_status IN ('pending', 'failed');

-- Add index for querying by last update time
CREATE INDEX IF NOT EXISTS idx_data_retention_properties_last_api_update_at 
ON data_retention_properties(last_api_update_at DESC NULLS LAST);

