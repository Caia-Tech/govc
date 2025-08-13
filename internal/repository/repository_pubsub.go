package repository

import (
	"fmt"
	"time"
)

// Subscribe creates a subscription to file changes matching the given pattern
func (r *Repository) Subscribe(pattern string, handler SimpleEventHandler) UnsubscribeFunc {
	if r.eventBus == nil {
		// Event bus not initialized, return no-op function
		return func() {}
	}
	
	return r.eventBus.Subscribe(pattern, handler)
}

// SubscribeWithTypes creates a subscription for specific event types
func (r *Repository) SubscribeWithTypes(pattern string, eventTypes []EventType, handler EventHandler) UnsubscribeFunc {
	if r.eventBus == nil {
		return func() {}
	}
	
	return r.eventBus.SubscribeWithTypes(pattern, eventTypes, handler)
}

// OnCommit subscribes to commit events
func (r *Repository) OnCommit(handler CommitEventHandler) UnsubscribeFunc {
	if r.eventBus == nil {
		return func() {}
	}
	
	return r.eventBus.OnCommit(handler)
}


// GetSubscriptions returns all active subscriptions
func (r *Repository) GetSubscriptions() []SubscriptionStats {
	if r.eventBus == nil {
		return []SubscriptionStats{}
	}
	
	return r.eventBus.GetSubscriptions()
}

// GetEventBusStats returns event bus statistics
func (r *Repository) GetEventBusStats() EventBusStats {
	if r.eventBus == nil {
		return EventBusStats{}
	}
	
	return r.eventBus.GetStats()
}

// publishEvent publishes an event to the event bus
func (r *Repository) publishEvent(eventType EventType, path string, oldValue, newValue []byte, metadata map[string]interface{}) {
	if r.eventBus == nil {
		return
	}
	
	event := RepoEvent{
		ID:        generateEventID(),
		Type:      eventType,
		Timestamp: time.Now(),
		Source:    func() string { s, _ := r.GetCurrentBranch(); return s }(),
		Path:      path,
		OldValue:  oldValue,
		NewValue:  newValue,
		Metadata:  metadata,
	}
	
	r.eventBus.Publish(event)
}

// publishCommitEvent publishes a commit event
func (r *Repository) publishCommitEvent(commitHash string, files []string) {
	if r.eventBus == nil {
		return
	}
	
	metadata := map[string]interface{}{
		"files": files,
	}
	
	event := RepoEvent{
		ID:        generateEventID(),
		Type:      CommitCreated,
		Timestamp: time.Now(),
		Source:    commitHash,
		Path:      "",
		Metadata:  metadata,
	}
	
	r.eventBus.Publish(event)
}

// publishBranchEvent publishes a branch-related event
func (r *Repository) publishBranchEvent(eventType EventType, branchName string, metadata map[string]interface{}) {
	if r.eventBus == nil {
		return
	}
	
	event := RepoEvent{
		ID:        generateEventID(),
		Type:      eventType,
		Timestamp: time.Now(),
		Source:    branchName,
		Path:      "",
		Metadata:  metadata,
	}
	
	r.eventBus.Publish(event)
}

// GetCurrentBranch returns the current branch name
func (r *Repository) GetCurrentBranch() (string, error) {
	if r.refManager != nil {
		if branch, err := r.refManager.GetCurrentBranch(); err == nil {
			return branch, nil
		}
	}
	return "main", nil
}

// generateEventID generates a unique event ID
func generateEventID() string {
	return fmt.Sprintf("evt_%d", time.Now().UnixNano())
}