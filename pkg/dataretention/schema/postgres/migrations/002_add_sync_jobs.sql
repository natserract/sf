-- Migration: 002_add_sync_jobs.sql
-- Description: Add sync_jobs table for tracking sync operations and metrics
-- Created: 2025-01-XX

-- Sync Jobs table for tracking sync operations
CREATE TABLE IF NOT EXISTS sync_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    total_items INTEGER NOT NULL DEFAULT 0,
    processed_items INTEGER NOT NULL DEFAULT 0,
    succeeded_items INTEGER NOT NULL DEFAULT 0,
    failed_items INTEGER NOT NULL DEFAULT 0,
    error_rate DECIMAL(5, 2) NOT NULL DEFAULT 0.00,
    success_rate DECIMAL(5, 2) NOT NULL DEFAULT 0.00,
    duration_ms INTEGER,
    avg_processing_time_ms INTEGER,
    metadata JSONB,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_sync_job_status CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled'))
);

-- Indexes for sync_jobs table
CREATE INDEX IF NOT EXISTS idx_sync_jobs_status ON sync_jobs(status);
CREATE INDEX IF NOT EXISTS idx_sync_jobs_created_at ON sync_jobs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sync_jobs_job_type ON sync_jobs(job_type);
CREATE INDEX IF NOT EXISTS idx_sync_jobs_status_created_at ON sync_jobs(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sync_jobs_metadata ON sync_jobs USING GIN(metadata);

-- Trigger to update updated_at timestamp (drop if exists to make idempotent)
DROP TRIGGER IF EXISTS update_sync_jobs_updated_at ON sync_jobs;
CREATE TRIGGER update_sync_jobs_updated_at BEFORE UPDATE ON sync_jobs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

