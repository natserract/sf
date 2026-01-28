-- Migration: 001_initial_schema.sql
-- Description: Initial database schema creation
-- Created: 2025-01-XX

-- This migration file is a copy of schema.sql for version control
-- In production, you would run migrations sequentially

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Folders table
CREATE TABLE IF NOT EXISTS folders (
    id VARCHAR(255) PRIMARY KEY,
    type VARCHAR(100) NOT NULL,
    last_updated TIMESTAMP WITH TIME ZONE NOT NULL,
    created_by INTEGER NOT NULL DEFAULT 0,
    parent_id VARCHAR(255),
    name VARCHAR(500) NOT NULL,
    description TEXT,
    icon_type VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_parent_folder FOREIGN KEY (parent_id) REFERENCES folders(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_folders_parent_id ON folders(parent_id);
CREATE INDEX IF NOT EXISTS idx_folders_type ON folders(type);
CREATE INDEX IF NOT EXISTS idx_folders_name ON folders(name);

-- Data Extensions table
CREATE TABLE IF NOT EXISTS data_extensions (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(500) NOT NULL,
    key VARCHAR(500) NOT NULL UNIQUE,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    is_sendable BOOLEAN NOT NULL DEFAULT false,
    sendable_custom_object_field VARCHAR(500),
    sendable_subscriber_field VARCHAR(500),
    is_testable BOOLEAN NOT NULL DEFAULT false,
    category_id VARCHAR(255) NOT NULL,
    owner_id INTEGER NOT NULL,
    is_object_deletable BOOLEAN NOT NULL DEFAULT true,
    is_field_addition_allowed BOOLEAN NOT NULL DEFAULT true,
    is_field_modification_allowed BOOLEAN NOT NULL DEFAULT true,
    created_date TIMESTAMP WITH TIME ZONE,
    created_by_id INTEGER NOT NULL,
    created_by_name VARCHAR(500),
    modified_date TIMESTAMP WITH TIME ZONE,
    modified_by_id INTEGER,
    modified_by_name VARCHAR(500),
    owner_name VARCHAR(500),
    partner_api_object_type_id INTEGER,
    partner_api_object_type_name VARCHAR(500),
    row_count INTEGER NOT NULL DEFAULT 0,
    field_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_data_extension_category FOREIGN KEY (category_id) REFERENCES folders(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_data_extensions_category_id ON data_extensions(category_id);
CREATE INDEX IF NOT EXISTS idx_data_extensions_key ON data_extensions(key);
CREATE INDEX IF NOT EXISTS idx_data_extensions_modified_date ON data_extensions(modified_date DESC);
CREATE INDEX IF NOT EXISTS idx_data_extensions_name ON data_extensions(name);

-- Data Retention Properties table
CREATE TABLE IF NOT EXISTS data_retention_properties (
    data_extension_id VARCHAR(255) PRIMARY KEY,
    data_retention_period_length INTEGER NOT NULL DEFAULT 0,
    data_retention_period_unit_of_measure INTEGER NOT NULL DEFAULT 0,
    is_delete_at_end_of_retention_period BOOLEAN NOT NULL DEFAULT false,
    is_row_based_retention BOOLEAN NOT NULL DEFAULT false,
    is_reset_retention_period_on_import BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_retention_data_extension FOREIGN KEY (data_extension_id) REFERENCES data_extensions(id) ON DELETE CASCADE
);

-- Message Queue table
CREATE TABLE IF NOT EXISTS message_queue (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    priority INTEGER NOT NULL DEFAULT 5,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 5,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMP WITH TIME ZONE,
    next_retry_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT chk_message_status CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'dead_letter'))
);

CREATE INDEX IF NOT EXISTS idx_message_queue_status ON message_queue(status);
CREATE INDEX IF NOT EXISTS idx_message_queue_next_retry_at ON message_queue(next_retry_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_message_queue_priority ON message_queue(priority, created_at);
CREATE INDEX IF NOT EXISTS idx_message_queue_type ON message_queue(message_type);
CREATE INDEX IF NOT EXISTS idx_message_queue_created_at ON message_queue(created_at);

-- Message History table
CREATE TABLE IF NOT EXISTS message_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id UUID NOT NULL,
    message_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,
    payload JSONB,
    error_message TEXT,
    processing_duration_ms INTEGER,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_message_history_message FOREIGN KEY (message_id) REFERENCES message_queue(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_message_history_message_id ON message_history(message_id);
CREATE INDEX IF NOT EXISTS idx_message_history_status ON message_history(status);
CREATE INDEX IF NOT EXISTS idx_message_history_created_at ON message_history(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_message_history_type ON message_history(message_type);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers (drop if exists to make idempotent)
DROP TRIGGER IF EXISTS update_folders_updated_at ON folders;
CREATE TRIGGER update_folders_updated_at BEFORE UPDATE ON folders
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_data_extensions_updated_at ON data_extensions;
CREATE TRIGGER update_data_extensions_updated_at BEFORE UPDATE ON data_extensions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_data_retention_properties_updated_at ON data_retention_properties;
CREATE TRIGGER update_data_retention_properties_updated_at BEFORE UPDATE ON data_retention_properties
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

