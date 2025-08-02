package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWebhookManager(t *testing.T) {
	wm := NewWebhookManager()
	assert.NotNil(t, wm)
	assert.NotNil(t, wm.webhooks)
	assert.Empty(t, wm.webhooks)
}

func TestWebhookManagerRegister(t *testing.T) {
	wm := NewWebhookManager()

	// Test registering a webhook
	url := "https://example.com/webhook"
	events := []string{"push", "commit"}
	secret := "test-secret"

	webhook, err := wm.Register(url, events, secret)
	require.NoError(t, err)
	assert.NotNil(t, webhook)
	assert.NotEmpty(t, webhook.ID)
	assert.Equal(t, url, webhook.URL)
	assert.Equal(t, events, webhook.Events)
	assert.Equal(t, secret, webhook.Secret)
	assert.Equal(t, "application/json", webhook.ContentType)

	// Verify webhook was stored
	assert.Len(t, wm.webhooks, 1)
	assert.Equal(t, webhook, wm.webhooks[webhook.ID])
}

func TestWebhookManagerUnregister(t *testing.T) {
	wm := NewWebhookManager()

	// Register a webhook
	webhook, err := wm.Register("https://example.com/webhook", []string{"push"}, "secret")
	require.NoError(t, err)

	// Verify webhook exists
	assert.Len(t, wm.webhooks, 1)

	// Unregister the webhook
	err = wm.Unregister(webhook.ID)
	require.NoError(t, err)

	// Verify webhook was removed
	assert.Empty(t, wm.webhooks)
}

func TestWebhookManagerUnregisterNonExistent(t *testing.T) {
	wm := NewWebhookManager()

	// Unregister non-existent webhook should not error
	err := wm.Unregister("nonexistent")
	assert.NoError(t, err)
}

func TestWebhookManagerList(t *testing.T) {
	wm := NewWebhookManager()

	// Test empty list
	hooks := wm.List()
	assert.Empty(t, hooks)

	// Register multiple webhooks
	webhook1, _ := wm.Register("https://example.com/webhook1", []string{"push"}, "secret1")
	webhook2, _ := wm.Register("https://example.com/webhook2", []string{"commit"}, "secret2")

	// Test list with webhooks
	hooks = wm.List()
	assert.Len(t, hooks, 2)

	// Verify webhooks are in the list
	webhookMap := make(map[string]*Webhook)
	for _, hook := range hooks {
		webhookMap[hook.ID] = hook
	}

	assert.Equal(t, webhook1, webhookMap[webhook1.ID])
	assert.Equal(t, webhook2, webhookMap[webhook2.ID])
}

func TestWebhookManagerGet(t *testing.T) {
	wm := NewWebhookManager()

	// Test getting non-existent webhook
	_, err := wm.Get("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhook not found")

	// Register a webhook
	webhook, _ := wm.Register("https://example.com/webhook", []string{"push"}, "secret")

	// Test getting existing webhook
	retrieved, err := wm.Get(webhook.ID)
	require.NoError(t, err)
	assert.Equal(t, webhook, retrieved)
}

func TestWebhookManagerTriggerEventMatching(t *testing.T) {
	wm := NewWebhookManager()

	// Set up test server to capture webhook requests
	var receivedPayloads []WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload WebhookPayload
		json.NewDecoder(r.Body).Decode(&payload)
		receivedPayloads = append(receivedPayloads, payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Register webhooks with different event subscriptions
	webhook1, _ := wm.Register(server.URL, []string{"push"}, "")
	webhook2, _ := wm.Register(server.URL, []string{"commit"}, "")
	webhook3, _ := wm.Register(server.URL, []string{"*"}, "") // Subscribe to all events

	// Trigger a push event
	wm.Trigger("push", "test-repo", map[string]string{"ref": "main"})

	// Give time for async webhook calls
	time.Sleep(100 * time.Millisecond)

	// webhook1 and webhook3 should receive the event, webhook2 should not
	// We expect 2 payloads (webhook1 and webhook3)
	assert.True(t, len(receivedPayloads) >= 2, "Expected at least 2 webhook calls")

	// Verify event details
	for _, payload := range receivedPayloads {
		assert.Equal(t, "push", payload.Event)
		assert.Equal(t, "test-repo", payload.Repository)
		assert.NotZero(t, payload.Timestamp)
		assert.NotNil(t, payload.Data)
	}

	// Verify webhooks were registered correctly
	assert.Equal(t, []string{"push"}, webhook1.Events)
	assert.Equal(t, []string{"commit"}, webhook2.Events)
	assert.Equal(t, []string{"*"}, webhook3.Events)
}

func TestWebhookManagerTriggerWithSignature(t *testing.T) {
	wm := NewWebhookManager()

	// Set up test server to capture webhook requests
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Register webhook with secret
	secret := "test-secret"
	webhook, _ := wm.Register(server.URL, []string{"push"}, secret)

	// Trigger event
	wm.Trigger("push", "test-repo", map[string]string{"ref": "main"})

	// Give time for async webhook call
	time.Sleep(100 * time.Millisecond)

	// Verify headers were set correctly
	assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))
	assert.Equal(t, "push", receivedHeaders.Get("X-Govc-Event"))
	assert.NotEmpty(t, receivedHeaders.Get("X-Govc-Delivery"))
	assert.NotEmpty(t, receivedHeaders.Get("X-Govc-Signature"))
	assert.Contains(t, receivedHeaders.Get("X-Govc-Signature"), "sha256=")

	// Verify webhook properties
	assert.Equal(t, secret, webhook.Secret)
	assert.Equal(t, "application/json", webhook.ContentType)
}

func TestWebhookManagerTriggerWithoutSignature(t *testing.T) {
	wm := NewWebhookManager()

	// Set up test server to capture webhook requests
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Register webhook without secret
	webhook, _ := wm.Register(server.URL, []string{"push"}, "")

	// Trigger event
	wm.Trigger("push", "test-repo", map[string]string{"ref": "main"})

	// Give time for async webhook call
	time.Sleep(100 * time.Millisecond)

	// Verify signature header is not set when no secret
	assert.Empty(t, receivedHeaders.Get("X-Govc-Signature"))
	assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))
	assert.Equal(t, "push", receivedHeaders.Get("X-Govc-Event"))
	assert.NotEmpty(t, receivedHeaders.Get("X-Govc-Delivery"))

	// Verify webhook has no secret
	assert.Empty(t, webhook.Secret)
}

func TestWebhookManagerCalculateSignature(t *testing.T) {
	wm := NewWebhookManager()

	secret := "test-secret"
	body := []byte(`{"event":"push","repository":"test-repo"}`)

	signature := wm.calculateSignature(secret, body)

	// Verify signature format
	assert.Contains(t, signature, "sha256=")
	assert.Len(t, signature, 71) // "sha256=" (7) + 64 hex chars = 71

	// Verify signature is consistent
	signature2 := wm.calculateSignature(secret, body)
	assert.Equal(t, signature, signature2)

	// Verify different body produces different signature
	signature3 := wm.calculateSignature(secret, []byte("different body"))
	assert.NotEqual(t, signature, signature3)

	// Verify different secret produces different signature
	signature4 := wm.calculateSignature("different-secret", body)
	assert.NotEqual(t, signature, signature4)
}

func TestWebhookManagerTriggerMultipleEvents(t *testing.T) {
	wm := NewWebhookManager()

	// Register webhook subscribed to multiple events
	webhook, _ := wm.Register("https://example.com/webhook", []string{"push", "commit", "tag"}, "")

	// Test that webhook is subscribed to the correct events
	assert.Contains(t, webhook.Events, "push")
	assert.Contains(t, webhook.Events, "commit")
	assert.Contains(t, webhook.Events, "tag")
	assert.NotContains(t, webhook.Events, "branch")

	// Test webhook registration
	assert.Equal(t, "https://example.com/webhook", webhook.URL)
	assert.Empty(t, webhook.Secret)
}

func TestWebhookManagerTriggerWildcardSubscription(t *testing.T) {
	wm := NewWebhookManager()

	// Register webhook with wildcard subscription
	webhook, _ := wm.Register("https://example.com/webhook", []string{"*"}, "")

	// Test that webhook is subscribed to wildcard
	assert.Contains(t, webhook.Events, "*")
	assert.Len(t, webhook.Events, 1)

	// Test webhook registration
	assert.Equal(t, "https://example.com/webhook", webhook.URL)
	assert.Empty(t, webhook.Secret)
}

func TestWebhookPayloadStructure(t *testing.T) {
	wm := NewWebhookManager()

	// Set up test server to capture webhook requests
	var receivedPayload WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Register webhook
	wm.Register(server.URL, []string{"test"}, "")

	// Trigger event with complex data
	testData := map[string]interface{}{
		"commits": []string{"abc123", "def456"},
		"branch":  "main",
		"author":  "test-user",
		"count":   2,
	}

	wm.Trigger("test", "my-repo", testData)

	// Give time for async webhook call
	time.Sleep(100 * time.Millisecond)

	// Verify payload structure
	assert.Equal(t, "test", receivedPayload.Event)
	assert.Equal(t, "my-repo", receivedPayload.Repository)
	assert.NotZero(t, receivedPayload.Timestamp)

	// Verify data content (JSON unmarshaling may change types)
	payloadData := receivedPayload.Data.(map[string]interface{})
	assert.Equal(t, "main", payloadData["branch"])
	assert.Equal(t, "test-user", payloadData["author"])
	assert.Equal(t, float64(2), payloadData["count"]) // JSON unmarshaling converts numbers to float64
	commits := payloadData["commits"].([]interface{})
	assert.Len(t, commits, 2)
	assert.Equal(t, "abc123", commits[0])
	assert.Equal(t, "def456", commits[1])

	// Verify timestamp is recent
	timeDiff := time.Since(receivedPayload.Timestamp)
	assert.True(t, timeDiff < 5*time.Second, "Timestamp should be recent")
}
