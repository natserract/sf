package postgres

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// DB wraps the pgx connection pool and provides methods for database operations
type DB struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// Config holds database configuration
type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// NewConfig creates a new database config from environment variables
func NewConfig() *Config {
	sslMode := os.Getenv("DB_SSLMODE")
	if sslMode == "" {
		sslMode = "disable"
	}

	maxConns := int32(25)
	minConns := int32(5)
	maxConnLifetime := 5 * time.Minute
	maxConnIdleTime := 30 * time.Minute

	return &Config{
		Host:            getEnv("DB_HOST", "localhost"),
		Port:            5432,
		User:            getEnv("DB_USER", "postgres"),
		Password:        getEnv("DB_PASSWORD", ""),
		Database:        getEnv("DB_NAME", "sforce"),
		SSLMode:         sslMode,
		MaxConns:        maxConns,
		MinConns:        minConns,
		MaxConnLifetime: maxConnLifetime,
		MaxConnIdleTime: maxConnIdleTime,
	}
}

// New creates a new database connection pool using pgx
func New(cfg *Config, logger *zap.Logger) (*DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode,
	)

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	config.MaxConns = cfg.MaxConns
	config.MinConns = cfg.MinConns
	config.MaxConnLifetime = cfg.MaxConnLifetime
	config.MaxConnIdleTime = cfg.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("Database connection pool established",
		zap.String("host", cfg.Host),
		zap.String("database", cfg.Database),
		zap.Int32("max_conns", cfg.MaxConns))

	return &DB{
		pool:   pool,
		logger: logger,
	}, nil
}

// Close closes the database connection pool
func (db *DB) Close() {
	if db.pool != nil {
		db.pool.Close()
	}
}

// Pool returns the underlying connection pool
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// Ping checks if the database connection is alive
func (db *DB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// BeginTx starts a new transaction
func (db *DB) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	return db.pool.BeginTx(ctx, txOptions)
}

// InitSchema initializes the database schema by executing the schema SQL
func (db *DB) InitSchema(ctx context.Context, schemaSQL string) error {
	db.logger.Info("Initializing database schema")

	_, err := db.pool.Exec(ctx, schemaSQL)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	db.logger.Info("Database schema initialized successfully")
	return nil
}

// InitSchemaFromFile initializes the database schema from a file
func (db *DB) InitSchemaFromFile(ctx context.Context, schemaPath string) error {
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}

	return db.InitSchema(ctx, string(schemaSQL))
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

