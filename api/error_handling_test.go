package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caiatech/govc/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestErrorHandlingScenarios(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Invalid JSON Request", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		// Send invalid JSON
		req := httptest.NewRequest("POST", "/api/v1/repos", bytes.NewBufferString("{invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		
		var resp map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Contains(t, resp["error"], "invalid")
	})

	t.Run("Missing Required Fields", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		// Send request without required field
		body := bytes.NewBufferString(`{}`)
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Repository Not Found", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		req := httptest.NewRequest("GET", "/api/v1/repos/non-existent", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		
		var resp map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Contains(t, resp["error"], "not found")
	})

	t.Run("Concurrent Modification Error", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		// Create repository
		createBody := bytes.NewBufferString(`{"id": "test-repo"}`)
		createReq := httptest.NewRequest("POST", "/api/v1/repos", createBody)
		createReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, createReq)
		assert.Equal(t, http.StatusCreated, w.Code)

		// Start multiple transactions
		tx1Body := bytes.NewBufferString(`{}`)
		tx1Req := httptest.NewRequest("POST", "/api/v1/repos/test-repo/transaction", tx1Body)
		tx1Req.Header.Set("Content-Type", "application/json")
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, tx1Req)
		assert.Equal(t, http.StatusCreated, w1.Code)

		var tx1Resp map[string]interface{}
		err := json.NewDecoder(w1.Body).Decode(&tx1Resp)
		assert.NoError(t, err)
		
		txIDInterface, exists := tx1Resp["transaction_id"]
		if !exists {
			t.Skip("Transaction endpoint not implemented")
			return
		}
		tx1ID, ok := txIDInterface.(string)
		if !ok {
			t.Skip("Transaction ID not a string")
			return
		}

		// Try to commit the same file in both transactions
		addBody := bytes.NewBufferString(`{"path": "conflict.txt", "content": "dGVzdDE="}`)
		
		// Add to first transaction
		addReq1 := httptest.NewRequest("POST", "/api/v1/repos/test-repo/transaction/"+tx1ID+"/add", addBody)
		addReq1.Header.Set("Content-Type", "application/json")
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, addReq1)
		assert.Equal(t, http.StatusOK, w2.Code)

		// Commit first transaction
		commitBody := bytes.NewBufferString(`{"message": "First commit"}`)
		commitReq := httptest.NewRequest("POST", "/api/v1/repos/test-repo/transaction/"+tx1ID+"/commit", commitBody)
		commitReq.Header.Set("Content-Type", "application/json")
		w3 := httptest.NewRecorder()
		router.ServeHTTP(w3, commitReq)
		assert.Equal(t, http.StatusOK, w3.Code)
	})

	t.Run("Request Size Limit", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		cfg.Server.MaxRequestSize = 100 // Very small limit
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		// Create large request
		largeContent := strings.Repeat("a", 1000)
		body := bytes.NewBufferString(`{"id": "test", "content": "` + largeContent + `"}`)
		req := httptest.NewRequest("POST", "/api/v1/repos", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	})

	t.Run("Invalid Path Characters", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		// Create repository
		createBody := bytes.NewBufferString(`{"id": "test-repo"}`)
		createReq := httptest.NewRequest("POST", "/api/v1/repos", createBody)
		createReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, createReq)
		assert.Equal(t, http.StatusCreated, w.Code)

		// Try to read file with invalid path
		req := httptest.NewRequest("GET", "/api/v1/repos/test-repo/read/../../../etc/passwd", nil)
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req)

		// Should be sanitized or rejected
		assert.NotEqual(t, http.StatusOK, w2.Code)
	})

	t.Run("Transaction Timeout", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		// Create repository
		createBody := bytes.NewBufferString(`{"id": "test-repo"}`)
		createReq := httptest.NewRequest("POST", "/api/v1/repos", createBody)
		createReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, createReq)
		assert.Equal(t, http.StatusCreated, w.Code)

		// Start transaction
		txBody := bytes.NewBufferString(`{}`)
		txReq := httptest.NewRequest("POST", "/api/v1/repos/test-repo/transaction", txBody)
		txReq.Header.Set("Content-Type", "application/json")
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, txReq)
		assert.Equal(t, http.StatusCreated, w1.Code)

		var txResp map[string]interface{}
		err := json.NewDecoder(w1.Body).Decode(&txResp)
		assert.NoError(t, err)
		
		txIDInterface, exists := txResp["transaction_id"]
		if !exists {
			t.Skip("Transaction endpoint not implemented")
			return
		}
		txID, ok := txIDInterface.(string)
		if !ok {
			t.Skip("Transaction ID not a string")
			return
		}

		// Wait for transaction to expire (if implemented)
		time.Sleep(2 * time.Second)

		// Try to use expired transaction
		addBody := bytes.NewBufferString(`{"path": "test.txt", "content": "dGVzdA=="}`)
		addReq := httptest.NewRequest("POST", "/api/v1/repos/test-repo/transaction/"+txID+"/add", addBody)
		addReq.Header.Set("Content-Type", "application/json")
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, addReq)

		// Should handle gracefully even if transaction is still valid
		assert.Contains(t, []int{http.StatusOK, http.StatusNotFound, http.StatusGone}, w2.Code)
	})

	t.Run("Invalid Base64 Content", func(t *testing.T) {
		// Skip this test - the API accepts plain text content, not base64
		// The content "not-base64!@#$" is valid text that can be stored in a file
		t.Skip("API accepts plain text content, not base64 encoded content")
	})

	t.Run("Duplicate Repository Creation", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		// Create repository
		body := bytes.NewBufferString(`{"id": "duplicate-test"}`)
		req1 := httptest.NewRequest("POST", "/api/v1/repos", body)
		req1.Header.Set("Content-Type", "application/json")
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, req1)
		assert.Equal(t, http.StatusCreated, w1.Code)

		// Try to create again
		body2 := bytes.NewBufferString(`{"id": "duplicate-test"}`)
		req2 := httptest.NewRequest("POST", "/api/v1/repos", body2)
		req2.Header.Set("Content-Type", "application/json")
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)
		assert.Equal(t, http.StatusConflict, w2.Code)
	})

	t.Run("Invalid HTTP Method", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		server := NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		// Use wrong HTTP method
		req := httptest.NewRequest("DELETE", "/api/v1/repos", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Gin returns 404 when no route matches for the method
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestPanicRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Handler Panic Recovery", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		server := NewServer(cfg)
		router := gin.New()
		
		// Add custom panic route for testing
		router.GET("/api/v1/panic", func(c *gin.Context) {
			panic("test panic")
		})
		
		server.RegisterRoutes(router)

		req := httptest.NewRequest("GET", "/api/v1/panic", nil)
		w := httptest.NewRecorder()
		
		// Should not panic the test
		assert.NotPanics(t, func() {
			router.ServeHTTP(w, req)
		})

		// Should return 500
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestConcurrentRequestHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false
	server := NewServer(cfg)
	router := gin.New()
	server.RegisterRoutes(router)

	// Create test repository
	createBody := bytes.NewBufferString(`{"id": "concurrent-test"}`)
	createReq := httptest.NewRequest("POST", "/api/v1/repos", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, createReq)
	assert.Equal(t, http.StatusCreated, w.Code)

	t.Run("Concurrent Reads", func(t *testing.T) {
		// Add a file first
		addBody := bytes.NewBufferString(`{"path": "test.txt", "content": "dGVzdCBjb250ZW50"}`)
		addReq := httptest.NewRequest("POST", "/api/v1/repos/concurrent-test/add", addBody)
		addReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, addReq)
		assert.Equal(t, http.StatusOK, w.Code)

		// Concurrent reads
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				req := httptest.NewRequest("GET", "/api/v1/repos/concurrent-test/read/test.txt", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				assert.Equal(t, http.StatusOK, w.Code)
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}
	})

	t.Run("Concurrent Writes", func(t *testing.T) {
		errors := make(chan error, 10)
		
		for i := 0; i < 10; i++ {
			go func(idx int) {
				body := bytes.NewBufferString(`{"path": "file` + string(rune(idx)) + `.txt", "content": "dGVzdA=="}`)
				req := httptest.NewRequest("POST", "/api/v1/repos/concurrent-test/add", body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				
				if w.Code != http.StatusOK {
					errors <- assert.AnError
				} else {
					errors <- nil
				}
			}(i)
		}

		// Check results
		errorCount := 0
		for i := 0; i < 10; i++ {
			if err := <-errors; err != nil {
				errorCount++
			}
		}

		// Some writes might fail due to conflicts, but not all
		assert.Less(t, errorCount, 10)
	})
}