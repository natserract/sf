package sfmce

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

func LoadConfig() (*Config, error) {
	// Try to load .env file, but don't fail if it doesn't exist
	_ = godotenv.Load()

	cfg := &Config{
		AuthBaseURI:  os.Getenv("MCE_AUTH_BASE_URI"),
		RestBaseURI:  os.Getenv("MCE_REST_BASE_URI"),
		ClientID:     os.Getenv("MCE_CLIENT_ID"),
		ClientSecret: os.Getenv("MCE_CLIENT_SECRET"),
		Scope:        os.Getenv("MCE_SCOPE"),
		AccountID:    os.Getenv("MCE_ACCOUNT_ID"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.AuthBaseURI == "" {
		return fmt.Errorf("MCE_AUTH_BASE_URI is required")
	}
	if c.RestBaseURI == "" {
		return fmt.Errorf("MCE_REST_BASE_URI is required")
	}
	if c.ClientID == "" {
		return fmt.Errorf("MCE_CLIENT_ID is required")
	}
	if c.ClientSecret == "" {
		return fmt.Errorf("MCE_CLIENT_SECRET is required")
	}
	if c.Scope == "" {
		return fmt.Errorf("MCE_SCOPE is required")
	}
	// AccountID is optional, so we don't validate it
	return nil
}
