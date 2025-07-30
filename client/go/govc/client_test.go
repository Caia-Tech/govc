package govc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		opts    []ClientOption
		wantErr bool
	}{
		{
			name:    "valid URL",
			baseURL: "https://api.example.com",
			wantErr: false,
		},
		{
			name:    "URL with trailing slash",
			baseURL: "https://api.example.com/",
			wantErr: false,
		},
		{
			name:    "empty URL",
			baseURL: "",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			baseURL: "://invalid",
			wantErr: true,
		},
		{
			name:    "with options",
			baseURL: "https://api.example.com",
			opts: []ClientOption{
				WithToken("test-token"),
				WithAPIKey("test-key"),
				WithUserAgent("test-agent"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.baseURL, tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("NewClient() returned nil client")
			}
		})
	}
}

func TestClientAuthentication(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authentication headers
		authHeader := r.Header.Get("Authorization")
		apiKeyHeader := r.Header.Get("X-API-Key")

		response := map[string]string{
			"auth":    authHeader,
			"api_key": apiKeyHeader,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	tests := []struct {
		name           string
		token          string
		apiKey         string
		expectedAuth   string
		expectedAPIKey string
	}{
		{
			name:         "JWT token",
			token:        "test-jwt-token",
			expectedAuth: "Bearer test-jwt-token",
		},
		{
			name:           "API key",
			apiKey:         "test-api-key",
			expectedAPIKey: "test-api-key",
		},
		{
			name:         "JWT token takes precedence",
			token:        "test-jwt-token",
			apiKey:       "test-api-key",
			expectedAuth: "Bearer test-jwt-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := NewClient(server.URL,
				WithToken(tt.token),
				WithAPIKey(tt.apiKey),
			)

			var result map[string]string
			err := client.get(context.Background(), "/test", &result)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			if tt.expectedAuth != "" && result["auth"] != tt.expectedAuth {
				t.Errorf("Expected auth header %q, got %q", tt.expectedAuth, result["auth"])
			}
			if tt.expectedAPIKey != "" && result["api_key"] != tt.expectedAPIKey {
				t.Errorf("Expected API key header %q, got %q", tt.expectedAPIKey, result["api_key"])
			}
		})
	}
}

func TestHealthCheck(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			t.Errorf("Expected path /api/v1/health, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		response := HealthResponse{
			Status:    "healthy",
			Timestamp: time.Now(),
			Uptime:    "1h30m",
			Version:   "1.0.0",
			Checks: map[string]string{
				"database": "ok",
				"memory":   "ok",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(server.URL)
	health, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}

	if health.Status != "healthy" {
		t.Errorf("Expected status healthy, got %s", health.Status)
	}
	if health.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", health.Version)
	}
}

func TestErrorResponse(t *testing.T) {
	// Create a mock server that returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		response := ErrorResponse{
			Error: "Repository not found",
			Code:  "REPO_NOT_FOUND",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(server.URL)
	
	// Try to get a non-existent repo
	repo, err := client.GetRepo(context.Background(), "non-existent")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if repo != nil {
		t.Error("Expected nil repo on error")
	}

	// Check error message
	errResp, ok := err.(*ErrorResponse)
	if !ok {
		t.Fatalf("Expected ErrorResponse, got %T", err)
	}

	if errResp.Code != "REPO_NOT_FOUND" {
		t.Errorf("Expected error code REPO_NOT_FOUND, got %s", errResp.Code)
	}
}

func TestContextCancellation(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client, _ := NewClient(server.URL)

	// Create a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// This should timeout
	var result map[string]string
	err := client.get(ctx, "/slow", &result)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}