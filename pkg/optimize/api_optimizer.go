package optimize

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// APIOptimizer optimizes API performance
type APIOptimizer struct {
	responseCache   *ResponseCache
	rateLimiter     *RateLimiter
	circuitBreaker  *CircuitBreaker
	loadBalancer    *LoadBalancer
	compressor      *ResponseCompressor
	
	stats APIStats
}

// APIStats tracks API performance metrics
type APIStats struct {
	RequestsTotal     atomic.Uint64
	RequestsSuccess   atomic.Uint64
	RequestsFailed    atomic.Uint64
	CacheHits        atomic.Uint64
	CacheMisses      atomic.Uint64
	AvgResponseTime  atomic.Int64 // microseconds
	BytesCompressed  atomic.Uint64
	RateLimited      atomic.Uint64
	CircuitBroken    atomic.Uint64
}

// NewAPIOptimizer creates an API optimizer
func NewAPIOptimizer() *APIOptimizer {
	return &APIOptimizer{
		responseCache:  NewResponseCache(1000, 5*time.Minute),
		rateLimiter:    NewRateLimiter(1000, time.Second),
		circuitBreaker: NewCircuitBreaker(50, 5, 30*time.Second),
		loadBalancer:   NewLoadBalancer(),
		compressor:     NewResponseCompressor(),
	}
}

// ResponseCache caches API responses
type ResponseCache struct {
	cache    *LRUCache
	ttl      time.Duration
	mu       sync.RWMutex
	
	hits     atomic.Uint64
	misses   atomic.Uint64
	evictions atomic.Uint64
}

// CachedResponse stores cached response data
type CachedResponse struct {
	Body       []byte
	Headers    http.Header
	StatusCode int
	Timestamp  time.Time
	TTL        time.Duration
	ETag       string
}

// NewResponseCache creates a response cache
func NewResponseCache(size int, ttl time.Duration) *ResponseCache {
	return &ResponseCache{
		cache: NewLRUCache(size),
		ttl:   ttl,
	}
}

// Get retrieves cached response
func (rc *ResponseCache) Get(key string) (*CachedResponse, bool) {
	data, hit := rc.cache.Get(key)
	if !hit {
		rc.misses.Add(1)
		return nil, false
	}
	
	// Deserialize
	resp := &CachedResponse{}
	if err := json.Unmarshal(data, resp); err != nil {
		return nil, false
	}
	
	// Check TTL
	if time.Since(resp.Timestamp) > resp.TTL {
		rc.evictions.Add(1)
		return nil, false
	}
	
	rc.hits.Add(1)
	return resp, true
}

// Put caches a response
func (rc *ResponseCache) Put(key string, resp *CachedResponse) error {
	resp.Timestamp = time.Now()
	if resp.TTL == 0 {
		resp.TTL = rc.ttl
	}
	
	// Generate ETag if not present
	if resp.ETag == "" {
		resp.ETag = fmt.Sprintf(`"%x"`, time.Now().UnixNano())
	}
	
	// Serialize
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	
	rc.cache.Put(key, data)
	return nil
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	rate     int
	interval time.Duration
	
	buckets map[string]*TokenBucket
	mu      sync.RWMutex
	
	limited atomic.Uint64
}

// TokenBucket implements token bucket algorithm
type TokenBucket struct {
	tokens    int
	capacity  int
	lastFill  time.Time
	fillRate  time.Duration
	mu        sync.Mutex
}

// NewRateLimiter creates a rate limiter
func NewRateLimiter(rate int, interval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		rate:     rate,
		interval: interval,
		buckets:  make(map[string]*TokenBucket),
	}
	
	// Start cleanup routine
	go rl.cleanup()
	
	return rl
}

// Allow checks if request is allowed
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.RLock()
	bucket, exists := rl.buckets[key]
	rl.mu.RUnlock()
	
	if !exists {
		rl.mu.Lock()
		bucket = &TokenBucket{
			tokens:   rl.rate,
			capacity: rl.rate,
			lastFill: time.Now(),
			fillRate: rl.interval / time.Duration(rl.rate),
		}
		rl.buckets[key] = bucket
		rl.mu.Unlock()
	}
	
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	
	// Refill tokens
	now := time.Now()
	elapsed := now.Sub(bucket.lastFill)
	tokensToAdd := int(elapsed / bucket.fillRate)
	
	if tokensToAdd > 0 {
		bucket.tokens = min(bucket.capacity, bucket.tokens+tokensToAdd)
		bucket.lastFill = now
	}
	
	// Check if we have tokens
	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}
	
	rl.limited.Add(1)
	return false
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, bucket := range rl.buckets {
			bucket.mu.Lock()
			if now.Sub(bucket.lastFill) > 10*time.Minute {
				delete(rl.buckets, key)
			}
			bucket.mu.Unlock()
		}
		rl.mu.Unlock()
	}
}

// CircuitBreaker prevents cascading failures
type CircuitBreaker struct {
	threshold      int
	timeout        time.Duration
	resetTimeout   time.Duration
	
	state          atomic.Uint32 // 0=closed, 1=open, 2=half-open
	failures       atomic.Uint32
	successes      atomic.Uint32
	lastFailTime   atomic.Int64
	
	trips          atomic.Uint64
}

const (
	CircuitClosed = iota
	CircuitOpen
	CircuitHalfOpen
)

// NewCircuitBreaker creates a circuit breaker
func NewCircuitBreaker(threshold, successThreshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:    threshold,
		resetTimeout: resetTimeout,
	}
}

// Call executes function with circuit breaker protection
func (cb *CircuitBreaker) Call(ctx context.Context, fn func() error) error {
	state := cb.state.Load()
	
	// Check if circuit is open
	if state == CircuitOpen {
		lastFail := time.Unix(0, cb.lastFailTime.Load())
		if time.Since(lastFail) > cb.resetTimeout {
			// Try half-open
			cb.state.Store(CircuitHalfOpen)
			cb.failures.Store(0)
			cb.successes.Store(0)
		} else {
			cb.trips.Add(1)
			return fmt.Errorf("circuit breaker is open")
		}
	}
	
	// Execute function
	err := fn()
	
	if err != nil {
		cb.failures.Add(1)
		cb.lastFailTime.Store(time.Now().UnixNano())
		
		if cb.failures.Load() >= uint32(cb.threshold) {
			cb.state.Store(CircuitOpen)
			cb.trips.Add(1)
		}
		
		return err
	}
	
	// Success
	cb.successes.Add(1)
	
	if state == CircuitHalfOpen && cb.successes.Load() >= 5 {
		// Close circuit
		cb.state.Store(CircuitClosed)
		cb.failures.Store(0)
	}
	
	return nil
}

// LoadBalancer distributes load across backends
type LoadBalancer struct {
	backends      []*Backend
	algorithm     LoadBalanceAlgorithm
	healthChecker *HealthChecker
	mu            sync.RWMutex
	
	current       atomic.Uint64
}

// Backend represents a backend server
type Backend struct {
	URL           string
	Weight        int
	Active        atomic.Bool
	Connections   atomic.Int64
	LastCheck     time.Time
	ResponseTime  atomic.Int64 // microseconds
}

// LoadBalanceAlgorithm defines load balancing strategy
type LoadBalanceAlgorithm int

const (
	RoundRobin LoadBalanceAlgorithm = iota
	LeastConnections
	WeightedRoundRobin
	ResponseTime
)

// NewLoadBalancer creates a load balancer
func NewLoadBalancer() *LoadBalancer {
	lb := &LoadBalancer{
		backends:      make([]*Backend, 0),
		algorithm:     RoundRobin,
		healthChecker: NewHealthChecker(),
	}
	
	// Start health checking
	go lb.healthCheck()
	
	return lb
}

// AddBackend adds a backend server
func (lb *LoadBalancer) AddBackend(url string, weight int) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	backend := &Backend{
		URL:    url,
		Weight: weight,
	}
	backend.Active.Store(true)
	
	lb.backends = append(lb.backends, backend)
}

// GetBackend selects a backend based on algorithm
func (lb *LoadBalancer) GetBackend() (*Backend, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	
	if len(lb.backends) == 0 {
		return nil, fmt.Errorf("no backends available")
	}
	
	switch lb.algorithm {
	case RoundRobin:
		return lb.roundRobin()
		
	case LeastConnections:
		return lb.leastConnections()
		
	case WeightedRoundRobin:
		return lb.weightedRoundRobin()
		
	case ResponseTime:
		return lb.responseTime()
		
	default:
		return lb.roundRobin()
	}
}

func (lb *LoadBalancer) roundRobin() (*Backend, error) {
	n := lb.current.Add(1)
	
	// Find next active backend
	for i := 0; i < len(lb.backends); i++ {
		idx := int(n) % len(lb.backends)
		backend := lb.backends[idx]
		if backend.Active.Load() {
			return backend, nil
		}
		n++
	}
	
	return nil, fmt.Errorf("no active backends")
}

func (lb *LoadBalancer) leastConnections() (*Backend, error) {
	var selected *Backend
	minConns := int64(^uint64(0) >> 1) // Max int64
	
	for _, backend := range lb.backends {
		if !backend.Active.Load() {
			continue
		}
		
		conns := backend.Connections.Load()
		if conns < minConns {
			minConns = conns
			selected = backend
		}
	}
	
	if selected == nil {
		return nil, fmt.Errorf("no active backends")
	}
	
	return selected, nil
}

func (lb *LoadBalancer) weightedRoundRobin() (*Backend, error) {
	// Simplified weighted round-robin
	return lb.roundRobin()
}

func (lb *LoadBalancer) responseTime() (*Backend, error) {
	var selected *Backend
	minTime := int64(^uint64(0) >> 1) // Max int64
	
	for _, backend := range lb.backends {
		if !backend.Active.Load() {
			continue
		}
		
		respTime := backend.ResponseTime.Load()
		if respTime < minTime {
			minTime = respTime
			selected = backend
		}
	}
	
	if selected == nil {
		return nil, fmt.Errorf("no active backends")
	}
	
	return selected, nil
}

func (lb *LoadBalancer) healthCheck() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		lb.mu.RLock()
		backends := lb.backends
		lb.mu.RUnlock()
		
		for _, backend := range backends {
			go lb.healthChecker.Check(backend)
		}
	}
}

// HealthChecker checks backend health
type HealthChecker struct {
	timeout time.Duration
	path    string
}

// NewHealthChecker creates a health checker
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		timeout: 5 * time.Second,
		path:    "/health",
	}
}

// Check checks backend health
func (hc *HealthChecker) Check(backend *Backend) {
	client := &http.Client{
		Timeout: hc.timeout,
	}
	
	start := time.Now()
	resp, err := client.Get(backend.URL + hc.path)
	elapsed := time.Since(start)
	
	backend.ResponseTime.Store(elapsed.Microseconds())
	backend.LastCheck = time.Now()
	
	if err != nil || resp.StatusCode != http.StatusOK {
		backend.Active.Store(false)
	} else {
		backend.Active.Store(true)
	}
	
	if resp != nil {
		resp.Body.Close()
	}
}

// ResponseCompressor compresses responses
type ResponseCompressor struct {
	minSize   int
	level     int
	
	compressed atomic.Uint64
	saved      atomic.Uint64
}

// NewResponseCompressor creates a response compressor
func NewResponseCompressor() *ResponseCompressor {
	return &ResponseCompressor{
		minSize: 1024, // Don't compress below 1KB
		level:   gzip.DefaultCompression,
	}
}

// ShouldCompress checks if response should be compressed
func (rc *ResponseCompressor) ShouldCompress(contentType string, size int) bool {
	if size < rc.minSize {
		return false
	}
	
	// Check content type
	compressible := []string{
		"text/", "application/json", "application/xml",
		"application/javascript", "application/x-javascript",
	}
	
	for _, prefix := range compressible {
		if strings.HasPrefix(contentType, prefix) {
			return true
		}
	}
	
	return false
}

// CompressHandler wraps handler with compression
func (rc *ResponseCompressor) CompressHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next(w, r)
			return
		}
		
		// Wrap response writer
		gzw := &gzipResponseWriter{
			ResponseWriter: w,
			compressor:     rc,
		}
		defer gzw.Close()
		
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length") // Will be different after compression
		
		next(gzw, r)
	}
}

// gzipResponseWriter wraps http.ResponseWriter with gzip compression
type gzipResponseWriter struct {
	http.ResponseWriter
	writer     *gzip.Writer
	compressor *ResponseCompressor
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if w.writer == nil {
		w.writer = gzip.NewWriter(w.ResponseWriter)
	}
	
	n, err := w.writer.Write(b)
	w.compressor.compressed.Add(uint64(n))
	w.compressor.saved.Add(uint64(len(b) - n))
	
	return len(b), err // Return original length to caller
}

func (w *gzipResponseWriter) Close() error {
	if w.writer != nil {
		return w.writer.Close()
	}
	return nil
}

// RequestCoalescer coalesces duplicate requests
type RequestCoalescer struct {
	inflight map[string]*inflightRequest
	mu       sync.RWMutex
	
	coalesced atomic.Uint64
}

type inflightRequest struct {
	result chan interface{}
	err    chan error
	count  int
}

// NewRequestCoalescer creates a request coalescer
func NewRequestCoalescer() *RequestCoalescer {
	return &RequestCoalescer{
		inflight: make(map[string]*inflightRequest),
	}
}

// Do executes function, coalescing duplicate requests
func (rc *RequestCoalescer) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	rc.mu.RLock()
	req, exists := rc.inflight[key]
	rc.mu.RUnlock()
	
	if exists {
		// Request already in flight, wait for result
		rc.coalesced.Add(1)
		select {
		case result := <-req.result:
			return result, nil
		case err := <-req.err:
			return nil, err
		}
	}
	
	// Create new inflight request
	rc.mu.Lock()
	req = &inflightRequest{
		result: make(chan interface{}, 1),
		err:    make(chan error, 1),
	}
	rc.inflight[key] = req
	rc.mu.Unlock()
	
	// Execute function
	go func() {
		result, err := fn()
		
		rc.mu.Lock()
		delete(rc.inflight, key)
		rc.mu.Unlock()
		
		if err != nil {
			req.err <- err
		} else {
			req.result <- result
		}
		
		close(req.result)
		close(req.err)
	}()
	
	// Wait for result
	select {
	case result := <-req.result:
		return result, nil
	case err := <-req.err:
		return nil, err
	}
}

// HTTPClient optimized HTTP client
type HTTPClient struct {
	client    *http.Client
	transport *http.Transport
	pool      *MemoryPool
	
	stats struct {
		requests  atomic.Uint64
		errors    atomic.Uint64
		totalTime atomic.Int64
	}
}

// NewHTTPClient creates optimized HTTP client
func NewHTTPClient() *HTTPClient {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false, // Let server handle compression
	}
	
	return &HTTPClient{
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		transport: transport,
		pool:      NewMemoryPool(),
	}
}

// Do performs optimized HTTP request
func (hc *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	start := time.Now()
	hc.stats.requests.Add(1)
	
	resp, err := hc.client.Do(req)
	
	elapsed := time.Since(start)
	hc.stats.totalTime.Add(elapsed.Microseconds())
	
	if err != nil {
		hc.stats.errors.Add(1)
		return nil, err
	}
	
	return resp, nil
}

// ReadBody reads response body with pooled buffer
func (hc *HTTPClient) ReadBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	
	// Use pooled buffer
	buf := hc.pool.Get(32 * 1024)
	defer hc.pool.Put(buf)
	
	return io.ReadAll(resp.Body)
}

// Stats returns API optimizer statistics
func (ao *APIOptimizer) Stats() map[string]interface{} {
	return map[string]interface{}{
		"requests_total":     ao.stats.RequestsTotal.Load(),
		"requests_success":   ao.stats.RequestsSuccess.Load(),
		"requests_failed":    ao.stats.RequestsFailed.Load(),
		"cache_hits":         ao.stats.CacheHits.Load(),
		"cache_misses":       ao.stats.CacheMisses.Load(),
		"avg_response_time":  ao.stats.AvgResponseTime.Load(),
		"bytes_compressed":   ao.stats.BytesCompressed.Load(),
		"rate_limited":       ao.stats.RateLimited.Load(),
		"circuit_broken":     ao.stats.CircuitBroken.Load(),
		"cache_hit_ratio":    float64(ao.stats.CacheHits.Load()) / float64(ao.stats.CacheHits.Load()+ao.stats.CacheMisses.Load()),
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}