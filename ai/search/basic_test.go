package search

import (
	"context"
	"strings"
	"testing"
)

// Mock embedding provider for basic testing
type basicMockEmbeddingProvider struct {
	dimensions int
}

func (m *basicMockEmbeddingProvider) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Create a simple embedding based on text hash
	embedding := make([]float32, m.dimensions)
	hash := simpleHash(text)
	for i := range embedding {
		embedding[i] = float32((hash+i)%1000) / 1000.0 - 0.5 // Values between -0.5 and 0.5
	}
	return embedding, nil
}

func (m *basicMockEmbeddingProvider) GetBatchEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		embedding, err := m.GetEmbedding(ctx, text)
		if err != nil {
			return nil, err
		}
		embeddings[i] = embedding
	}
	return embeddings, nil
}

func (m *basicMockEmbeddingProvider) GetDimensions() int {
	return m.dimensions
}

func simpleHash(s string) int {
	hash := 0
	for _, c := range s {
		hash = hash*31 + int(c)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}

func TestDefaultSearchConfig(t *testing.T) {
	config := DefaultSearchConfig()
	
	if config.MinSimilarity != 0.3 {
		t.Errorf("Expected MinSimilarity 0.3, got %f", config.MinSimilarity)
	}
	
	if config.MaxResults != 20 {
		t.Errorf("Expected MaxResults 20, got %d", config.MaxResults)
	}
	
	if config.IndexChunkSize != 1000 {
		t.Errorf("Expected IndexChunkSize 1000, got %d", config.IndexChunkSize)
	}
	
	if len(config.SupportedExtensions) == 0 {
		t.Error("Expected supported extensions to be populated")
	}
}

func TestNewSemanticSearchEngine(t *testing.T) {
	provider := &basicMockEmbeddingProvider{dimensions: 256}
	config := DefaultSearchConfig()
	
	engine := NewSemanticSearchEngine(provider, config)
	
	if engine == nil {
		t.Fatal("Engine should not be nil")
	}
	
	if engine.provider != provider {
		t.Error("Provider not set correctly")
	}
	
	if engine.config.MinSimilarity != config.MinSimilarity {
		t.Error("Config not set correctly")
	}
}

func TestSemanticSearchEngine_DetectLanguage(t *testing.T) {
	provider := &basicMockEmbeddingProvider{dimensions: 128}
	config := DefaultSearchConfig()
	engine := NewSemanticSearchEngine(provider, config)
	
	testCases := []struct {
		filename string
		content  string
		expected string
	}{
		{"main.go", "package main\nfunc main() {}", "go"},
		{"script.js", "function test() {}", "javascript"},
		{"app.py", "def main():\n    pass", "python"},
		{"README.md", "# Title\n\nContent", "markdown"},
		{"data.json", `{"key": "value"}`, "json"},
		{"unknown.xyz", "some content", "text"},
	}
	
	for _, tc := range testCases {
		result := engine.detectLanguage(tc.filename, tc.content)
		if result != tc.expected {
			t.Errorf("File %s: expected language %s, got %s", tc.filename, tc.expected, result)
		}
	}
}

func TestSemanticSearchEngine_ChunkContent(t *testing.T) {
	provider := &basicMockEmbeddingProvider{dimensions: 128}
	config := DefaultSearchConfig()
	config.IndexChunkSize = 50 // Small chunk size for testing
	engine := NewSemanticSearchEngine(provider, config)
	
	content := strings.Repeat("This is a test line.\n", 10) // 200+ characters
	language := "go"
	
	chunks := engine.chunkContent(content, language)
	
	if len(chunks) == 0 {
		t.Fatal("Expected at least one chunk")
	}
	
	// Check that chunks don't exceed the limit too much
	for i, chunk := range chunks {
		if len(chunk.Content) > config.IndexChunkSize*2 {
			t.Errorf("Chunk %d is too large: %d characters", i, len(chunk.Content))
		}
	}
	
	// Check that line numbers are set correctly
	if chunks[0].StartLine != 1 {
		t.Errorf("First chunk should start at line 1, got %d", chunks[0].StartLine)
	}
}

func TestSemanticSearchEngine_FilterSupportedFiles(t *testing.T) {
	provider := &basicMockEmbeddingProvider{dimensions: 128}
	config := DefaultSearchConfig()
	engine := NewSemanticSearchEngine(provider, config)
	
	files := []string{
		"main.go",
		"script.js",
		"README.md",
		"binary.exe",
		"image.png",
		"node_modules/package.json",
		"vendor/lib.go",
	}
	
	filtered := engine.filterSupportedFiles(files)
	
	expected := []string{"main.go", "script.js", "README.md"}
	if len(filtered) != len(expected) {
		t.Errorf("Expected %d files, got %d", len(expected), len(filtered))
	}
	
	for i, file := range expected {
		if i >= len(filtered) || filtered[i] != file {
			t.Errorf("Expected file %s at index %d, got %s", file, i, filtered[i])
		}
	}
}

func TestSemanticSearchEngine_CosineSimilarity(t *testing.T) {
	provider := &basicMockEmbeddingProvider{dimensions: 128}
	config := DefaultSearchConfig()
	engine := NewSemanticSearchEngine(provider, config)
	
	// Test identical vectors
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}
	similarity := engine.cosineSimilarity(a, b)
	if similarity < 0.99 || similarity > 1.01 {
		t.Errorf("Identical vectors should have similarity ~1.0, got %f", similarity)
	}
	
	// Test orthogonal vectors
	c := []float32{1.0, 0.0, 0.0}
	d := []float32{0.0, 1.0, 0.0}
	similarity = engine.cosineSimilarity(c, d)
	if similarity < -0.01 || similarity > 0.01 {
		t.Errorf("Orthogonal vectors should have similarity ~0.0, got %f", similarity)
	}
	
	// Test opposite vectors
	e := []float32{1.0, 0.0, 0.0}
	f := []float32{-1.0, 0.0, 0.0}
	similarity = engine.cosineSimilarity(e, f)
	if similarity > -0.99 || similarity < -1.01 {
		t.Errorf("Opposite vectors should have similarity ~-1.0, got %f", similarity)
	}
}

func TestSemanticSearchEngine_GetIndexStats(t *testing.T) {
	provider := &basicMockEmbeddingProvider{dimensions: 64}
	config := DefaultSearchConfig()
	engine := NewSemanticSearchEngine(provider, config)
	
	// Test stats for empty index
	stats := engine.GetIndexStats()
	if stats.TotalFiles != 0 {
		t.Errorf("Empty index should have 0 files, got %d", stats.TotalFiles)
	}
	
	// Version may be empty for empty index, that's ok
	if stats.LastUpdated.IsZero() {
		t.Log("LastUpdated is zero for empty index, which is expected")
	}
}

func TestSemanticSearchEngine_EmptyIndex(t *testing.T) {
	provider := &basicMockEmbeddingProvider{dimensions: 64}
	config := DefaultSearchConfig()
	engine := NewSemanticSearchEngine(provider, config)
	
	ctx := context.Background()
	
	// Try to search without indexing
	_, err := engine.Search(ctx, "test")
	if err == nil {
		t.Error("Expected error when searching empty index")
	}
}

// Benchmark tests
func BenchmarkSemanticSearchEngine_CosineSimilarity(b *testing.B) {
	provider := &basicMockEmbeddingProvider{dimensions: 256}
	config := DefaultSearchConfig()
	engine := NewSemanticSearchEngine(provider, config)
	
	a := make([]float32, 256)
	bVec := make([]float32, 256)
	for i := range a {
		a[i] = float32(i) / 256.0
		bVec[i] = float32(i+1) / 256.0
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.cosineSimilarity(a, bVec)
	}
}

func BenchmarkSemanticSearchEngine_ChunkContent(b *testing.B) {
	provider := &basicMockEmbeddingProvider{dimensions: 256}
	config := DefaultSearchConfig()
	engine := NewSemanticSearchEngine(provider, config)
	
	content := strings.Repeat("func test() { return 42 }\n", 100)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.chunkContent(content, "go")
	}
}