package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"

type Auth struct {
	BearerToken string
	CsrfToken   string
	Cookie      string
}

type FetchPageParams struct {
	PageSize int
	Page     int
	OrderBy  string

	FilterConditionOperator string
	FilterConditionValue    string
}

type FetchPageResult struct {
	PageNumber   int
	KeysInserted []string
	EmptyPage    bool
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Client) FetchAllContactsPage(ctx context.Context, auth Auth, p FetchPageParams) (Response, *http.Response, error) {
	u, bodyJSON, err := c.buildRequestURLAndBody("/contactsmeta/fuelapi/contacts/v1/addresses/search/channel/", p.PageSize, p.Page, p.OrderBy, map[string]string{
		"filterConditionOperator": p.FilterConditionOperator,
	})
	if err != nil {
		return Response{}, nil, err
	}
	raw, resp, err := c.doJSONRequest(ctx, auth, u, bodyJSON)
	if err != nil {
		return Response{}, resp, err
	}
	if os.Getenv("API_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "FetchPage raw response: %s\n", string(raw))
	}

	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return Response{}, resp, fmt.Errorf("unmarshal response: %w", err)
	}
	return out, resp, nil
}

func (c *Client) FetchChannelPage(ctx context.Context, auth Auth, p FetchPageParams) (Response, *http.Response, error) {
	op := p.FilterConditionOperator
	if op == "" {
		op = "Is"
	}
	u, bodyJSON, err := c.buildRequestURLAndBody("/contactsmeta/fuelapi/contacts/v1/addresses/search/channel/", p.PageSize, p.Page, p.OrderBy, map[string]string{
		"filterConditionOperator": op,
		"filterConditionValue":    p.FilterConditionValue,
	})
	if err != nil {
		return Response{}, nil, err
	}
	raw, resp, err := c.doJSONRequest(ctx, auth, u, bodyJSON)
	if err != nil {
		return Response{}, resp, err
	}
	if os.Getenv("API_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "FetchPage raw response: %s\n", string(raw))
	}

	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return Response{}, resp, fmt.Errorf("unmarshal response: %w", err)
	}
	return out, resp, nil
}

type FetchCountParams struct {
	PageSize int
	Page     int
	OrderBy  string

	FilterConditionOperator string
	FilterConditionValue    string
}

func (c *Client) FetchAllContactsCount(ctx context.Context, auth Auth, p FetchCountParams) (CountResponse, *http.Response, error) {
	if p.PageSize <= 0 {
		p.PageSize = 25
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.OrderBy == "" {
		p.OrderBy = "contactKey ASC"
	}
	u, bodyJSON, err := c.buildRequestURLAndBody("/contactsmeta/fuelapi/contacts/v1/addresses/count/", p.PageSize, p.Page, p.OrderBy, map[string]any{
		"queryFilter": map[string]any{
			"hasCriteria": true,
			"rootExpressionSet": map[string]any{
				"expressions": []any{},
			},
		},
	})
	if err != nil {
		return CountResponse{}, nil, err
	}
	raw, resp, err := c.doJSONRequest(ctx, auth, u, bodyJSON)
	if err != nil {
		return CountResponse{}, resp, err
	}
	var out CountResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return CountResponse{}, resp, fmt.Errorf("unmarshal count response: %w", err)
	}
	return out, resp, nil
}

func (c *Client) FetchChannelCount(ctx context.Context, auth Auth, p FetchCountParams) (CountResponse, *http.Response, error) {
	if p.PageSize <= 0 {
		p.PageSize = 25
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.OrderBy == "" {
		p.OrderBy = "contactKey ASC"
	}
	u, bodyJSON, err := c.buildRequestURLAndBody("/contactsmeta/fuelapi/contacts/v1/addresses/count/", p.PageSize, p.Page, p.OrderBy, map[string]any{
		"queryFilter": map[string]any{
			"hasCriteria": true,
			"rootExpressionSet": map[string]any{
				"expressions": []any{
					map[string]any{
						"customerDataDefinitionID": 104,
						"operator":                 "Equal",
						"values":                   []string{p.FilterConditionValue},
					},
				},
			},
		},
	})
	if err != nil {
		return CountResponse{}, nil, err
	}
	raw, resp, err := c.doJSONRequest(ctx, auth, u, bodyJSON)
	if err != nil {
		return CountResponse{}, resp, err
	}
	var out CountResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return CountResponse{}, resp, fmt.Errorf("unmarshal count response: %w", err)
	}
	return out, resp, nil
}

func (c *Client) FetchMessageHistory(ctx context.Context, auth Auth, contactID string) (MessageHistoryResponse, *http.Response, error) {
	u := fmt.Sprintf("%s/contactsmeta/fuelapi/contacts-internal/v1/contact/%s/feeds/messageHistory?includeSchema=false", c.baseURL, contactID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return MessageHistoryResponse{}, nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-CSRF-Token", auth.CsrfToken)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(strings.TrimPrefix(auth.BearerToken, "Bearer ")))
	req.Header.Set("Cookie", strings.TrimSpace(strings.TrimPrefix(auth.Cookie, "Cookie:")))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return MessageHistoryResponse{}, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MessageHistoryResponse{}, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return MessageHistoryResponse{}, nil, fmt.Errorf("history api http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out MessageHistoryResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return MessageHistoryResponse{}, resp, fmt.Errorf("unmarshal message history response: %w", err)
	}
	return out, resp, nil
}

func (c *Client) FetchMessageHistoryConcurrent(
	ctx context.Context,
	auth Auth,
	contacts []ContactInfo,
	maxWorkers int,
) ([]string, *http.Response, error) {
	if len(contacts) == 0 {
		return nil, nil, nil
	}

	g, gctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, maxWorkers)
	var mu sync.Mutex
	var withHistory []string
	var resp *http.Response

	for _, contact := range contacts {
		contact := contact
		g.Go(func() error {
			select {
			case sem <- struct{}{}:
			case <-gctx.Done():
				return gctx.Err()
			}
			defer func() { <-sem }()

			// No retry loop here anymore; we handle it in the Processor
			history, response, err := c.FetchMessageHistory(gctx, auth, contact.ContactID)
			if IsAuthError(response) {
				resp = response
			}
			if err != nil {
				return err // Return raw error (including 401/403)
			}

			if len(history.DataSources) > 0 {
				mu.Lock()
				withHistory = append(withHistory, contact.ContactID)
				mu.Unlock()
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, nil, err
	}
	return withHistory, resp, nil
}

func (c *Client) buildRequestURLAndBody(path string, pageSize, page int, orderBy string, body any) (*url.URL, []byte, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse base url: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	q := u.Query()
	q.Set("$pageSize", fmt.Sprintf("%d", pageSize))
	q.Set("$page", fmt.Sprintf("%d", page))
	if orderBy != "" {
		q.Set("$orderBy", orderBy)
	}
	u.RawQuery = q.Encode()

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal body: %w", err)
	}
	return u, bodyJSON, nil
}

func (c *Client) doJSONRequest(ctx context.Context, auth Auth, u *url.URL, bodyJSON []byte) ([]byte, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, nil, fmt.Errorf("new request: %w", err)
	}

	// Match browser/curl more closely; some endpoints are surprisingly picky.
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.baseURL)
	req.Header.Set("Referer", c.baseURL+"/contactsmeta/admin.html")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("tz", "accountPreference")
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	if auth.Cookie != "" {
		cookie := strings.TrimSpace(auth.Cookie)
		cookie = strings.TrimPrefix(cookie, "Cookie:")
		cookie = strings.TrimSpace(cookie)
		req.Header.Set("Cookie", cookie)
	}
	if auth.CsrfToken != "" {
		req.Header.Set("X-CSRF-Token", auth.CsrfToken)
	}
	if auth.BearerToken != "" {
		tok := strings.TrimSpace(auth.BearerToken)
		tok = strings.TrimPrefix(tok, "Bearer ")
		tok = strings.TrimPrefix(tok, "bearer ")
		tok = strings.TrimSpace(tok)
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	if os.Getenv("API_DEBUG") != "" {
		// Avoid leaking secrets; print only high-signal request details.
		fmt.Fprintf(os.Stderr, "API request: %s %s\n", req.Method, req.URL.String())
		fmt.Fprintf(os.Stderr, "API headers: Accept=%q Content-Type=%q Origin=%q Referer=%q X-Requested-With=%q tz=%q User-Agent=%q\n",
			req.Header.Get("Accept"),
			req.Header.Get("Content-Type"),
			req.Header.Get("Origin"),
			req.Header.Get("Referer"),
			req.Header.Get("X-Requested-With"),
			req.Header.Get("tz"),
			req.Header.Get("User-Agent"),
		)
		fmt.Fprintf(os.Stderr, "API auth: cookie_set=%v csrf_set=%v bearer_set=%v\n",
			req.Header.Get("Cookie") != "",
			req.Header.Get("X-CSRF-Token") != "",
			req.Header.Get("Authorization") != "",
		)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, resp, nil
}

type PingAuthParams struct {
	PageSize                int
	FilterConditionOperator string
	FilterConditionValue    string
	OrderBy                 string
}

func (c *Client) PingAuth(ctx context.Context, auth Auth, p PingAuthParams) (*http.Response, error) {
	if p.PageSize <= 0 {
		p.PageSize = 25
	}
	if p.FilterConditionOperator == "" {
		p.FilterConditionOperator = "Is"
	}
	if p.FilterConditionValue == "" {
		p.FilterConditionValue = "MOBILE"
	}
	if p.OrderBy == "" {
		p.OrderBy = "contactKey ASC"
	}
	_, resp, err := c.FetchAllContactsPage(ctx, auth, FetchPageParams{
		PageSize:                p.PageSize,
		Page:                    1,
		OrderBy:                 p.OrderBy,
		FilterConditionOperator: p.FilterConditionOperator,
		FilterConditionValue:    p.FilterConditionValue,
	})
	return resp, err
}

func IsAuthError(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	return resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden
}

func ExtractContactInfo(r Response) ([]ContactInfo, bool) {
	if len(r.Addresses) == 0 {
		return nil, true
	}

	println("Addresses: ", r.Addresses)
	result := make([]ContactInfo, 0, len(r.Addresses))

	for _, a := range r.Addresses {
		key := a.ContactKey.Value
		id := a.ContactID.Value

		if key == "" && id == "" {
			continue
		}

		result = append(result, ContactInfo{
			ContactKey: key,
			ContactID:  id,
		})
	}

	return result, false
}
