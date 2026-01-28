package sfmcn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	httpclient "github.com/natserract/sf/pkg/http"
	"go.uber.org/zap"
)

// Salesforce is the main client for interacting with Salesforce Marketing Cloud API
type Salesforce struct {
	config     *Config
	httpClient *httpclient.Client
	tokenCache *tokenCache
	logger     *zap.Logger
}

// tokenCache manages the OAuth access token with thread-safe access
type tokenCache struct {
	mu          sync.RWMutex
	accessToken string
	expiresAt   time.Time
}

// NewSalesforce creates a new Salesforce client with default production logger
func NewSalesforce(cfg *Config) *Salesforce {
	logger, _ := zap.NewProduction()
	return &Salesforce{
		config:     cfg,
		httpClient: httpclient.NewClientWithLogger(logger),
		tokenCache: &tokenCache{},
		logger:     logger,
	}
}

// NewSalesforceWithLogger creates a new Salesforce client with a custom logger
func NewSalesforceWithLogger(cfg *Config, logger *zap.Logger) *Salesforce {
	return &Salesforce{
		config:     cfg,
		httpClient: httpclient.NewClientWithLogger(logger),
		tokenCache: &tokenCache{},
		logger:     logger,
	}
}

// PrepareRequest creates an *http.Request suitable for passing to CallAPI.
// - urlOrPath may be an absolute URL or a relative path (resolved against Config.BaseURI in CallAPI).
// - body defaults to JSON encoding unless Content-Type is application/x-www-form-urlencoded.
func (s *Salesforce) PrepareRequest(
	ctx context.Context,
	method string,
	urlOrPath string,
	headers map[string]string,
	queryParams map[string]string,
	body interface{},
) (*http.Request, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if method == "" {
		return nil, fmt.Errorf("method is required")
	}
	if urlOrPath == "" {
		return nil, fmt.Errorf("urlOrPath is required")
	}

	u, err := url.Parse(urlOrPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse urlOrPath: %w", err)
	}

	if len(queryParams) > 0 {
		q := u.Query()
		for k, v := range queryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		contentType := ""
		if headers != nil {
			contentType = headers["Content-Type"]
			if contentType == "" {
				contentType = headers["content-type"]
			}
		}

		switch v := body.(type) {
		case io.Reader:
			bodyReader = v
		case []byte:
			bodyReader = bytes.NewReader(v)
		case string:
			bodyReader = strings.NewReader(v)
		default:
			if strings.HasPrefix(strings.ToLower(contentType), "application/x-www-form-urlencoded") {
				form := url.Values{}
				switch vv := body.(type) {
				case url.Values:
					form = vv
				case map[string]string:
					for k, val := range vv {
						form.Set(k, val)
					}
				case map[string]interface{}:
					for k, val := range vv {
						if val == nil {
							continue
						}
						form.Set(k, fmt.Sprint(val))
					}
				default:
					b, err := json.Marshal(body)
					if err != nil {
						return nil, fmt.Errorf("failed to marshal form body: %w", err)
					}
					var m map[string]interface{}
					if err := json.Unmarshal(b, &m); err != nil {
						return nil, fmt.Errorf("failed to unmarshal form body: %w", err)
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
				b, err := json.Marshal(body)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal json body: %w", err)
				}
				bodyReader = bytes.NewReader(b)
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Apply headers passed by caller.
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Sensible defaults when a body is present.
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	return req, nil
}

func (s *Salesforce) CallAPI(request *http.Request) (*http.Response, error) {
	if request == nil {
		return nil, http.ErrMissingFile
	}

	// If caller provided a relative URL, resolve it against BaseURI.
	if request.URL != nil && !request.URL.IsAbs() && s.config != nil && s.config.BaseURI != "" {
		base, err := url.Parse(s.config.BaseURI)
		if err != nil {
			return nil, err
		}
		request.URL = base.ResolveReference(request.URL)
	}

	token, err := s.getAccessToken(context.Background())
	if err != nil {
		s.logger.Error("Failed to get access token", zap.Error(err))
		return nil, err
	}

	// Add Authorization header unless caller already set it.
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	if request.Header.Get("Authorization") == "" && token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if request.Header.Get("Accept") == "" {
		request.Header.Set("Accept", "application/json")
	}

	resp, err := s.httpClient.DoRequest(request)
	if err != nil {
		s.logger.Error("Call API request failed", zap.Error(err), zap.String("url", request.URL.String()), zap.String("method", request.Method))
		return nil, err
	}

	return resp, nil
}
