package sfmcn

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// getAccessToken retrieves a valid access token, using cache if available.
// If the token is expired or not available, it calls Authenticate() to get a new token.
// Tokens are valid for 20 minutes, so we cache them and refresh when expired.
func (s *Salesforce) getAccessToken(ctx context.Context) (string, error) {
	s.tokenCache.mu.RLock()
	// Check if we have a valid (non-expired) token
	if s.tokenCache.accessToken != "" && time.Now().Before(s.tokenCache.expiresAt) {
		token := s.tokenCache.accessToken
		remaining := time.Until(s.tokenCache.expiresAt)
		s.tokenCache.mu.RUnlock()
		s.logger.Debug("Using cached access token", zap.Duration("remaining", remaining))
		return token, nil
	}
	s.tokenCache.mu.RUnlock()

	// Token expired or not available, call Authenticate() to get a new token
	// Tokens are valid for 20 minutes, so we need to re-authenticate when expired
	s.logger.Info("Access token expired or not available, authenticating")
	authResp, err := s.Authenticate()
	if err != nil {
		s.logger.Error("Failed to authenticate", zap.Error(err))
		return "", fmt.Errorf("failed to authenticate: %w", err)
	}

	s.tokenCache.mu.Lock()
	s.tokenCache.accessToken = authResp.AccessToken
	s.tokenCache.mu.Unlock()

	s.logger.Info("Successfully authenticated and cached access token",
		zap.Time("expires_at", s.tokenCache.expiresAt))

	return authResp.AccessToken, nil
}

// Authenticate retrieves an OAuth access token
func (s *Salesforce) Authenticate() (*AuthResponse, error) {
	url := fmt.Sprintf("%s/services/oauth2/token", s.config.BaseURI)
	s.logger.Info("Authenticating with Salesforce", zap.String("url", url))

	authReq := AuthRequest{
		GrantType:    "client_credentials",
		ClientID:     s.config.ClientID,
		ClientSecret: s.config.ClientSecret,
	}

	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	}

	resp, err := s.httpClient.Post(context.Background(), url, headers, authReq)
	if err != nil {
		s.logger.Error("Authentication request failed", zap.Error(err), zap.String("url", url))
		return nil, fmt.Errorf("authentication request failed: %w", err)
	}

	if resp.StatusCode != 200 {
		s.logger.Error("Authentication failed",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(resp.Body)))
		return nil, fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}

	var authResp AuthResponse
	if err := json.Unmarshal(resp.Body, &authResp); err != nil {
		s.logger.Error("Failed to parse authentication response", zap.Error(err))
		return nil, fmt.Errorf("failed to parse authentication response: %w", err)
	}

	s.logger.Info("Successfully authenticated",
		zap.String("token_type", authResp.TokenType))

	return &authResp, nil
}
