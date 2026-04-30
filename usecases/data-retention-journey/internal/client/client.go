package client

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sfmc-retention/internal/models"
)

const (
	defaultTimeout = 60 * time.Second
	maxPageSize    = 50
)

// Credentials holds all auth params extracted from the bash scripts.
// - JBHost:    jbinteractions.s12.marketingcloudapps.com (Journey Builder)
// - MCHost:    mc.s12.marketingcloudapps.com (MC app for PATCH)
// - JBCookie:  ff6288631bbb74b54ce9223b62465d85=... (session cookie for JB)
// - MCCookie:  567c649970167cc328895c8cba7fd270=... (session cookie for MC)
// - JBCSRFToken: X-CSRF-Token for JB calls
// - MCCSRFToken: X-CSRF-Token for MC PATCH call
// - BearerToken: authorization Bearer token for PATCH
type Credentials struct {
	JBHost      string
	MCHost      string
	JBCookie    string
	MCCookie    string
	JBCSRFToken string
	MCCSRFToken string
	BearerToken string
}

// Client wraps the HTTP client with credentials.
type Client struct {
	creds Credentials
	http  *http.Client
	debug bool
}

func New(creds Credentials, debug bool) *Client {
	return &Client{
		creds: creds,
		http:  &http.Client{Timeout: defaultTimeout},
		debug: debug,
	}
}

// decodeCookie URL-decodes a cookie value if it contains percent-encoded chars.
// Browsers copy cookies in encoded form (e.g. %3A, %2F) but the Cookie header
// must contain the decoded value — otherwise SFMC sees a different session key.
func decodeCookie(raw string) string {
	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		return raw // if it fails, use as-is
	}
	return decoded
}

// ─── Ping / Auth Verify ───────────────────────────────────────────────────────

// Ping verifies credentials by fetching page 1 of journeys (1 item).
func (c *Client) Ping() error {
	u := fmt.Sprintf("https://%s/fuelapi/interaction/v1/interactions/?mostRecentVersionOnly=false&$page=1&$pageSize=1", c.creds.JBHost)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	c.setJBHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("authentication failed (HTTP %d): check your JB cookie and CSRF token", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ─── Get All Journeys (paginated) ─────────────────────────────────────────────

func (c *Client) GetAllJourneys() ([]models.Journey, error) {
	var all []models.Journey
	page := 1
	for {
		batch, total, err := c.getJourneysPage(page, maxPageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(all) >= total || len(batch) == 0 {
			break
		}
		page++
	}
	return all, nil
}

func (c *Client) getJourneysPage(page, pageSize int) ([]models.Journey, int, error) {
	u := fmt.Sprintf(
		"https://%s/fuelapi/interaction/v1/interactions/?mostRecentVersionOnly=false&mostRecentVersionOrRunningOnly=true&$page=%d&$pageSize=%d&extras=trigger,stats,tag,activity,campaigns&$orderBy=name%%20asc",
		c.creds.JBHost, page, pageSize,
	)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, 0, err
	}
	c.setJBHeaders(req)

	var result models.JourneyListResponse
	if err := c.doJSON(req, &result); err != nil {
		return nil, 0, fmt.Errorf("get journeys page %d: %w", page, err)
	}
	return result.Items, result.Count, nil
}

// ─── Get Journey Detail ───────────────────────────────────────────────────────

func (c *Client) GetJourneyDetail(id string, version int) (*models.Journey, error) {
	u := fmt.Sprintf(
		"https://%s/fuelapi/interaction/v1/interactions/%s?extras=all&includeStops=true&versionNumber=%d",
		c.creds.JBHost, id, version,
	)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	c.setJBHeaders(req)

	var j models.Journey
	if err := c.doJSON(req, &j); err != nil {
		return nil, fmt.Errorf("get journey detail %s: %w", id, err)
	}
	return &j, nil
}

// ─── Get Event Definition ─────────────────────────────────────────────────────

func (c *Client) GetEventDefinition(eventDefID string) (*models.EventDefinition, error) {
	u := fmt.Sprintf(
		"https://%s/fuelapi/interaction/v1/eventDefinitions/%s",
		c.creds.JBHost, eventDefID,
	)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	c.setJBHeaders(req)

	var ed models.EventDefinition
	if err := c.doJSON(req, &ed); err != nil {
		return nil, fmt.Errorf("get event definition %s: %w", eventDefID, err)
	}
	return &ed, nil
}

// ─── Get Data Extension Detail ────────────────────────────────────────────────

func (c *Client) GetDataExtension(deID string) (*models.DataExtension, error) {
	u := fmt.Sprintf(
		"https://%s/fuelapi/internal/v1/customObjects/%s/",
		c.creds.JBHost, deID,
	)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	c.setJBHeaders(req)

	var de models.DataExtension
	if err := c.doJSON(req, &de); err != nil {
		return nil, fmt.Errorf("get data extension %s: %w", deID, err)
	}
	return &de, nil
}

// ─── Update Data Retention ────────────────────────────────────────────────────

// UpdateDataRetention sets retention to 7 days (row-based, no delete at end).
// Matches the PATCH in 5_Update-Data-Retention.bash.
func (c *Client) UpdateDataRetention(deID string) error {
	u := fmt.Sprintf(
		"https://%s/contactsmeta/fuelapi/internal/v1/customobjects/%s",
		c.creds.MCHost, deID,
	)

	formData := url.Values{}
	formData.Set("dataRetentionProperties[isResetRetentionPeriodOnImport]", "false")
	formData.Set("dataRetentionProperties[isDeleteAtEndOfRetentionPeriod]", "false")
	formData.Set("dataRetentionProperties[isRowBasedRetention]", "true")
	formData.Set("dataRetentionProperties[dataRetentionPeriodLength]", "7")
	formData.Set("dataRetentionProperties[dataRetentionPeriodUnitOfMeasure]", "1") // 1 = Days

	req, err := http.NewRequest("PATCH", u, strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}
	c.setMCHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("update data retention %s: %w", deID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("authentication failed (HTTP %d) updating DE %s", resp.StatusCode, deID)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update failed (HTTP %d) for DE %s: %s", resp.StatusCode, deID, string(body))
	}
	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (c *Client) setJBHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "en-GB,en-US;q=0.9,en;q=0.8")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("x-fueldata-version", "1.1")
	req.Header.Set("Referer", fmt.Sprintf("https://%s/", c.creds.JBHost))
	if c.creds.JBCSRFToken != "" {
		req.Header.Set("X-CSRF-Token", c.creds.JBCSRFToken)
	}
	if c.creds.JBCookie != "" {
		cookie := decodeCookie(c.creds.JBCookie)
		req.Header.Set("Cookie", cookie)
		if c.debug {
			log.Printf("[debug] JB Cookie (decoded): %s\n", truncate(cookie, 60))
		}
	}
	if c.debug {
		log.Printf("[debug] → %s %s\n", req.Method, req.URL.String())
	}
}

func (c *Client) setMCHeaders(req *http.Request) {
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-GB,en-US;q=0.9,en;q=0.8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", fmt.Sprintf("https://%s", c.creds.MCHost))
	req.Header.Set("Referer", fmt.Sprintf("https://%s/contactsmeta/admin.html", c.creds.MCHost))
	req.Header.Set("tz", "accountPreference")
	if c.creds.MCCSRFToken != "" {
		req.Header.Set("X-CSRF-Token", c.creds.MCCSRFToken)
	}
	if c.creds.MCCookie != "" {
		cookie := decodeCookie(c.creds.MCCookie)
		req.Header.Set("Cookie", cookie)
		if c.debug {
			log.Printf("[debug] MC Cookie (decoded): %s\n", truncate(cookie, 60))
		}
	}
	if c.creds.BearerToken != "" {
		req.Header.Set("authorization", fmt.Sprintf("Bearer %s", c.creds.BearerToken))
	}
	if c.debug {
		log.Printf("[debug] → %s %s\n", req.Method, req.URL.String())
	}
}

func (c *Client) doJSON(req *http.Request, out interface{}) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if c.debug {
		log.Printf("[debug] ← HTTP %d | body: %s\n", resp.StatusCode, truncate(string(body), 300))
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("authentication failed (HTTP %d) — body: %s", resp.StatusCode, truncate(string(body), 300))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode JSON: %w — body: %s", err, truncate(string(body), 200))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
