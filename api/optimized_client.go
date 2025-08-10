package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
	"compress/gzip"

	"github.com/vmihailenco/msgpack/v5"
)

// OptimizedHTTPClient provides high-performance HTTP client with connection pooling
type OptimizedHTTPClient struct {
	client       *http.Client
	baseURL      string
	mu           sync.RWMutex
	connPool     *ConnectionPool
	binaryMode   bool
	timeout      time.Duration
	retryPolicy  RetryPolicy
}

// ConnectionPool manages HTTP connection pooling
type ConnectionPool struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	DialTimeout         time.Duration
	KeepAliveTimeout    time.Duration
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
}

// ClientOptions configures the optimized client
type ClientOptions struct {
	BaseURL         string
	Timeout         time.Duration
	BinaryMode      bool
	TLSConfig       *tls.Config
	ConnectionPool  *ConnectionPool
	RetryPolicy     *RetryPolicy
	EnableGzip      bool
	UserAgent       string
	MaxRequestSize  int64
	MaxResponseSize int64
}

// NewOptimizedHTTPClient creates a new optimized HTTP client
func NewOptimizedHTTPClient(opts *ClientOptions) *OptimizedHTTPClient {
	if opts == nil {
		opts = &ClientOptions{}
	}
	
	// Set defaults
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	
	if opts.ConnectionPool == nil {
		opts.ConnectionPool = &ConnectionPool{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 30,
			IdleConnTimeout:     90 * time.Second,
			DialTimeout:         10 * time.Second,
			KeepAliveTimeout:    30 * time.Second,
		}
	}
	
	if opts.RetryPolicy == nil {
		opts.RetryPolicy = &RetryPolicy{
			MaxRetries:    3,
			InitialDelay:  100 * time.Millisecond,
			MaxDelay:      5 * time.Second,
			BackoffFactor: 2.0,
		}
	}
	
	// Create optimized transport
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   opts.ConnectionPool.DialTimeout,
			KeepAlive: opts.ConnectionPool.KeepAliveTimeout,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          opts.ConnectionPool.MaxIdleConns,
		MaxIdleConnsPerHost:   opts.ConnectionPool.MaxIdleConnsPerHost,
		IdleConnTimeout:       opts.ConnectionPool.IdleConnTimeout,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		DisableCompression:    !opts.EnableGzip, // Disable if gzip not wanted
		TLSClientConfig:       opts.TLSConfig,
		MaxResponseHeaderBytes: 64 * 1024, // 64KB max headers
	}
	
	client := &http.Client{
		Transport: transport,
		Timeout:   opts.Timeout,
	}
	
	return &OptimizedHTTPClient{
		client:      client,
		baseURL:     opts.BaseURL,
		connPool:    opts.ConnectionPool,
		binaryMode:  opts.BinaryMode,
		timeout:     opts.Timeout,
		retryPolicy: *opts.RetryPolicy,
	}
}

// BinaryRequest represents a binary-encoded request
type BinaryRequest struct {
	Operation string      `msgpack:"operation"`
	Payload   interface{} `msgpack:"payload"`
	RequestID string      `msgpack:"request_id,omitempty"`
}

// BinaryResponse represents a binary-encoded response
type BinaryResponse struct {
	Success   bool        `msgpack:"success"`
	Data      interface{} `msgpack:"data,omitempty"`
	Error     string      `msgpack:"error,omitempty"`
	RequestID string      `msgpack:"request_id,omitempty"`
}

// Post sends a POST request with automatic retry and connection reuse
func (c *OptimizedHTTPClient) Post(ctx context.Context, endpoint string, payload interface{}) ([]byte, error) {
	url := c.baseURL + endpoint
	
	var reqBody []byte
	var contentType string
	var err error
	
	if c.binaryMode {
		// Use MessagePack for binary serialization (3-5x faster than JSON)
		binReq := &BinaryRequest{
			Operation: endpoint,
			Payload:   payload,
		}
		reqBody, err = msgpack.Marshal(binReq)
		if err != nil {
			return nil, fmt.Errorf("msgpack marshal error: %w", err)
		}
		contentType = "application/msgpack"
	} else {
		// Use JSON
		reqBody, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("json marshal error: %w", err)
		}
		contentType = "application/json"
	}
	
	return c.doRequestWithRetry(ctx, "POST", url, reqBody, contentType)
}

// Get sends a GET request with automatic retry and connection reuse
func (c *OptimizedHTTPClient) Get(ctx context.Context, endpoint string) ([]byte, error) {
	url := c.baseURL + endpoint
	return c.doRequestWithRetry(ctx, "GET", url, nil, "")
}

// BatchRequest sends multiple operations in a single request
func (c *OptimizedHTTPClient) BatchRequest(ctx context.Context, operations []BatchOperation) (*BatchResponse, error) {
	batchReq := &BatchRequest{
		Operations: operations,
		Parallel:   true,
	}
	
	respData, err := c.Post(ctx, "/batch", batchReq)
	if err != nil {
		return nil, err
	}
	
	var batchResp BatchResponse
	if c.binaryMode {
		var binResp BinaryResponse
		if err := msgpack.Unmarshal(respData, &binResp); err != nil {
			return nil, fmt.Errorf("msgpack unmarshal error: %w", err)
		}
		
		if !binResp.Success {
			return nil, fmt.Errorf("batch request failed: %s", binResp.Error)
		}
		
		// Convert binary response data to BatchResponse
		respBytes, err := msgpack.Marshal(binResp.Data)
		if err != nil {
			return nil, fmt.Errorf("msgpack marshal intermediate error: %w", err)
		}
		if err := msgpack.Unmarshal(respBytes, &batchResp); err != nil {
			return nil, fmt.Errorf("msgpack unmarshal batch response error: %w", err)
		}
	} else {
		if err := json.Unmarshal(respData, &batchResp); err != nil {
			return nil, fmt.Errorf("json unmarshal error: %w", err)
		}
	}
	
	return &batchResp, nil
}

// doRequestWithRetry executes HTTP request with exponential backoff retry
func (c *OptimizedHTTPClient) doRequestWithRetry(ctx context.Context, method, url string, body []byte, contentType string) ([]byte, error) {
	var lastErr error
	delay := c.retryPolicy.InitialDelay
	
	for attempt := 0; attempt <= c.retryPolicy.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				// Continue with retry
			}
		}
		
		resp, err := c.doRequest(ctx, method, url, body, contentType)
		if err == nil {
			return resp, nil
		}
		
		lastErr = err
		
		// Don't retry on context cancellation or certain HTTP errors
		if ctx.Err() != nil || !c.shouldRetry(err) {
			break
		}
		
		// Exponential backoff
		delay = time.Duration(float64(delay) * c.retryPolicy.BackoffFactor)
		if delay > c.retryPolicy.MaxDelay {
			delay = c.retryPolicy.MaxDelay
		}
	}
	
	return nil, lastErr
}

// doRequest executes a single HTTP request
func (c *OptimizedHTTPClient) doRequest(ctx context.Context, method, url string, body []byte, contentType string) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request error: %w", err)
	}
	
	// Set headers
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", "govc-optimized-client/1.0")
	req.Header.Set("Connection", "keep-alive")
	
	// Accept gzip if enabled
	if !c.client.Transport.(*http.Transport).DisableCompression {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	
	// Execute request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request error: %w", err)
	}
	defer resp.Body.Close()
	
	// Handle gzip decompression
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip reader error: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}
	
	// Read response
	respBody, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read response error: %w", err)
	}
	
	// Check HTTP status
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	
	return respBody, nil
}

// shouldRetry determines if an error is retryable
func (c *OptimizedHTTPClient) shouldRetry(err error) bool {
	// Retry on network errors, timeouts, and 5xx status codes
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout() || netErr.Temporary()
	}
	
	// Check for HTTP status codes that should be retried
	if httpErr, ok := err.(interface{ StatusCode() int }); ok {
		statusCode := httpErr.StatusCode()
		return statusCode >= 500 && statusCode < 600
	}
	
	return false
}

// Close closes the underlying HTTP client and cleans up connections
func (c *OptimizedHTTPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if transport, ok := c.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	
	return nil
}

// GetConnectionStats returns connection pool statistics
func (c *OptimizedHTTPClient) GetConnectionStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	stats := map[string]interface{}{
		"max_idle_conns":          c.connPool.MaxIdleConns,
		"max_idle_conns_per_host": c.connPool.MaxIdleConnsPerHost,
		"idle_conn_timeout":       c.connPool.IdleConnTimeout.String(),
		"dial_timeout":            c.connPool.DialTimeout.String(),
		"keep_alive_timeout":      c.connPool.KeepAliveTimeout.String(),
		"binary_mode":             c.binaryMode,
		"timeout":                 c.timeout.String(),
	}
	
	return stats
}

// EnableBinaryMode switches to binary serialization (MessagePack)
func (c *OptimizedHTTPClient) EnableBinaryMode() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.binaryMode = true
}

// DisableBinaryMode switches to JSON serialization
func (c *OptimizedHTTPClient) DisableBinaryMode() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.binaryMode = false
}

// SetTimeout updates the client timeout
func (c *OptimizedHTTPClient) SetTimeout(timeout time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timeout = timeout
	c.client.Timeout = timeout
}

// WarmupConnections pre-establishes connections to reduce cold start latency
func (c *OptimizedHTTPClient) WarmupConnections(ctx context.Context, endpoints []string) error {
	for _, endpoint := range endpoints {
		go func(ep string) {
			// Make a lightweight request to establish connection
			_, _ = c.Get(ctx, ep)
		}(endpoint)
	}
	
	// Wait a bit for connections to establish
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}