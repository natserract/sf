package main

import (
	"sync"
	"time"

	"github.com/natserract/sf/pkg/config"
	httpclient "github.com/natserract/sf/pkg/http"
	"go.uber.org/zap"
)

// Salesforce is the main client for interacting with Salesforce Marketing Cloud API
type Salesforce struct {
	config     *config.Config
	httpClient *httpclient.Client
	tokenCache *tokenCache
	logger     *zap.Logger
}

// tokenCache manages the OAuth access token with thread-safe access
type tokenCache struct {
	mu          sync.RWMutex
	accessToken string
	expiresAt   time.Time
}

// NewSalesforce creates a new Salesforce client with default production logger
func NewSalesforce(cfg *config.Config) *Salesforce {
	logger, _ := zap.NewProduction()
	return &Salesforce{
		config:     cfg,
		httpClient: httpclient.NewClientWithLogger(logger),
		tokenCache: &tokenCache{},
		logger:     logger,
	}
}

// NewSalesforceWithLogger creates a new Salesforce client with a custom logger
func NewSalesforceWithLogger(cfg *config.Config, logger *zap.Logger) *Salesforce {
	return &Salesforce{
		config:     cfg,
		httpClient: httpclient.NewClientWithLogger(logger),
		tokenCache: &tokenCache{},
		logger:     logger,
	}
}
