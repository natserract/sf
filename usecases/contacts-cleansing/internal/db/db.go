package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig holds tunable pool parameters so callers can override defaults
// without touching this package (useful in tests and CLI flags).
type PoolConfig struct {
	// MaxConns caps the total number of open connections in the pool.
	// Rule of thumb: (number of processor instances × 2) + headroom.
	// For a single HistoryProcessor the real concurrency is 2 DB connections
	// (GetContactsForHistory + UpdateHistoryStatus), so 10 is already generous.
	// If you run behind PgBouncer in transaction mode, this can be higher
	// because PgBouncer multiplexes; without it, keep it ≤ 25 to avoid
	// overwhelming Postgres's process scheduler.
	MaxConns int32

	// MinConns keeps this many connections warm so the first query after an
	// idle period does not pay a connection setup cost.
	MinConns int32

	// MaxConnLifetime recycles connections to avoid hitting server-side
	// idle timeouts or load-balancer RSTs on long-running processes.
	MaxConnLifetime time.Duration

	// MaxConnIdleTime closes connections that have been idle longer than this.
	// Keeps the pool lean when load drops after a burst.
	MaxConnIdleTime time.Duration

	// HealthCheckPeriod controls how often the pool pings idle connections.
	// Catches stale TCP connections before a query fails on them.
	HealthCheckPeriod time.Duration
}

// DefaultPoolConfig returns conservative, safe defaults for a single
// HistoryProcessor workload. Adjust MaxConns if you run multiple instances.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConns:          10, // 2 active + headroom; increase per added instance
		MinConns:          2,  // keep 2 warm — matches steady-state usage
		MaxConnLifetime:   30 * time.Minute,
		MaxConnIdleTime:   5 * time.Minute,
		HealthCheckPeriod: 30 * time.Second,
	}
}

func Open(ctx context.Context, dsn string, opts ...PoolConfig) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse db dsn: %w", err)
	}

	pc := DefaultPoolConfig()
	if len(opts) > 0 {
		pc = opts[0]
	}

	cfg.MaxConns = pc.MaxConns
	cfg.MinConns = pc.MinConns
	cfg.MaxConnLifetime = pc.MaxConnLifetime
	cfg.MaxConnIdleTime = pc.MaxConnIdleTime
	cfg.HealthCheckPeriod = pc.HealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}
