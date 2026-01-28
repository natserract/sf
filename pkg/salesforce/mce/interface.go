package sfmce

// SalesforceClient defines the interface for Salesforce API operations
type SalesforceClient interface {
	// Authenticate retrieves an OAuth access token
	Authenticate() (*AuthResponse, error)

	// GetFolders retrieves all folders matching the allowed types
	GetFolders() (*FoldersResponse, error)

	// GetSubFolders retrieves subfolders for a given category ID
	GetSubFolders(folderID string) (*FoldersResponse, error)

	// GetDataExtensions retrieves data extensions for a given category ID with pagination
	GetDataExtensions(folderID string, page, pageSize int) (*DataExtensionsResponse, error)

	// UpdateDataRetention updates the data retention properties for a data extension
	UpdateDataRetention(dataExtensionID string, retention *DataRetentionProperties) error
}
