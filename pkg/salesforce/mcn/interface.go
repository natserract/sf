package sfmcn

import (
	"context"
	"net/http"
)

// SalesforceClient defines the interface for Salesforce API operations
type SalesforceClient interface {
	// Authenticate retrieves an OAuth access token
	Authenticate() (*AuthResponse, error)

	// PrepareRequest builds a request that can be executed via CallAPI.
	PrepareRequest(ctx context.Context, method string, urlOrPath string, headers map[string]string, queryParams map[string]string, body interface{}) (*http.Request, error)

	CallAPI(request *http.Request) (*http.Response, error)
}
