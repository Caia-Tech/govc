package embeddings

import (
	"context"
	"testing"
	"time"
)

func TestDefaultEmbeddingConfig(t *testing.T) {
	config := DefaultEmbeddingConfig()
	
	if config.Provider != "local" {
		t.Errorf("Expected default provider to be 'local', got %s", config.Provider)
	}
	
	if config.Dimensions != 512 {
		t.Errorf("Expected default dimensions to be 512, got %d", config.Dimensions)
	}
	
	if config.Model != "text-embedding-3-small" {
		t.Errorf("Expected default model to be 'text-embedding-3-small', got %s", config.Model)
	}
}

func TestNewEmbeddingProvider_Local(t *testing.T) {
	config := EmbeddingConfig{
		Provider:   "local",
		Dimensions: 256,
	}
	
	provider, err := NewEmbeddingProvider(config)
	if err != nil {
		t.Fatalf("Failed to create local embedding provider: %v", err)
	}
	
	if provider == nil {
		t.Fatal("Provider should not be nil")
	}
	
	// Test dimensions
	if provider.GetDimensions() != 256 {
		t.Errorf("Expected dimensions 256, got %d", provider.GetDimensions())
	}
}

func TestNewEmbeddingProvider_OpenAI(t *testing.T) {
	config := EmbeddingConfig{
		Provider:     "openai",
		OpenAIAPIKey: "test-key",
		Model:        "text-embedding-3-small",
	}
	
	provider, err := NewEmbeddingProvider(config)
	if err != nil {
		t.Fatalf("Failed to create OpenAI embedding provider: %v", err)
	}
	
	if provider == nil {
		t.Fatal("Provider should not be nil")
	}
	
	// OpenAI provider should have correct dimensions
	if provider.GetDimensions() != 1536 {
		t.Errorf("Expected OpenAI dimensions 1536, got %d", provider.GetDimensions())
	}
}

func TestNewEmbeddingProvider_InvalidProvider(t *testing.T) {
	config := EmbeddingConfig{
		Provider: "invalid",
	}
	
	_, err := NewEmbeddingProvider(config)
	if err == nil {
		t.Error("Expected error for invalid provider, got nil")
	}
}

func TestLocalProvider_GetEmbedding(t *testing.T) {
	provider := NewLocalProvider(256)
	
	ctx := context.Background()
	text := "This is a test string for embedding"
	
	embedding, err := provider.GetEmbedding(ctx, text)
	if err != nil {
		t.Fatalf("Failed to get embedding: %v", err)
	}
	
	if len(embedding) != 256 {
		t.Errorf("Expected embedding length 256, got %d", len(embedding))
	}
	
	// Check that embedding values are reasonable (between -1 and 1)
	for i, val := range embedding {
		if val < -1.0 || val > 1.0 {
			t.Errorf("Embedding value at index %d is out of range [-1, 1]: %f", i, val)
		}
	}
}

func TestLocalProvider_GetBatchEmbeddings(t *testing.T) {
	provider := NewLocalProvider(128)
	
	ctx := context.Background()
	texts := []string{
		"First test string",
		"Second test string",
		"Third test string",
	}
	
	embeddings, err := provider.GetBatchEmbeddings(ctx, texts)
	if err != nil {
		t.Fatalf("Failed to get batch embeddings: %v", err)
	}
	
	if len(embeddings) != len(texts) {
		t.Errorf("Expected %d embeddings, got %d", len(texts), len(embeddings))
	}
	
	for i, embedding := range embeddings {
		if len(embedding) != 128 {
			t.Errorf("Embedding %d has wrong length: expected 128, got %d", i, len(embedding))
		}
	}
}

func TestLocalProvider_EmptyText(t *testing.T) {
	provider := NewLocalProvider(256)
	
	ctx := context.Background()
	
	embedding, err := provider.GetEmbedding(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get embedding for empty text: %v", err)
	}
	
	if len(embedding) != 256 {
		t.Errorf("Expected embedding length 256 for empty text, got %d", len(embedding))
	}
}

func TestLocalProvider_Consistency(t *testing.T) {
	provider := NewLocalProvider(128)
	
	ctx := context.Background()
	text := "Consistent test string"
	
	// Get embedding twice
	embedding1, err := provider.GetEmbedding(ctx, text)
	if err != nil {
		t.Fatalf("Failed to get first embedding: %v", err)
	}
	
	embedding2, err := provider.GetEmbedding(ctx, text)
	if err != nil {
		t.Fatalf("Failed to get second embedding: %v", err)
	}
	
	// Embeddings should be identical for the same input
	if len(embedding1) != len(embedding2) {
		t.Error("Embeddings have different lengths")
	}
	
	for i := range embedding1 {
		if embedding1[i] != embedding2[i] {
			t.Errorf("Embeddings differ at index %d: %f vs %f", i, embedding1[i], embedding2[i])
		}
	}
}

func TestOpenAIProvider_MissingAPIKey(t *testing.T) {
	provider := NewOpenAIProvider("")
	
	ctx := context.Background()
	
	_, err := provider.GetEmbedding(ctx, "test")
	if err == nil {
		t.Error("Expected error for missing API key, got nil")
	}
}

func TestOpenAIProvider_GetDimensions(t *testing.T) {
	testCases := []struct {
		model      string
		expectedDim int
	}{
		{"text-embedding-3-small", 1536},
		{"text-embedding-3-large", 3072},
		{"text-embedding-ada-002", 1536},
		{"unknown-model", 1536}, // default
	}
	
	for _, tc := range testCases {
		provider := NewOpenAIProvider("test-key")
		provider.Model = tc.model
		
		dims := provider.GetDimensions()
		if dims != tc.expectedDim {
			t.Errorf("Model %s: expected dimensions %d, got %d", tc.model, tc.expectedDim, dims)
		}
	}
}

func TestEmbeddingCache(t *testing.T) {
	// This would test caching functionality if we implement it
	t.Skip("Caching not implemented yet")
}

func TestContextCancellation(t *testing.T) {
	provider := NewOpenAIProvider("test-key")
	
	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	
	_, err := provider.GetEmbedding(ctx, "test")
	if err == nil {
		t.Error("Expected error for cancelled context, got nil")
	}
}

func TestContextTimeout(t *testing.T) {
	provider := NewOpenAIProvider("test-key")
	
	// Create a context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	
	// Give it a moment to timeout
	time.Sleep(1 * time.Millisecond)
	
	_, err := provider.GetEmbedding(ctx, "test")
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

// Benchmark tests
func BenchmarkLocalProvider_GetEmbedding(b *testing.B) {
	provider := NewLocalProvider(512)
	
	ctx := context.Background()
	text := "This is a benchmark test string for measuring embedding performance"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := provider.GetEmbedding(ctx, text)
		if err != nil {
			b.Fatalf("Benchmark failed: %v", err)
		}
	}
}

func BenchmarkLocalProvider_GetBatchEmbeddings(b *testing.B) {
	provider := NewLocalProvider(256)
	
	ctx := context.Background()
	texts := []string{
		"First benchmark string",
		"Second benchmark string",
		"Third benchmark string",
		"Fourth benchmark string",
		"Fifth benchmark string",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := provider.GetBatchEmbeddings(ctx, texts)
		if err != nil {
			b.Fatalf("Batch benchmark failed: %v", err)
		}
	}
}