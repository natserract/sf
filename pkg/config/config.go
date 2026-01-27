package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AuthBaseURI  string
	RestBaseURI  string
	ClientID     string
	ClientSecret string
	Scope        string
	AccountID    string
}

func Load() (*Config, error) {
	// Try to load .env file, but don't fail if it doesn't exist
	_ = godotenv.Load()

	cfg := &Config{
		AuthBaseURI:  os.Getenv("AUTH_BASE_URI"),
		RestBaseURI:  os.Getenv("REST_BASE_URI"),
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		Scope:        os.Getenv("SCOPE"),
		AccountID:    os.Getenv("ACCOUNT_ID"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.AuthBaseURI == "" {
		return fmt.Errorf("AUTH_BASE_URI is required")
	}
	if c.RestBaseURI == "" {
		return fmt.Errorf("REST_BASE_URI is required")
	}
	if c.ClientID == "" {
		return fmt.Errorf("CLIENT_ID is required")
	}
	if c.ClientSecret == "" {
		return fmt.Errorf("CLIENT_SECRET is required")
	}
	if c.Scope == "" {
		return fmt.Errorf("SCOPE is required")
	}
	// AccountID is optional, so we don't validate it
	return nil
}

