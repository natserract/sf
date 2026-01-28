-include .env

export CGO_ENABLED ?= 1
export GOOS = $(shell go env GOOS)

GOTESTSUM_VERSION = 1.9.0
GOLANGCI_VERSION = 1.62.0
BASE_DIRS = .
ROOT_DIR := $(shell pwd)

.PHONY: build
build:
	go build -o sforce main.go

.PHONY: run
run:
	go run -race main.go

.PHONY: export-top-de
export-top-de:
	go run ./cmd/export_top_dataextensions.go

# Database migration targets
# Note: These targets use psql directly. For more advanced migration management,
# consider using golang-migrate (https://github.com/golang-migrate/migrate)

# Load database connection from environment
DB_HOST ?= localhost
DB_PORT ?= 5432
DB_USER ?= postgres
DB_NAME ?= sforce
DB_SSLMODE ?= disable

PSQL = psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d $(DB_NAME) -q

.PHONY: migrate-up
migrate-up:
	@echo "Running database migrations..."
	@$(PSQL) -f schema/postgres/migrations/001_initial_schema.sql 2>&1 | grep -v "NOTICE:" || true
	@$(PSQL) -f schema/postgres/migrations/002_add_sync_jobs.sql 2>&1 | grep -v "NOTICE:" || true
	@$(PSQL) -f schema/postgres/migrations/003_add_retention_update_tracking.sql 2>&1 | grep -v "NOTICE:" || true
	@echo "Migrations completed successfully"

.PHONY: migrate-down
migrate-down:
	@echo "Warning: This will drop all tables. Are you sure? [y/N]" && read ans && [ $${ans:-N} = y ]
	@echo "Dropping all tables..."
	@$(PSQL) -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	@echo "Database schema dropped"

.PHONY: migrate-status
migrate-status:
	@echo "Checking migration status..."
	@$(PSQL) -c "\dt" || echo "No tables found - migrations not applied"
	@$(PSQL) -c "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;" || echo "Unable to check tables"

.PHONY: migrate-create
migrate-create:
	@echo "Creating new migration file..."
	@read -p "Enter migration name (e.g., add_user_table): " name; \
	timestamp=$$(date +%Y%m%d%H%M%S); \
	filename="schema/postgres/migrations/$${timestamp}_$${name}.sql"; \
	echo "-- Migration: $${timestamp}_$${name}.sql" > $$filename; \
	echo "-- Description: " >> $$filename; \
	echo "-- Created: $$(date -u +%Y-%m-%d)" >> $$filename; \
	echo "" >> $$filename; \
	echo "-- Add your migration SQL here" >> $$filename; \
	echo "Migration file created: $$filename"

.PHONY: migrate-reset
migrate-reset:
	@echo "Warning: This will drop and recreate the database schema. Are you sure? [y/N]" && read ans && [ $${ans:-N} = y ]
	@echo "Resetting database..."
	@$(PSQL) -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	@$(PSQL) -f schema/postgres/migrations/001_initial_schema.sql
	@echo "Database reset completed"

.PHONY: db-connect
db-connect:
	@$(PSQL)