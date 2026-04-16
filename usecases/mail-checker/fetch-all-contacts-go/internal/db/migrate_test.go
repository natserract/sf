package db

import (
	"path/filepath"
	"testing"
)

func TestLoadMigrations_FindsInit(t *testing.T) {
	dir := filepath.Join("..", "..", "db", "migrations")
	migs, err := LoadMigrations(dir)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(migs) == 0 {
		t.Fatalf("expected migrations")
	}
	if migs[0].Name != "001_init.sql" {
		t.Fatalf("expected first migration 001_init.sql, got %s", migs[0].Name)
	}
	if migs[len(migs)-1].Name != "005_last_exit_page.sql" {
		t.Fatalf("expected latest migration 005_last_exit_page.sql, got %s", migs[len(migs)-1].Name)
	}
}
