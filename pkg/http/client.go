package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
	"go.uber.org/zap"
)

type Client struct {
	httpClient *http.Client
	logger     *zap.Logger
}

type RequestOptions struct {
	Method          string
	URL             string
	Headers         map[string]string
	Body            interface{}
	Context         context.Context
	MaxRetries      int
	MaxElapsed      time.Duration
	InitialInterval time.Duration
	MaxInterval     time.Duration
}

type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

func NewClient() *Client {
	logger, _ := zap.NewProduction()
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// NewClientWithLogger creates a new HTTP client with a custom logger
func NewClientWithLogger(logger *zap.Logger) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

func (c *Client) Do(opts RequestOptions) (*Response, error) {
	// Set default backoff configuration
	if opts.MaxElapsed == 0 {
		opts.MaxElapsed = 5 * time.Minute
	}
	if opts.InitialInterval == 0 {
		opts.InitialInterval = 100 * time.Millisecond
	}
	if opts.MaxInterval == 0 {
		opts.MaxInterval = 30 * time.Second
	}

	// Create exponential backoff
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = opts.InitialInterval
	expBackoff.MaxInterval = opts.MaxInterval
	expBackoff.Reset()

	// Use context if provided
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	operation := func() (*Response, error) {
		req, err := c.buildRequest(ctx, opts)
		if err != nil {
			c.logger.Error("Failed to build request", zap.Error(err), zap.String("method", opts.Method), zap.String("url", opts.URL))
			return nil, backoff.Permanent(err)
		}

		c.logger.Debug("Making HTTP request",
			zap.String("method", opts.Method),
			zap.String("url", opts.URL))

		httpResp, err := c.httpClient.Do(req)
		if err != nil {
			// Network errors are retryable
			c.logger.Warn("HTTP request failed, will retry",
				zap.Error(err),
				zap.String("method", opts.Method),
				zap.String("url", opts.URL))
			return nil, err
		}
		defer httpResp.Body.Close()

		body, err := io.ReadAll(httpResp.Body)
		if err != nil {
			c.logger.Error("Failed to read response body", zap.Error(err))
			return nil, backoff.Permanent(fmt.Errorf("failed to read response body: %w", err))
		}

		resp := &Response{
			StatusCode: httpResp.StatusCode,
			Headers:    httpResp.Header,
			Body:       body,
		}

		// Check if status code indicates retryable error
		if httpResp.StatusCode >= 500 {
			c.logger.Warn("Server error, will retry",
				zap.Int("status_code", httpResp.StatusCode),
				zap.String("method", opts.Method),
				zap.String("url", opts.URL))
			return nil, fmt.Errorf("server error: %d - %s", httpResp.StatusCode, string(body))
		}

		// 4xx errors are not retryable
		if httpResp.StatusCode >= 400 {
			c.logger.Error("Client error, not retryable",
				zap.Int("status_code", httpResp.StatusCode),
				zap.String("method", opts.Method),
				zap.String("url", opts.URL),
				zap.String("response", string(body)))
			return nil, backoff.Permanent(fmt.Errorf("client error: %d - %s", httpResp.StatusCode, string(body)))
		}

		c.logger.Debug("HTTP request successful",
			zap.Int("status_code", httpResp.StatusCode),
			zap.String("method", opts.Method),
			zap.String("url", opts.URL))

		return resp, nil
	}

	retryOpts := []backoff.RetryOption{
		backoff.WithBackOff(expBackoff),
		backoff.WithMaxElapsedTime(opts.MaxElapsed),
	}

	resp, err := backoff.Retry(ctx, operation, retryOpts...)
	if err != nil {
		c.logger.Error("HTTP request failed after retries",
			zap.Error(err),
			zap.String("method", opts.Method),
			zap.String("url", opts.URL))
		return nil, err
	}

	c.logger.Info("HTTP request completed successfully",
		zap.Int("status_code", resp.StatusCode),
		zap.String("method", opts.Method),
		zap.String("url", opts.URL))

	return resp, nil
}

func (c *Client) buildRequest(ctx context.Context, opts RequestOptions) (*http.Request, error) {
	var bodyReader io.Reader
	if opts.Body != nil {
		if bodyBytes, ok := opts.Body.([]byte); ok {
			bodyReader = bytes.NewReader(bodyBytes)
		} else {
			// If Content-Type explicitly requests form encoding, honor it.
			contentType := opts.Headers["Content-Type"]
			if contentType == "" {
				contentType = opts.Headers["content-type"]
			}

			if strings.HasPrefix(strings.ToLower(contentType), "application/x-www-form-urlencoded") {
				form := url.Values{}

				switch v := opts.Body.(type) {
				case url.Values:
					form = v
				case map[string]string:
					for k, val := range v {
						form.Set(k, val)
					}
				case map[string]interface{}:
					for k, val := range v {
						if val == nil {
							continue
						}
						form.Set(k, fmt.Sprint(val))
					}
				default:
					// Convert structs (or other JSON-marshalable types) into a map first.
					bodyJSON, err := json.Marshal(opts.Body)
					if err != nil {
						return nil, fmt.Errorf("failed to marshal request body: %w", err)
					}
					var m map[string]interface{}
					if err := json.Unmarshal(bodyJSON, &m); err != nil {
						return nil, fmt.Errorf("failed to unmarshal request body: %w", err)
					}
					for k, val := range m {
						if val == nil {
							continue
						}
						form.Set(k, fmt.Sprint(val))
					}
				}

				bodyReader = strings.NewReader(form.Encode())
			} else {
				bodyJSON, err := json.Marshal(opts.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal request body: %w", err)
				}
				bodyReader = bytes.NewReader(bodyJSON)
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, opts.Method, opts.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set default headers
	if opts.Body != nil && opts.Headers["Content-Type"] == "" && opts.Headers["content-type"] == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	// Set custom headers
	for key, value := range opts.Headers {
		req.Header.Set(key, value)
	}

	return req, nil
}

func (c *Client) Get(ctx context.Context, url string, headers map[string]string) (*Response, error) {
	return c.Do(RequestOptions{
		Method:  http.MethodGet,
		URL:     url,
		Headers: headers,
		Context: ctx,
	})
}

func (c *Client) Post(ctx context.Context, url string, headers map[string]string, body interface{}) (*Response, error) {
	return c.Do(RequestOptions{
		Method:  http.MethodPost,
		URL:     url,
		Headers: headers,
		Body:    body,
		Context: ctx,
	})
}

// DoRequest executes a fully-constructed net/http request. This is useful for
// calling endpoints that don't fit the typed helper methods (custom/native APIs).
func (c *Client) DoRequest(req *http.Request) (*http.Response, error) {
	return c.httpClient.Do(req)
}

func (c *Client) Patch(ctx context.Context, url string, headers map[string]string, body interface{}) (*Response, error) {
	return c.Do(RequestOptions{
		Method:  http.MethodPatch,
		URL:     url,
		Headers: headers,
		Body:    body,
		Context: ctx,
	})
}
