/**
Data Cloud (CDP) Metadata API service.

Provides access to Data Lake Objects (DLOs) and Data Model Objects (DMOs)
via the Salesforce Data Cloud Metadata API.

API Reference: https://developer.salesforce.com/docs/data/data-cloud-ref/guide/c360a-api-metadata-api.htm

Authentication Flow:
1. Use existing Salesforce access token from OAuth
2. Exchange it for a Data Cloud-specific token via /services/a360/token
3. Use the returned DC instance URL (e.g., https://xxxxx.c360a.salesforce.com) for API calls
**/

package sfmcn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DataCloudTokenResponse represents Salesforce Data Cloud token exchange response.
type DataCloudTokenResponse struct {
	AccessToken     string `json:"access_token"`
	TokenType       string `json:"token_type,omitempty"`
	ExpiresIn       int    `json:"expires_in,omitempty"`
	IssuedTokenType string `json:"issued_token_type,omitempty"`
	InstanceURL     string `json:"instance_url,omitempty"`
}

// ExchangeDataCloudToken exchanges a core Salesforce access token for a Data Cloud token.
// If subjectToken is empty, it retrieves one from Authenticate()/cache flow.
func (s *Salesforce) ExchangeDataCloudToken(ctx context.Context, subjectToken string) (*DataCloudTokenResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if subjectToken == "" {
		var err error
		subjectToken, err = s.getAccessToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get subject token: %w", err)
		}
	}

	requestURL := fmt.Sprintf("%s/services/a360/token", s.config.BaseURI)
	headers := map[string]string{
		"Content-Type":  "application/x-www-form-urlencoded",
		"Authorization": "Bearer " + subjectToken,
	}
	body := map[string]string{
		"grant_type":         "urn:salesforce:grant-type:external:cdp",
		"subject_token":      subjectToken,
		"subject_token_type": "urn:ietf:params:oauth:token-type:access_token",
	}

	req, err := s.PrepareRequest(ctx, http.MethodPost, requestURL, headers, nil, body)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare data cloud token exchange request: %w", err)
	}

	resp, err := s.CallAPI(req)
	if err != nil {
		return nil, fmt.Errorf("data cloud token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read data cloud token exchange response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("data cloud token exchange failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp DataCloudTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse data cloud token exchange response: %w", err)
	}

	return &tokenResp, nil
}
