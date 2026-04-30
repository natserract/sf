package store

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Config struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

func ConfigFromEnv() Config {
	return Config{
		Host:     envOr("SFMC_DB_HOST", "127.0.0.1"),
		Port:     envOr("SFMC_DB_PORT", "5432"),
		Name:     envOr("SFMC_DB_NAME", "sfmc_retention"),
		User:     envOr("SFMC_DB_USER", "postgres"),
		Password: envOr("SFMC_DB_PASSWORD", ""),
		SSLMode:  envOr("SFMC_DB_SSLMODE", "disable"),
	}
}

func Open(cfg Config) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Name, cfg.User, cfg.Password, cfg.SSLMode,
	)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
