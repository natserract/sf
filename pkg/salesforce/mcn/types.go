package sfmcn

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
	AccessToken    string `json:"access_token"`
	Signature      string `json:"signature"`
	Scope          string `json:"scope"`
	InstanceURL    string `json:"instance_url,omitempty"`
	ID             string `json:"id"`
	TokenType      string `json:"token_type"`
	IssuedAt       string `json:"issued_at"`
	APIInstanceURL string `json:"api_instance_url,omitempty"`
}

// AuthRequest represents the OAuth token request
type AuthRequest struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}
