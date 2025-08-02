package core

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// WebhookManager manages repository webhooks
type WebhookManager struct {
	webhooks map[string]*Webhook
}

// NewWebhookManager creates a new webhook manager
func NewWebhookManager() *WebhookManager {
	return &WebhookManager{
		webhooks: make(map[string]*Webhook),
	}
}

// Webhook represents a repository webhook
type Webhook struct {
	ID          string   `json:"id"`
	URL         string   `json:"url"`
	Events      []string `json:"events"`
	Secret      string   `json:"secret"`
	ContentType string   `json:"content_type"`
}

// WebhookPayload represents the payload sent to webhooks
type WebhookPayload struct {
	Event      string      `json:"event"`
	Repository string      `json:"repository"`
	Timestamp  time.Time   `json:"timestamp"`
	Data       interface{} `json:"data"`
}

// Register adds a new webhook
func (wm *WebhookManager) Register(url string, events []string, secret string) (*Webhook, error) {
	webhook := &Webhook{
		ID:          uuid.New().String(),
		URL:         url,
		Events:      events,
		Secret:      secret,
		ContentType: "application/json",
	}
	
	wm.webhooks[webhook.ID] = webhook
	return webhook, nil
}

// Unregister removes a webhook
func (wm *WebhookManager) Unregister(id string) error {
	delete(wm.webhooks, id)
	return nil
}

// List returns all webhooks
func (wm *WebhookManager) List() []*Webhook {
	hooks := make([]*Webhook, 0, len(wm.webhooks))
	for _, hook := range wm.webhooks {
		hooks = append(hooks, hook)
	}
	return hooks
}

// Get returns a specific webhook
func (wm *WebhookManager) Get(id string) (*Webhook, error) {
	hook, ok := wm.webhooks[id]
	if !ok {
		return nil, fmt.Errorf("webhook not found: %s", id)
	}
	return hook, nil
}

// Trigger sends an event to all registered webhooks
func (wm *WebhookManager) Trigger(event string, repoName string, data interface{}) {
	payload := WebhookPayload{
		Event:      event,
		Repository: repoName,
		Timestamp:  time.Now(),
		Data:       data,
	}
	
	for _, webhook := range wm.webhooks {
		// Check if webhook is subscribed to this event
		subscribed := false
		for _, e := range webhook.Events {
			if e == event || e == "*" {
				subscribed = true
				break
			}
		}
		
		if subscribed {
			// Send webhook asynchronously
			go wm.sendWebhook(webhook, payload)
		}
	}
}

// sendWebhook sends a webhook request
func (wm *WebhookManager) sendWebhook(webhook *Webhook, payload WebhookPayload) {
	body, err := json.Marshal(payload)
	if err != nil {
		// Log error
		return
	}
	
	req, err := http.NewRequest("POST", webhook.URL, bytes.NewBuffer(body))
	if err != nil {
		// Log error
		return
	}
	
	req.Header.Set("Content-Type", webhook.ContentType)
	req.Header.Set("X-Govc-Event", payload.Event)
	req.Header.Set("X-Govc-Delivery", uuid.New().String())
	
	// Add signature if secret is set
	if webhook.Secret != "" {
		signature := wm.calculateSignature(webhook.Secret, body)
		req.Header.Set("X-Govc-Signature", signature)
	}
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Log error
		return
	}
	defer resp.Body.Close()
	
	// Log response status
}

// calculateSignature calculates HMAC-SHA256 signature
func (wm *WebhookManager) calculateSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}