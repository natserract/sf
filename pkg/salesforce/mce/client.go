// Package salesforce provides a client for interacting with Salesforce Marketing Cloud Engagement (MCE) API.
//
// Marketing Cloud Engagement (MCE), formerly known as Marketing Cloud, is Salesforce's
// enterprise marketing automation platform. MCE enables organizations to create, manage,
// and execute personalized marketing campaigns across multiple channels including email,
// SMS, push notifications, social media, and advertising.
//
// Key features of MCE include:
//   - Data Extensions: Custom data tables for storing subscriber and marketing data
//   - Email Studio: Email campaign creation and management
//   - Journey Builder: Visual workflow automation for customer journeys
//   - Automation Studio: Scheduled and triggered automation workflows
//   - Contact Builder: Unified customer data model
//   - Analytics: Campaign performance tracking and reporting
//
// This package provides a Go client for interacting with the Marketing Cloud REST API,
// handling authentication, token management, and API requests for various MCE resources
// such as data extensions, folders, and other marketing cloud entities.
package sfmce

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
