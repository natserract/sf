package sfmcn

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	BaseURI      string
	ClientID     string
	ClientSecret string
}

func Load() (*Config, error) {
	// Try to load .env file, but don't fail if it doesn't exist
	_ = godotenv.Load()

	cfg := &Config{
		BaseURI:      os.Getenv("MCN_BASE_URI"),
		ClientID:     os.Getenv("MCN_CLIENT_ID"),
		ClientSecret: os.Getenv("MCN_CLIENT_SECRET"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.BaseURI == "" {
		return fmt.Errorf("MCN_BaseURI is required")
	}
	if c.ClientID == "" {
		return fmt.Errorf("MCN_CLIENT_ID is required")
	}
	if c.ClientSecret == "" {
		return fmt.Errorf("MCN_CLIENT_SECRET is required")
	}
	// AccountID is optional, so we don't validate it
	return nil
}
