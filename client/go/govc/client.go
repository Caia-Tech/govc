package govc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is the main client for interacting with a govc server
type Client struct {
	baseURL    string
	token      string
	apiKey     string
	httpClient *http.Client
	userAgent  string
}

// ClientOption configures a Client
type ClientOption func(*Client)

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithToken sets the JWT token for authentication
func WithToken(token string) ClientOption {
	return func(c *Client) {
		c.token = token
	}
}

// WithAPIKey sets the API key for authentication
func WithAPIKey(apiKey string) ClientOption {
	return func(c *Client) {
		c.apiKey = apiKey
	}
}

// WithUserAgent sets a custom user agent
func WithUserAgent(userAgent string) ClientOption {
	return func(c *Client) {
		c.userAgent = userAgent
	}
}

// NewClient creates a new govc client
func NewClient(baseURL string, opts ...ClientOption) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL cannot be empty")
	}

	// Ensure baseURL has no trailing slash
	baseURL = strings.TrimRight(baseURL, "/")

	// Validate URL
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("invalid baseURL: %w", err)
	}

	c := &Client{
		baseURL:   baseURL,
		userAgent: "govc-client/1.0",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// doRequest performs an HTTP request with authentication
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	url := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Add authentication
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	} else if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// parseResponse parses the response and handles errors
func (c *Client) parseResponse(resp *http.Response, result interface{}) error {
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return fmt.Errorf("server returned %d: failed to parse error response", resp.StatusCode)
		}
		return &errResp
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// get performs a GET request
func (c *Client) get(ctx context.Context, path string, result interface{}) error {
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return c.parseResponse(resp, result)
}

// post performs a POST request
func (c *Client) post(ctx context.Context, path string, body, result interface{}) error {
	resp, err := c.doRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	return c.parseResponse(resp, result)
}

// put performs a PUT request
func (c *Client) put(ctx context.Context, path string, body, result interface{}) error {
	resp, err := c.doRequest(ctx, http.MethodPut, path, body)
	if err != nil {
		return err
	}
	return c.parseResponse(resp, result)
}

// delete performs a DELETE request
func (c *Client) delete(ctx context.Context, path string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	return c.parseResponse(resp, nil)
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func (e *ErrorResponse) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Error)
	}
	return e.Error
}

// HealthCheck performs a health check on the server
func (c *Client) HealthCheck(ctx context.Context) (*HealthResponse, error) {
	var health HealthResponse
	err := c.get(ctx, "/api/v1/health", &health)
	return &health, err
}

// HealthResponse represents the server health status
type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Uptime    string            `json:"uptime"`
	Version   string            `json:"version"`
	Checks    map[string]string `json:"checks"`
}