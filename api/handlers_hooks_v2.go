package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// registerHookV2 registers a webhook using new architecture
func (s *Server) registerHookV2(c *gin.Context) {
	repoID := c.Param("repo_id")

	var req HookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Convert events to strings
	eventStrings := make([]string, len(req.Events))
	for i, event := range req.Events {
		eventStrings[i] = string(event)
	}

	// Register webhook
	webhook, err := components.Webhooks.Register(req.URL, eventStrings, req.Secret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("failed to register webhook: %v", err),
			Code:  "WEBHOOK_REGISTER_FAILED",
		})
		return
	}

	c.JSON(http.StatusCreated, HookResponse{
		ID:          webhook.ID,
		URL:         webhook.URL,
		Events:      convertToWebhookEvents(webhook.Events),
		ContentType: webhook.ContentType,
		Active:      true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})
}

// listHooksV2 lists all webhooks using new architecture
func (s *Server) listHooksV2(c *gin.Context) {
	repoID := c.Param("repo_id")

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// List webhooks
	webhooks := components.Webhooks.List()

	// Convert to response format
	hookList := make([]HookResponse, len(webhooks))
	for i, webhook := range webhooks {
		hookList[i] = HookResponse{
			ID:          webhook.ID,
			URL:         webhook.URL,
			Events:      convertToWebhookEvents(webhook.Events),
			ContentType: webhook.ContentType,
			Active:      true,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
	}

	c.JSON(http.StatusOK, HookListResponse{
		Hooks: hookList,
		Count: len(hookList),
	})
}

// getHookV2 returns details of a specific webhook
func (s *Server) getHookV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	hookID := c.Param("hook_id")

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Get webhook
	webhook, err := components.Webhooks.Get(hookID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "WEBHOOK_NOT_FOUND",
		})
		return
	}

	c.JSON(http.StatusOK, HookResponse{
		ID:          webhook.ID,
		URL:         webhook.URL,
		Events:      convertToWebhookEvents(webhook.Events),
		ContentType: webhook.ContentType,
		Active:      true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})
}

// deleteHookV2 removes a webhook using new architecture
func (s *Server) deleteHookV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	hookID := c.Param("hook_id")

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Unregister webhook
	if err := components.Webhooks.Unregister(hookID); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "WEBHOOK_NOT_FOUND",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: fmt.Sprintf("webhook '%s' deleted", hookID[:7]),
	})
}

// executeHookV2 manually triggers webhook events
func (s *Server) executeHookV2(c *gin.Context) {
	repoID := c.Param("repo_id")
	hookType := c.Param("hook_type")

	var req struct {
		Data interface{} `json:"data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_REQUEST",
		})
		return
	}

	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "REPO_NOT_FOUND",
		})
		return
	}

	// Prepare event data based on hook type
	var eventData interface{}
	switch hookType {
	case "push":
		// Get recent commits
		commits, _ := components.Operations.Log(5)
		commitData := make([]map[string]interface{}, len(commits))
		for i, commit := range commits {
			commitData[i] = map[string]interface{}{
				"id":      commit.Hash(),
				"message": commit.Message,
				"author": map[string]interface{}{
					"name":  commit.Author.Name,
					"email": commit.Author.Email,
				},
				"timestamp": commit.Author.Time,
			}
		}

		status, _ := components.Operations.Status()
		eventData = map[string]interface{}{
			"ref":        fmt.Sprintf("refs/heads/%s", status.Branch),
			"repository": repoID,
			"commits":    commitData,
		}

	case "commit":
		// Get last commit
		commits, _ := components.Operations.Log(1)
		if len(commits) > 0 {
			commit := commits[0]
			eventData = map[string]interface{}{
				"commit": map[string]interface{}{
					"id":      commit.Hash(),
					"message": commit.Message,
					"author": map[string]interface{}{
						"name":  commit.Author.Name,
						"email": commit.Author.Email,
					},
					"timestamp": commit.Author.Time,
				},
				"repository": repoID,
			}
		}

	default:
		eventData = req.Data
	}

	// Trigger webhooks
	components.Webhooks.Trigger(hookType, repoID, eventData)

	c.JSON(http.StatusOK, SuccessResponse{
		Message: fmt.Sprintf("triggered '%s' webhooks", hookType),
	})
}

// triggerWebhooksForEvent triggers webhooks for a specific event
func (s *Server) triggerWebhooksForEvent(repoID string, event string, data interface{}) {
	// Get components
	components, err := s.getRepositoryComponents(repoID)
	if err != nil {
		s.logger.Errorf("Failed to get repository components for webhook: %v", err)
		return
	}

	// Trigger webhooks
	components.Webhooks.Trigger(event, repoID, data)
}

// convertToWebhookEvents converts string events to WebhookEvent type
func convertToWebhookEvents(events []string) []WebhookEvent {
	result := make([]WebhookEvent, len(events))
	for i, event := range events {
		result[i] = WebhookEvent(event)
	}
	return result
}
