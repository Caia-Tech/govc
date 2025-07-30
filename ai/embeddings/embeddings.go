package embeddings

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// EmbeddingProvider defines the interface for embedding providers
type EmbeddingProvider interface {
	GetEmbedding(ctx context.Context, text string) ([]float32, error)
	GetBatchEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
	GetDimensions() int
}

// OpenAIProvider implements embedding using OpenAI's API
type OpenAIProvider struct {
	APIKey    string
	Model     string
	BaseURL   string
	client    *http.Client
	rateLimit chan struct{}
}

// NewOpenAIProvider creates a new OpenAI embedding provider
func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	// Rate limiting: 3000 requests per minute for OpenAI
	rateLimit := make(chan struct{}, 50) // 50 concurrent requests
	for i := 0; i < 50; i++ {
		rateLimit <- struct{}{}
	}
	
	go func() {
		ticker := time.NewTicker(time.Minute / 3000) // Refill rate
		defer ticker.Stop()
		for range ticker.C {
			select {
			case rateLimit <- struct{}{}:
			default:
			}
		}
	}()
	
	return &OpenAIProvider{
		APIKey:    apiKey,
		Model:     "text-embedding-3-small", // 1536 dimensions, cost-effective
		BaseURL:   "https://api.openai.com/v1",
		client:    &http.Client{Timeout: 30 * time.Second},
		rateLimit: rateLimit,
	}
}

type openAIRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type openAIResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func (p *OpenAIProvider) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := p.GetBatchEmbeddings(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return embeddings[0], nil
}

func (p *OpenAIProvider) GetBatchEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("no texts provided")
	}
	
	// Rate limiting
	<-p.rateLimit
	defer func() {
		time.AfterFunc(time.Minute/3000, func() {
			select {
			case p.rateLimit <- struct{}{}:
			default:
			}
		})
	}()
	
	// Clean and truncate texts
	cleanTexts := make([]string, len(texts))
	for i, text := range texts {
		cleanTexts[i] = p.cleanText(text)
		if len(cleanTexts[i]) > 8192 { // OpenAI token limit
			cleanTexts[i] = cleanTexts[i][:8192]
		}
	}
	
	reqBody := openAIRequest{
		Input: cleanTexts,
		Model: p.Model,
	}
	
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("User-Agent", "govc/1.0")
	
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	
	var apiResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	// Sort by index to maintain order
	embeddings := make([][]float32, len(texts))
	for _, data := range apiResp.Data {
		if data.Index < len(embeddings) {
			embeddings[data.Index] = data.Embedding
		}
	}
	
	return embeddings, nil
}

func (p *OpenAIProvider) GetDimensions() int {
	switch p.Model {
	case "text-embedding-3-small":
		return 1536
	case "text-embedding-3-large":
		return 3072
	case "text-embedding-ada-002":
		return 1536
	default:
		return 1536
	}
}

func (p *OpenAIProvider) cleanText(text string) string {
	// Remove excessive whitespace and normalize
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	
	// Split into lines and clean
	lines := strings.Split(text, "\n")
	cleanLines := make([]string, 0, len(lines))
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}
	
	return strings.Join(cleanLines, "\n")
}

// LocalProvider implements a simple local embedding using TF-IDF (for development/testing)
type LocalProvider struct {
	dimensions int
	cache      map[string][]float32
	mu         sync.RWMutex
}

func NewLocalProvider(dimensions int) *LocalProvider {
	if dimensions <= 0 {
		dimensions = 512 // Default dimension
	}
	return &LocalProvider{
		dimensions: dimensions,
		cache:      make(map[string][]float32),
	}
}

func (p *LocalProvider) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := p.GetBatchEmbeddings(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (p *LocalProvider) GetBatchEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	
	for i, text := range texts {
		// Check cache first
		hash := p.hashText(text)
		p.mu.RLock()
		if cached, exists := p.cache[hash]; exists {
			p.mu.RUnlock()
			embeddings[i] = cached
			continue
		}
		p.mu.RUnlock()
		
		// Generate simple embedding based on text characteristics
		embedding := p.generateSimpleEmbedding(text)
		
		// Cache the result
		p.mu.Lock()
		p.cache[hash] = embedding
		p.mu.Unlock()
		
		embeddings[i] = embedding
	}
	
	return embeddings, nil
}

func (p *LocalProvider) GetDimensions() int {
	return p.dimensions
}

func (p *LocalProvider) hashText(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

func (p *LocalProvider) generateSimpleEmbedding(text string) []float32 {
	// Simple deterministic embedding based on text characteristics
	// This is for development/testing only - not production quality
	
	embedding := make([]float32, p.dimensions)
	
	// Use text characteristics to generate features
	textBytes := []byte(strings.ToLower(text))
	
	for i := 0; i < p.dimensions; i++ {
		// Generate pseudo-random but deterministic values
		seed := int64(i)
		for j, b := range textBytes {
			seed = seed*31 + int64(b) + int64(j)
		}
		
		// Normalize to [-1, 1] range
		embedding[i] = float32((seed%2000)-1000) / 1000.0
	}
	
	// Normalize the vector
	var norm float32
	for _, val := range embedding {
		norm += val * val
	}
	norm = float32(1.0 / (1e-8 + float64(norm))) // Avoid division by zero
	
	for i := range embedding {
		embedding[i] *= norm
	}
	
	return embedding
}

// EmbeddingConfig holds configuration for the embedding system
type EmbeddingConfig struct {
	Provider        string            `json:"provider"`         // "openai" or "local"
	OpenAIAPIKey    string            `json:"openai_api_key"`   // OpenAI API key
	Model           string            `json:"model"`            // Model name
	CacheSize       int               `json:"cache_size"`       // Max embeddings to cache
	BatchSize       int               `json:"batch_size"`       // Batch size for processing
	Dimensions      int               `json:"dimensions"`       // Embedding dimensions (for local provider)
	CustomProviders map[string]string `json:"custom_providers"` // Custom provider configurations
}

// DefaultEmbeddingConfig returns sensible defaults
func DefaultEmbeddingConfig() EmbeddingConfig {
	return EmbeddingConfig{
		Provider:   "local", // Safe default for development
		Model:      "text-embedding-3-small",
		CacheSize:  10000,
		BatchSize:  100,
		Dimensions: 512,
	}
}

// NewEmbeddingProvider creates a new embedding provider based on config
func NewEmbeddingProvider(config EmbeddingConfig) (EmbeddingProvider, error) {
	switch strings.ToLower(config.Provider) {
	case "openai":
		if config.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("OpenAI API key required for OpenAI provider")
		}
		provider := NewOpenAIProvider(config.OpenAIAPIKey)
		if config.Model != "" {
			provider.Model = config.Model
		}
		return provider, nil
		
	case "local":
		dimensions := config.Dimensions
		if dimensions <= 0 {
			dimensions = 512
		}
		return NewLocalProvider(dimensions), nil
		
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", config.Provider)
	}
}