package history

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type AuthInput struct {
	BearerToken string `json:"bearerToken"`
	CsrfToken   string `json:"csrfToken"`
	Cookie      string `json:"cookie"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: 25 * time.Second,
		},
	}
}

func (c *Client) FetchMessageHistory(ctx context.Context, auth AuthInput, contactID string) ([]byte, error) {
	u := fmt.Sprintf("%s/contactsmeta/fuelapi/contacts-internal/v1/contact/%s/feeds/messageHistory", c.baseURL, contactID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-CSRF-Token", auth.CsrfToken)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(strings.TrimPrefix(auth.BearerToken, "Bearer ")))
	req.Header.Set("Cookie", strings.TrimSpace(strings.TrimPrefix(auth.Cookie, "Cookie:")))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("history api http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
