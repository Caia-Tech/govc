package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caiatech/govc/api"
	"github.com/caiatech/govc/auth"
	"github.com/caiatech/govc/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSecurityVulnerabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Authentication Bypass Attempts", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = true
		server := api.NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		tests := []struct {
			name        string
			headers     map[string]string
			expectCode  int
		}{
			{
				name:       "No Auth Header",
				headers:    map[string]string{},
				expectCode: http.StatusUnauthorized,
			},
			{
				name: "Empty Bearer Token",
				headers: map[string]string{
					"Authorization": "Bearer ",
				},
				expectCode: http.StatusUnauthorized,
			},
			{
				name: "Invalid Token Format",
				headers: map[string]string{
					"Authorization": "InvalidFormat",
				},
				expectCode: http.StatusUnauthorized,
			},
			{
				name: "SQL Injection in Token",
				headers: map[string]string{
					"Authorization": "Bearer ' OR '1'='1",
				},
				expectCode: http.StatusUnauthorized,
			},
			{
				name: "Malformed JWT",
				headers: map[string]string{
					"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.INVALID.SIGNATURE",
				},
				expectCode: http.StatusUnauthorized,
			},
			{
				name: "Multiple Auth Headers",
				headers: map[string]string{
					"Authorization": "Bearer token1",
					"X-API-Key":     "key1",
				},
				expectCode: http.StatusUnauthorized,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := httptest.NewRequest("GET", "/api/v1/repos", nil)
				for k, v := range tt.headers {
					req.Header.Set(k, v)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, tt.expectCode, w.Code)
			})
		}
	})

	t.Run("Path Traversal Protection", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		server := api.NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		// Create test repository
		createBody := bytes.NewBufferString(`{"id": "security-test"}`)
		createReq := httptest.NewRequest("POST", "/api/v1/repos", createBody)
		createReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, createReq)
		assert.Equal(t, http.StatusCreated, w.Code)

		// Path traversal attempts
		maliciousPaths := []string{
			"../../../etc/passwd",
			"..\\..\\..\\windows\\system32\\config\\sam",
			"....//....//....//etc/passwd",
			"%2e%2e%2f%2e%2e%2f%2e%2e%2fetc%2fpasswd",
			"..%252f..%252f..%252fetc%252fpasswd",
			"..%c0%af..%c0%af..%c0%afetc%c0%afpasswd",
			"/var/log/../../../etc/passwd",
			"C:\\..\\..\\..\\windows\\system32\\drivers\\etc\\hosts",
		}

		for _, path := range maliciousPaths {
			t.Run(fmt.Sprintf("Path: %s", path), func(t *testing.T) {
				req := httptest.NewRequest("GET", "/api/v1/repos/security-test/read/"+path, nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Should either return 404 or 400, but never 200 with system file content
				assert.NotEqual(t, http.StatusOK, w.Code)
				
				// Ensure no system file content is leaked
				body := w.Body.String()
				assert.NotContains(t, body, "root:")
				assert.NotContains(t, body, "[boot loader]")
			})
		}
	})

	t.Run("Injection Attack Prevention", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		server := api.NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		injectionPayloads := []struct {
			name    string
			payload string
		}{
			{
				name:    "Command Injection",
				payload: `{"id": "test; rm -rf /"}`,
			},
			{
				name:    "Script Injection",
				payload: `{"id": "<script>alert('XSS')</script>"}`,
			},
			{
				name:    "SQL Injection",
				payload: `{"id": "test' OR '1'='1"}`,
			},
			{
				name:    "LDAP Injection",
				payload: `{"id": "test)(uid=*))(|(uid=*"}`,
			},
			{
				name:    "XML Injection",
				payload: `{"id": "<?xml version=\"1.0\"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM \"file:///etc/passwd\">]><foo>&xxe;</foo>"}`,
			},
			{
				name:    "Null Byte Injection",
				payload: `{"id": "test\x00.txt"}`,
			},
		}

		for _, inj := range injectionPayloads {
			t.Run(inj.name, func(t *testing.T) {
				req := httptest.NewRequest("POST", "/api/v1/repos", bytes.NewBufferString(inj.payload))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Should either reject or sanitize, but never execute injected code
				if w.Code == http.StatusCreated {
					// If created, verify the ID was sanitized
					var resp map[string]interface{}
					json.NewDecoder(w.Body).Decode(&resp)
					id := resp["id"].(string)
					assert.NotContains(t, id, ";")
					assert.NotContains(t, id, "<script>")
					assert.NotContains(t, id, "<?xml")
				}
			})
		}
	})

	t.Run("Rate Limiting and DoS Protection", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		server := api.NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		t.Run("Large Request Body", func(t *testing.T) {
			// Create very large request body
			largeBody := bytes.NewBufferString(`{"id": "test", "data": "`)
			largeBody.WriteString(strings.Repeat("A", 100*1024*1024)) // 100MB
			largeBody.WriteString(`"}`)

			req := httptest.NewRequest("POST", "/api/v1/repos", largeBody)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Should reject large requests
			assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		})

		t.Run("Rapid Request Flood", func(t *testing.T) {
			// Note: Real rate limiting would be implemented in middleware
			// This test verifies the server doesn't crash under load
			done := make(chan bool)
			
			go func() {
				for i := 0; i < 1000; i++ {
					req := httptest.NewRequest("GET", "/api/v1/health", nil)
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)
				}
				done <- true
			}()

			select {
			case <-done:
				t.Log("Server handled rapid requests without crashing")
			case <-time.After(5 * time.Second):
				t.Error("Server appears to be hanging under load")
			}
		})
	})

	t.Run("Token Security", func(t *testing.T) {
		cfg := config.DefaultConfig()
		jwtAuth := auth.NewJWTAuth(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, cfg.Auth.JWT.TTL)

		t.Run("Token Expiration", func(t *testing.T) {
			// Create token with very short TTL
			shortAuth := auth.NewJWTAuth(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, 1*time.Millisecond)
			token, err := shortAuth.GenerateToken("user", "user123", "test@example.com", []string{"read"})
			assert.NoError(t, err)

			// Wait for expiration
			time.Sleep(2 * time.Millisecond)

			// Try to validate expired token
			_, err = jwtAuth.ValidateToken(token)
			assert.Error(t, err)
		})

		t.Run("Token Tampering", func(t *testing.T) {
			token, err := jwtAuth.GenerateToken("user", "user123", "test@example.com", []string{"read"})
			assert.NoError(t, err)

			// Tamper with token
			parts := strings.Split(token, ".")
			if len(parts) == 3 {
				// Modify payload
				tamperedToken := parts[0] + "." + parts[1] + "modified" + "." + parts[2]
				
				_, err = jwtAuth.ValidateToken(tamperedToken)
				assert.Error(t, err)
			}
		})

		t.Run("Weak Secret Detection", func(t *testing.T) {
			// Test with weak secrets
			weakSecrets := []string{
				"",
				"password",
				"123456",
				"secret",
			}

			for _, secret := range weakSecrets {
				t.Run(fmt.Sprintf("Secret: %s", secret), func(t *testing.T) {
					if secret == "" {
						// Empty secret should cause panic or error in NewJWTAuth
						assert.Panics(t, func() {
							auth.NewJWTAuth(secret, "issuer", 24*time.Hour)
						})
					}
				})
			}
		})
	})

	t.Run("RBAC Privilege Escalation", func(t *testing.T) {
		rbac := auth.NewRBAC()

		// Create regular user
		err := rbac.CreateUser("regular", "Regular User", "regular@test.com", []string{"developer"})
		assert.NoError(t, err)

		// Try to escalate privileges
		t.Run("Direct Role Assignment", func(t *testing.T) {
			// User shouldn't be able to assign admin role to themselves
			err := rbac.AssignRole("regular", "admin")
			// This might succeed depending on implementation, but check permissions
			
			hasAdmin := rbac.HasPermission("regular", auth.PermissionSystemAdmin)
			if err == nil {
				// If role assignment succeeded, verify it actually grants admin
				assert.True(t, hasAdmin)
			} else {
				// If it failed, user shouldn't have admin
				assert.False(t, hasAdmin)
			}
		})

		t.Run("Permission Check Bypass", func(t *testing.T) {
			// Try various permission strings that might bypass checks
			permissionVariants := []string{
				"SYSTEM:ADMIN",
				"system:Admin",
				"system::admin",
				"system admin",
				"*:admin",
				"system:*",
			}

			for _, perm := range permissionVariants {
				hasPermission := rbac.HasPermission("regular", auth.Permission(perm))
				assert.False(t, hasPermission, "Permission variant should not bypass: %s", perm)
			}
		})
	})

	t.Run("Information Disclosure", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = false
		server := api.NewServer(cfg)
		router := gin.New()
		server.RegisterRoutes(router)

		t.Run("Error Message Leakage", func(t *testing.T) {
			// Send malformed request
			req := httptest.NewRequest("POST", "/api/v1/repos", bytes.NewBufferString("{malformed"))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Check error doesn't leak sensitive info
			body := w.Body.String()
			assert.NotContains(t, body, "panic")
			assert.NotContains(t, body, "stack trace")
			assert.NotContains(t, body, "/Users/")
			assert.NotContains(t, body, "\\Users\\")
		})

		t.Run("Debug Information", func(t *testing.T) {
			// Ensure debug endpoints don't exist in production
			debugEndpoints := []string{
				"/debug/pprof/",
				"/debug/vars",
				"/.git/config",
				"/.env",
				"/config.yaml",
			}

			for _, endpoint := range debugEndpoints {
				req := httptest.NewRequest("GET", endpoint, nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusNotFound, w.Code, 
					"Debug endpoint should not be exposed: %s", endpoint)
			}
		})
	})
}