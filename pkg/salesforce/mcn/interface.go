package sfmcn

// SalesforceClient defines the interface for Salesforce API operations
type SalesforceClient interface {
	// Authenticate retrieves an OAuth access token
	Authenticate() (*AuthResponse, error)
}
