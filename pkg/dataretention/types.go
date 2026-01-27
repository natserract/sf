package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// APITime is a custom time type that handles Salesforce API date formats
// The API returns dates without timezone (e.g., "2020-09-09T04:04:02.257")
type APITime struct {
	time.Time
}

// UnmarshalJSON implements json.Unmarshaler for APITime
func (t *APITime) UnmarshalJSON(data []byte) error {
	var timeStr string
	if err := json.Unmarshal(data, &timeStr); err != nil {
		return err
	}

	// Handle empty string
	if timeStr == "" {
		t.Time = time.Time{}
		return nil
	}

	// Try different date formats that the API might use
	// First, try RFC3339 formats (with timezone)
	formats := []string{
		time.RFC3339,     // Full RFC3339 with timezone
		time.RFC3339Nano, // RFC3339 with nanoseconds and timezone
	}

	for _, format := range formats {
		if parsed, err := time.Parse(format, timeStr); err == nil {
			t.Time = parsed
			return nil
		}
	}

	// If no timezone, handle formats without timezone
	// Check if string contains milliseconds (has a dot)
	if strings.Contains(timeStr, ".") {
		// Split by dot to separate date/time from milliseconds
		parts := strings.Split(timeStr, ".")
		if len(parts) == 2 {
			// Parse the date/time part (without milliseconds)
			if parsed, err := time.Parse("2006-01-02T15:04:05", parts[0]); err == nil {
				t.Time = parsed
				return nil
			}
		}
	}

	// Try parsing without milliseconds
	if parsed, err := time.Parse("2006-01-02T15:04:05", timeStr); err == nil {
		t.Time = parsed
		return nil
	}

	// If all parsing attempts fail, return an error
	return fmt.Errorf("unable to parse time string: %s", timeStr)
}

// MarshalJSON implements json.Marshaler for APITime
func (t APITime) MarshalJSON() ([]byte, error) {
	if t.Time.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.Time.Format(time.RFC3339))
}

// AuthResponse represents the OAuth token response
type AuthResponse struct {
	AccessToken     string `json:"access_token"`
	TokenType       string `json:"token_type"`
	ExpiresIn       int    `json:"expires_in"`
	Scope           string `json:"scope"`
	RestInstanceURL string `json:"rest_instance_url,omitempty"`
	SoapInstanceURL string `json:"soap_instance_url,omitempty"`
}

// AuthRequest represents the OAuth token request
type AuthRequest struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Scope        string `json:"scope"`
	AccountID    string `json:"account_id,omitempty"`
}

// Folder represents a Salesforce folder entry
type Folder struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	LastUpdated time.Time `json:"lastUpdated"`
	CreatedBy   int       `json:"createdBy"`
	ParentID    string    `json:"parentId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IconType    string    `json:"iconType"`
}

// FoldersResponse represents the response from GetFolders and GetSubFolders
type FoldersResponse struct {
	StartIndex   int      `json:"startIndex"`
	ItemsPerPage int      `json:"itemsPerPage"`
	TotalResults int      `json:"totalResults"`
	Entry        []Folder `json:"entry"`
}

// DataRetentionProperties represents data retention settings
type DataRetentionProperties struct {
	DataRetentionPeriodLength        int  `json:"dataRetentionPeriodLength"`
	DataRetentionPeriodUnitOfMeasure int  `json:"dataRetentionPeriodUnitOfMeasure"`
	IsDeleteAtEndOfRetentionPeriod   bool `json:"isDeleteAtEndOfRetentionPeriod"`
	IsRowBasedRetention              bool `json:"isRowBasedRetention"`
	IsResetRetentionPeriodOnImport   bool `json:"isResetRetentionPeriodOnImport"`
}

// DataExtension represents a Salesforce data extension
type DataExtension struct {
	ID                            string                   `json:"id"`
	Name                          string                   `json:"name"`
	Key                           string                   `json:"key"`
	Description                   string                   `json:"description"`
	IsActive                      bool                     `json:"isActive"`
	IsSendable                    bool                     `json:"isSendable"`
	SendableCustomObjectField     string                   `json:"sendableCustomObjectField,omitempty"`
	SendableSubscriberField       string                   `json:"sendableSubscriberField,omitempty"`
	IsTestable                    bool                     `json:"isTestable"`
	CategoryID                    int                      `json:"categoryId"`
	OwnerID                       int                      `json:"ownerId"`
	IsObjectDeletable             bool                     `json:"isObjectDeletable"`
	IsFieldAdditionAllowed        bool                     `json:"isFieldAdditionAllowed"`
	IsFieldModificationAllowed    bool                     `json:"isFieldModificationAllowed"`
	CreatedDate                   APITime                  `json:"createdDate"`
	CreatedByID                   int                      `json:"createdById"`
	CreatedByName                 string                   `json:"createdByName"`
	ModifiedDate                  APITime                  `json:"modifiedDate"`
	ModifiedByID                  int                      `json:"modifiedById"`
	ModifiedByName                string                   `json:"modifiedByName"`
	OwnerName                     string                   `json:"ownerName"`
	PartnerAPIObjectTypeID        int                      `json:"partnerApiObjectTypeId"`
	PartnerAPIObjectTypeName      string                   `json:"partnerApiObjectTypeName"`
	RowCount                      int                      `json:"rowCount"`
	DataRetentionProperties       *DataRetentionProperties `json:"dataRetentionProperties"`
	FieldCount                    int                      `json:"fieldCount"`
	CategoryIDForRestoringDE      int                      `json:"categoryIDForRestoringDE"`
	CategoryFullPathForRecycleBin *string                  `json:"categoryFullPathForRecyclebin"`
}

// DataExtensionItem represents a single data extension item in the response (legacy structure)
type DataExtensionItem struct {
	DataExtension DataExtension `json:"0"`
}

// DataExtensionsResponse represents the response from GetDataExtensions
type DataExtensionsResponse struct {
	Count    int                    `json:"count"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"pageSize"`
	Links    map[string]interface{} `json:"links"`
	Items    []DataExtension        `json:"items"`
}

// UpdateDataRetentionRequest represents the request body for updating data retention
type UpdateDataRetentionRequest struct {
	DataRetentionProperties *DataRetentionProperties `json:"dataRetentionProperties"`
}
