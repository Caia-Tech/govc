package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRegisterWebhook(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "webhook-test")
	createInitialCommit(t, router, "webhook-test")

	t.Run("Register webhook successfully", func(t *testing.T) {
		hookReq := HookRequest{
			URL:         "https://example.com/webhook",
			Events:      []WebhookEvent{EventPush, EventCommit},
			Secret:      "secret123",
			ContentType: "application/json",
			Active:      true,
			InsecureSSL: false,
		}
		reqBody, _ := json.Marshal(hookReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/webhook-test/hooks", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusCreated, w.Code, w.Body.String())
			return
		}

		var resp HookResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.URL != hookReq.URL {
			t.Errorf("Expected URL %s, got %s", hookReq.URL, resp.URL)
		}

		if len(resp.Events) != 2 {
			t.Errorf("Expected 2 events, got %d", len(resp.Events))
		}

		if resp.Active != hookReq.Active {
			t.Errorf("Expected Active %t, got %t", hookReq.Active, resp.Active)
		}

		if resp.ID == "" {
			t.Error("Expected non-empty webhook ID")
		}
	})

	t.Run("Register webhook with missing URL", func(t *testing.T) {
		hookReq := HookRequest{
			Events: []WebhookEvent{EventPush},
			Active: true,
		}
		reqBody, _ := json.Marshal(hookReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/webhook-test/hooks", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("Register webhook with no events", func(t *testing.T) {
		hookReq := HookRequest{
			URL:    "https://example.com/webhook",
			Events: []WebhookEvent{},
			Active: true,
		}
		reqBody, _ := json.Marshal(hookReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/webhook-test/hooks", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}

		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse error response: %v", err)
		}

		if resp.Code != "MISSING_EVENTS" {
			t.Errorf("Expected error code 'MISSING_EVENTS', got '%s'", resp.Code)
		}
	})

	t.Run("Register webhook for non-existent repository", func(t *testing.T) {
		hookReq := HookRequest{
			URL:    "https://example.com/webhook",
			Events: []WebhookEvent{EventPush},
			Active: true,
		}
		reqBody, _ := json.Marshal(hookReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/nonexistent/hooks", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestListWebhooks(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "webhook-list-test")
	createInitialCommit(t, router, "webhook-list-test")

	// Register a couple of webhooks first
	hookReq1 := HookRequest{
		URL:    "https://example.com/webhook1",
		Events: []WebhookEvent{EventPush},
		Active: true,
	}
	reqBody, _ := json.Marshal(hookReq1)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/repos/webhook-list-test/hooks", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create first webhook: %d", w.Code)
	}

	hookReq2 := HookRequest{
		URL:    "https://example.com/webhook2",
		Events: []WebhookEvent{EventCommit, EventBranch},
		Active: false,
	}
	reqBody, _ = json.Marshal(hookReq2)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/repos/webhook-list-test/hooks", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create second webhook: %d", w.Code)
	}

	t.Run("List all webhooks", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/webhook-list-test/hooks", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		var resp HookListResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Count != 2 {
			t.Errorf("Expected 2 webhooks, got %d", resp.Count)
		}

		if len(resp.Hooks) != 2 {
			t.Errorf("Expected 2 hooks in array, got %d", len(resp.Hooks))
		}

		// Check that we got the right webhooks
		urls := make(map[string]bool)
		for _, hook := range resp.Hooks {
			urls[hook.URL] = true
		}

		if !urls["https://example.com/webhook1"] || !urls["https://example.com/webhook2"] {
			t.Error("Expected to find both webhook URLs in response")
		}
	})

	t.Run("List webhooks for non-existent repository", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/nonexistent/hooks", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestGetWebhook(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "webhook-get-test")
	createInitialCommit(t, router, "webhook-get-test")

	// Register a webhook first
	hookReq := HookRequest{
		URL:    "https://example.com/webhook",
		Events: []WebhookEvent{EventPush, EventCommit},
		Active: true,
		Secret: "test-secret",
	}
	reqBody, _ := json.Marshal(hookReq)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/repos/webhook-get-test/hooks", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create webhook: %d", w.Code)
	}

	var createResp HookResponse
	if err := json.Unmarshal(w.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}

	t.Run("Get webhook by ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/webhook-get-test/hooks/"+createResp.ID, nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		var resp HookResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.ID != createResp.ID {
			t.Errorf("Expected ID %s, got %s", createResp.ID, resp.ID)
		}

		if resp.URL != hookReq.URL {
			t.Errorf("Expected URL %s, got %s", hookReq.URL, resp.URL)
		}

		if len(resp.Events) != 2 {
			t.Errorf("Expected 2 events, got %d", len(resp.Events))
		}
	})

	t.Run("Get non-existent webhook", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/webhook-get-test/hooks/nonexistent", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}

		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse error response: %v", err)
		}

		if resp.Code != "WEBHOOK_NOT_FOUND" {
			t.Errorf("Expected error code 'WEBHOOK_NOT_FOUND', got '%s'", resp.Code)
		}
	})
}

func TestDeleteWebhook(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "webhook-delete-test")
	createInitialCommit(t, router, "webhook-delete-test")

	// Register a webhook first
	hookReq := HookRequest{
		URL:    "https://example.com/webhook",
		Events: []WebhookEvent{EventPush},
		Active: true,
	}
	reqBody, _ := json.Marshal(hookReq)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/repos/webhook-delete-test/hooks", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create webhook: %d", w.Code)
	}

	var createResp HookResponse
	if err := json.Unmarshal(w.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}

	t.Run("Delete webhook successfully", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/v1/repos/webhook-delete-test/hooks/"+createResp.ID, nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		var resp SuccessResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Status != "success" {
			t.Errorf("Expected status 'success', got '%s'", resp.Status)
		}

		// Verify webhook is actually deleted by trying to get it
		w2 := httptest.NewRecorder()
		req2, _ := http.NewRequest("GET", "/api/v1/repos/webhook-delete-test/hooks/"+createResp.ID, nil)
		router.ServeHTTP(w2, req2)

		if w2.Code != http.StatusNotFound {
			t.Errorf("Expected webhook to be deleted, but it still exists")
		}
	})

	t.Run("Delete non-existent webhook", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/v1/repos/webhook-delete-test/hooks/nonexistent", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}

		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse error response: %v", err)
		}

		if resp.Code != "WEBHOOK_NOT_FOUND" {
			t.Errorf("Expected error code 'WEBHOOK_NOT_FOUND', got '%s'", resp.Code)
		}
	})
}

func TestEventStream(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "event-stream-test")
	createInitialCommit(t, router, "event-stream-test")

	t.Run("Event stream connection", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/event-stream-test/events", nil)
		
		// Start the event stream in a goroutine
		done := make(chan bool, 1)
		go func() {
			router.ServeHTTP(w, req)
			done <- true
		}()

		// Give it a moment to establish connection
		time.Sleep(50 * time.Millisecond)

		// Check headers (allow charset parameter)
		contentType := w.Header().Get("Content-Type")
		if !strings.HasPrefix(contentType, "text/event-stream") {
			t.Errorf("Expected Content-Type to start with 'text/event-stream', got '%s'", contentType)
		}

		if w.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("Expected Cache-Control 'no-cache', got '%s'", w.Header().Get("Cache-Control"))
		}

		// Check that we get the initial connection event
		body := w.Body.String()
		if !strings.Contains(body, "event:connected") && !strings.Contains(body, "event: connected") {
			t.Errorf("Expected to see 'event: connected' in response. Got: %s", body)
		}

		if !strings.Contains(body, "Event stream connected") {
			t.Error("Expected to see connection message in response")
		}

		// Stop the event stream by closing the request context
		select {
		case <-done:
			// Stream ended
		case <-time.After(1 * time.Second):
			// Timeout - that's okay for this test
		}
	})

	t.Run("Event stream for non-existent repository", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/repos/nonexistent/events", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestExecuteHook(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "hook-execution-test")
	createInitialCommit(t, router, "hook-execution-test")

	t.Run("Execute pre-commit hook successfully", func(t *testing.T) {
		hookReq := HookExecutionRequest{
			Script:      "echo 'Running pre-commit checks'",
			Environment: map[string]string{"TEST_VAR": "test_value"},
			Timeout:     30,
		}
		reqBody, _ := json.Marshal(hookReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/hook-execution-test/hooks/execute/pre-commit", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		var resp HookExecutionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if !resp.Success {
			t.Errorf("Expected hook execution to succeed, got success=%t", resp.Success)
		}

		if resp.ExitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", resp.ExitCode)
		}

		if !strings.Contains(resp.Output, "pre-commit") {
			t.Errorf("Expected output to contain 'pre-commit', got: %s", resp.Output)
		}

		if resp.Environment["GOVC_HOOK_TYPE"] != "pre-commit" {
			t.Errorf("Expected GOVC_HOOK_TYPE to be 'pre-commit', got %s", resp.Environment["GOVC_HOOK_TYPE"])
		}

		if resp.Environment["TEST_VAR"] != "test_value" {
			t.Errorf("Expected TEST_VAR to be 'test_value', got %s", resp.Environment["TEST_VAR"])
		}
	})

	t.Run("Execute hook with failure", func(t *testing.T) {
		hookReq := HookExecutionRequest{
			Script:  "exit 1",
			Timeout: 30,
		}
		reqBody, _ := json.Marshal(hookReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/hook-execution-test/hooks/execute/pre-commit", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		var resp HookExecutionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Success {
			t.Errorf("Expected hook execution to fail, got success=%t", resp.Success)
		}

		if resp.ExitCode != 1 {
			t.Errorf("Expected exit code 1, got %d", resp.ExitCode)
		}

		if resp.Error == "" {
			t.Error("Expected error message, got empty string")
		}
	})

	t.Run("Execute hook with invalid request", func(t *testing.T) {
		hookReq := HookExecutionRequest{
			// Missing required Script field
			Timeout: 30,
		}
		reqBody, _ := json.Marshal(hookReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/hook-execution-test/hooks/execute/pre-commit", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}

		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse error response: %v", err)
		}

		if resp.Code != "INVALID_REQUEST" {
			t.Errorf("Expected error code 'INVALID_REQUEST', got '%s'", resp.Code)
		}
	})

	t.Run("Execute hook for non-existent repository", func(t *testing.T) {
		hookReq := HookExecutionRequest{
			Script:  "echo 'test'",
			Timeout: 30,
		}
		reqBody, _ := json.Marshal(hookReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/nonexistent/hooks/execute/pre-commit", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestHookIntegration(t *testing.T) {
	_, router := setupTestServer()

	// Create a test repository
	createRepo(t, router, "hook-integration-test")
	createInitialCommit(t, router, "hook-integration-test")

	t.Run("Pre-commit and post-commit hooks during commit", func(t *testing.T) {
		// Add a file
		addReq := AddFileRequest{Path: "test-hook.txt", Content: "Testing hooks"}
		reqBody, _ := json.Marshal(addReq)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/repos/hook-integration-test/add", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("Failed to add file: %d", w.Code)
		}

		// Commit (this should trigger both pre-commit and post-commit hooks)
		commitReq := CommitRequest{Message: "Test commit with hooks"}
		reqBody, _ = json.Marshal(commitReq)
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("POST", "/api/v1/repos/hook-integration-test/commit", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d. Response: %s", http.StatusCreated, w.Code, w.Body.String())
			return
		}

		var resp CommitResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Hash == "" {
			t.Error("Expected commit hash to be non-empty")
		}

		if resp.Message != "Test commit with hooks" {
			t.Errorf("Expected commit message 'Test commit with hooks', got '%s'", resp.Message)
		}
	})
}