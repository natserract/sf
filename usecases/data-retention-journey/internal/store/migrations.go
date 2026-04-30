package store

import (
	"database/sql"
	"fmt"
)

func EnsureSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS journey_refs (
			run_id TEXT NOT NULL,
			business_unit_id TEXT NOT NULL,
			journey_id TEXT NOT NULL,
			journey_name TEXT,
			event_def_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS event_defs (
			run_id TEXT NOT NULL,
			business_unit_id TEXT NOT NULL,
			journey_id TEXT NOT NULL,
			journey_name TEXT,
			event_def_id TEXT NOT NULL,
			de_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS data_extensions (
			run_id TEXT NOT NULL,
			business_unit_id TEXT NOT NULL,
			de_id TEXT NOT NULL,
			de_name TEXT,
			customer_key TEXT,
			journey_id TEXT,
			journey_name TEXT,
			source_type TEXT,
			is_sendable BOOLEAN,
			is_active BOOLEAN,
			field_count INTEGER,
			total_row_count INTEGER,
			created_date TEXT,
			modified_date TEXT,
			created_by TEXT,
			modified_by TEXT,
			description TEXT,
			retention_period_length INTEGER,
			retention_period_unit INTEGER,
			is_delete_at_end BOOLEAN,
			is_row_based BOOLEAN,
			is_reset_on_import BOOLEAN,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			snapshot_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS update_results (
			run_id TEXT NOT NULL,
			business_unit_id TEXT NOT NULL,
			de_id TEXT NOT NULL,
			de_name TEXT,
			customer_key TEXT,
			journey_id TEXT,
			journey_name TEXT,
			source_type TEXT,
			updated BOOLEAN NOT NULL,
			error_text TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS after_data_extensions (
			run_id TEXT NOT NULL,
			business_unit_id TEXT NOT NULL,
			de_id TEXT NOT NULL,
			de_name TEXT,
			customer_key TEXT,
			journey_id TEXT,
			journey_name TEXT,
			source_type TEXT,
			is_sendable BOOLEAN,
			is_active BOOLEAN,
			field_count INTEGER,
			total_row_count INTEGER,
			created_date TEXT,
			modified_date TEXT,
			created_by TEXT,
			modified_by TEXT,
			description TEXT,
			retention_period_length INTEGER,
			retention_period_unit INTEGER,
			is_delete_at_end BOOLEAN,
			is_row_based BOOLEAN,
			is_reset_on_import BOOLEAN,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			snapshot_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("schema migration failed: %w", err)
		}
	}
	return nil
}
