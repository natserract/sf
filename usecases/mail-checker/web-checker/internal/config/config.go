package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	APIBaseURL string
	PageSize   int

	DBDSN string

	MaxInFlight        int
	MaxAttempts        int
	RetryInitial       time.Duration
	RetryMax           time.Duration
	LockTimeout        time.Duration
	IdleSleep          time.Duration
	ReapInterval       time.Duration
}

func FromEnv() (Config, error) {
	base := os.Getenv("API_BASE_URL")
	if base == "" {
		base = "https://mc.s12.marketingcloudapps.com"
	}
	pageSize := 25
	if v := os.Getenv("PAGE_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("PAGE_SIZE must be int: %w", err)
		}
		pageSize = n
	}
	return Config{APIBaseURL: base, PageSize: pageSize}, nil
}

func (c Config) WithDefaults() Config {
	if c.MaxInFlight <= 0 {
		c.MaxInFlight = 50
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 8
	}
	if c.RetryInitial <= 0 {
		c.RetryInitial = time.Second
	}
	if c.RetryMax <= 0 {
		c.RetryMax = time.Minute
	}
	if c.LockTimeout <= 0 {
		c.LockTimeout = 2 * time.Minute
	}
	if c.IdleSleep <= 0 {
		c.IdleSleep = 2 * time.Second
	}
	if c.ReapInterval <= 0 {
		c.ReapInterval = 10 * time.Second
	}
	if c.PageSize <= 0 {
		c.PageSize = 25
	}
	return c
}

func FromEnvFull() (Config, error) {
	c, err := FromEnv()
	if err != nil {
		return Config{}, err
	}
	c.DBDSN = os.Getenv("DB_DSN")

	if v := os.Getenv("MAX_IN_FLIGHT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("MAX_IN_FLIGHT must be int: %w", err)
		}
		c.MaxInFlight = n
	}
	if v := os.Getenv("MAX_ATTEMPTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("MAX_ATTEMPTS must be int: %w", err)
		}
		c.MaxAttempts = n
	}
	if v := os.Getenv("RETRY_INITIAL_MS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("RETRY_INITIAL_MS must be int: %w", err)
		}
		c.RetryInitial = time.Duration(n) * time.Millisecond
	}
	if v := os.Getenv("RETRY_MAX_MS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("RETRY_MAX_MS must be int: %w", err)
		}
		c.RetryMax = time.Duration(n) * time.Millisecond
	}
	if v := os.Getenv("LOCK_TIMEOUT_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("LOCK_TIMEOUT_SECONDS must be int: %w", err)
		}
		c.LockTimeout = time.Duration(n) * time.Second
	}
	if v := os.Getenv("IDLE_SLEEP_MS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("IDLE_SLEEP_MS must be int: %w", err)
		}
		c.IdleSleep = time.Duration(n) * time.Millisecond
	}
	if v := os.Getenv("REAP_INTERVAL_MS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("REAP_INTERVAL_MS must be int: %w", err)
		}
		c.ReapInterval = time.Duration(n) * time.Millisecond
	}

	return c.WithDefaults(), nil
}
