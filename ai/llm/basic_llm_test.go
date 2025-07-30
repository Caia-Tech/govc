package llm

import (
	"context"
	"testing"
)

// Basic tests that don't require full repository integration

func TestModelInfo(t *testing.T) {
	info := ModelInfo{
		Name:        "test-model",
		Provider:    "test-provider",
		MaxTokens:   1000,
		ContextSize: 4000,
		Cost:        "free",
	}
	
	if info.Name != "test-model" {
		t.Errorf("Expected Name 'test-model', got %s", info.Name)
	}
	
	if info.Provider != "test-provider" {
		t.Errorf("Expected Provider 'test-provider', got %s", info.Provider)
	}
	
	if info.MaxTokens != 1000 {
		t.Errorf("Expected MaxTokens 1000, got %d", info.MaxTokens)
	}
}

func TestGenerationOptions(t *testing.T) {
	options := GenerationOptions{
		MaxTokens:   500,
		Temperature: 0.7,
		TopP:        0.9,
		Stop:        []string{"END"},
	}
	
	if options.MaxTokens != 500 {
		t.Errorf("Expected MaxTokens 500, got %d", options.MaxTokens)
	}
	
	if options.Temperature != 0.7 {
		t.Errorf("Expected Temperature 0.7, got %f", options.Temperature)
	}
	
	if options.TopP != 0.9 {
		t.Errorf("Expected TopP 0.9, got %f", options.TopP)
	}
	
	if len(options.Stop) != 1 || options.Stop[0] != "END" {
		t.Errorf("Expected Stop ['END'], got %v", options.Stop)
	}
}

func TestDefaultCommitMessageConfig(t *testing.T) {
	config := DefaultCommitMessageConfig()
	
	if config.Style != "conventional" {
		t.Errorf("Expected Style 'conventional', got %s", config.Style)
	}
	
	if config.MaxLength != 72 {
		t.Errorf("Expected MaxLength 72, got %d", config.MaxLength)
	}
	
	if !config.IncludeFiles {
		t.Error("Expected IncludeFiles to be true")
	}
	
	if !config.IncludeFunctions {
		t.Error("Expected IncludeFunctions to be true")
	}
	
	if config.Language != "en" {
		t.Errorf("Expected Language 'en', got %s", config.Language)
	}
}

func TestDefaultCodeReviewConfig(t *testing.T) {
	config := DefaultCodeReviewConfig()
	
	if config.Provider != "local" {
		t.Errorf("Expected Provider 'local', got %s", config.Provider)
	}
	
	if config.ReviewDepth != "detailed" {
		t.Errorf("Expected ReviewDepth 'detailed', got %s", config.ReviewDepth)
	}
	
	if config.AutoComment {
		t.Error("Expected AutoComment to be false")
	}
	
	if config.SeverityThreshold != "warning" {
		t.Errorf("Expected SeverityThreshold 'warning', got %s", config.SeverityThreshold)
	}
}

func TestDefaultConflictResolutionConfig(t *testing.T) {
	config := DefaultConflictResolutionConfig()
	
	if config.Provider != "local" {
		t.Errorf("Expected Provider 'local', got %s", config.Provider)
	}
	
	if config.Confidence != 0.8 {
		t.Errorf("Expected Confidence 0.8, got %f", config.Confidence)
	}
	
	if config.AutoApply {
		t.Error("Expected AutoApply to be false")
	}
	
	if !config.BackupEnabled {
		t.Error("Expected BackupEnabled to be true")
	}
}

// Test basic LLM provider interface
type mockBasicLLMProvider struct {
	responses map[string]string
}

func (m *mockBasicLLMProvider) GenerateText(ctx context.Context, prompt string, options GenerationOptions) (string, error) {
	if response, exists := m.responses[prompt]; exists {
		return response, nil
	}
	return "mock response for: " + prompt, nil
}

func (m *mockBasicLLMProvider) GetModelInfo() ModelInfo {
	return ModelInfo{
		Name:        "mock-model",
		Provider:    "mock",
		MaxTokens:   1000,
		ContextSize: 4000,
		Cost:        "free",
	}
}

func TestLLMProviderInterface(t *testing.T) {
	provider := &mockBasicLLMProvider{
		responses: map[string]string{
			"test prompt": "test response",
		},
	}
	
	ctx := context.Background()
	options := GenerationOptions{MaxTokens: 100}
	
	// Test GenerateText
	response, err := provider.GenerateText(ctx, "test prompt", options)
	if err != nil {
		t.Fatalf("GenerateText failed: %v", err)
	}
	
	if response != "test response" {
		t.Errorf("Expected 'test response', got '%s'", response)
	}
	
	// Test GetModelInfo
	info := provider.GetModelInfo()
	if info.Name != "mock-model" {
		t.Errorf("Expected model name 'mock-model', got '%s'", info.Name)
	}
	
	if info.Provider != "mock" {
		t.Errorf("Expected provider 'mock', got '%s'", info.Provider)
	}
}

func TestCommitMessageGenerator_Creation(t *testing.T) {
	provider := &mockBasicLLMProvider{
		responses: make(map[string]string),
	}
	config := DefaultCommitMessageConfig()
	
	generator := NewCommitMessageGenerator(provider, config)
	
	if generator == nil {
		t.Fatal("Generator should not be nil")
	}
	
	if generator.provider != provider {
		t.Error("Provider not set correctly")
	}
	
	if generator.config.Style != config.Style {
		t.Error("Config not set correctly")
	}
}

func TestCodeReviewer_Creation(t *testing.T) {
	provider := &mockBasicLLMProvider{
		responses: make(map[string]string),
	}
	config := DefaultCodeReviewConfig()
	
	reviewer := NewCodeReviewer(provider, config)
	
	if reviewer == nil {
		t.Fatal("Reviewer should not be nil")
	}
	
	if reviewer.provider != provider {
		t.Error("Provider not set correctly")
	}
	
	if reviewer.config.ReviewDepth != config.ReviewDepth {
		t.Error("Config not set correctly")
	}
}

func TestConflictResolver_Creation(t *testing.T) {
	provider := &mockBasicLLMProvider{
		responses: make(map[string]string),
	}
	config := DefaultConflictResolutionConfig()
	
	resolver := NewConflictResolver(provider, config)
	
	if resolver == nil {
		t.Fatal("Resolver should not be nil")
	}
	
	if resolver.provider != provider {
		t.Error("Provider not set correctly")
	}
	
	if resolver.config.Confidence != config.Confidence {
		t.Error("Config not set correctly")
	}
}

// Test context handling
func TestContextCancellation(t *testing.T) {
	provider := &mockBasicLLMProvider{
		responses: make(map[string]string),
	}
	
	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	
	options := GenerationOptions{MaxTokens: 100}
	
	// This should handle context cancellation gracefully
	_, err := provider.GenerateText(ctx, "test", options)
	
	// Note: Our mock doesn't actually check context cancellation
	// In a real implementation, this would return an error
	if err != nil {
		t.Logf("Context cancellation handled: %v", err)
	} else {
		t.Log("Mock provider doesn't handle context cancellation")
	}
}