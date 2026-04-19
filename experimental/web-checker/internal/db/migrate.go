package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Migration struct {
	Name string
	SQL  string
}

func LoadMigrations(migrationsDir string) ([]Migration, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	out := make([]Migration, 0, len(names))
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		out = append(out, Migration{Name: name, SQL: string(b)})
	}
	return out, nil
}

func EnsureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
create table if not exists schema_migrations (
  name text primary key,
  applied_at timestamptz not null default now()
);`)
	return err
}

func AppliedMigrations(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `select name from schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}
	return applied, rows.Err()
}

func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if err := EnsureMigrationsTable(ctx, pool); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	migs, err := LoadMigrations("db/migrations")
	if err != nil {
		return err
	}
	applied, err := AppliedMigrations(ctx, pool)
	if err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}

	for _, m := range migs {
		if applied[m.Name] {
			continue
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, m.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", m.Name, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations(name) values($1)`, m.Name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", m.Name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}
