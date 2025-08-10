package govc

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// SearchSubscriber provides real-time search subscriptions
type SearchSubscriber struct {
	subscriptions map[string]*SearchSubscription
	mu            sync.RWMutex
	nextID        int64
}

// SearchSubscription represents an active search subscription
type SearchSubscription struct {
	ID          string                    `json:"id"`
	Query       *FullTextSearchRequest   `json:"query"`
	Channel     chan SearchNotification  `json:"-"`
	LastResults []SearchResult           `json:"last_results"`
	Created     time.Time                `json:"created"`
	LastUpdate  time.Time                `json:"last_update"`
	Active      bool                     `json:"active"`
	Context     context.Context          `json:"-"`
	Cancel      context.CancelFunc       `json:"-"`
	Options     SubscriptionOptions      `json:"options"`
}

// SearchNotification represents a real-time search update
type SearchNotification struct {
	SubscriptionID string              `json:"subscription_id"`
	Type           NotificationType    `json:"type"`
	Results        []SearchResult      `json:"results,omitempty"`
	NewResult      *SearchResult       `json:"new_result,omitempty"`
	RemovedPath    string              `json:"removed_path,omitempty"`
	UpdatedResult  *SearchResult       `json:"updated_result,omitempty"`
	Timestamp      time.Time           `json:"timestamp"`
	Message        string              `json:"message,omitempty"`
}

// NotificationType defines types of search notifications
type NotificationType string

const (
	NotificationNewMatch     NotificationType = "new_match"      // New file matches the search
	NotificationUpdatedMatch NotificationType = "updated_match"  // Existing match was updated
	NotificationRemovedMatch NotificationType = "removed_match"  // File no longer matches
	NotificationFullRefresh  NotificationType = "full_refresh"   // Complete result set changed
	NotificationError        NotificationType = "error"          // Error occurred
)

// SubscriptionOptions configures subscription behavior
type SubscriptionOptions struct {
	PollInterval    time.Duration `json:"poll_interval"`    // How often to check for updates
	MaxNotifications int          `json:"max_notifications"` // Buffer size for notifications
	IncludeContent   bool         `json:"include_content"`   // Include full content in notifications
	OnlyChanges      bool         `json:"only_changes"`      // Only send actual changes, not full refreshes
}

// SearchAggregator provides aggregation and analytics for search results
type SearchAggregator struct {
	mu sync.RWMutex
}

// AggregationRequest represents a request for search result aggregation
type AggregationRequest struct {
	Query        *FullTextSearchRequest `json:"query"`
	GroupBy      []string               `json:"group_by"`      // Fields to group by: "extension", "size_range", "date_range", "author"
	Aggregations []string               `json:"aggregations"`  // "count", "sum", "avg", "min", "max"
	TimeRange    string                 `json:"time_range,omitempty"` // "hour", "day", "week", "month", "year"
}

// AggregationResponse contains aggregated search results
type AggregationResponse struct {
	Groups      []GroupResult     `json:"groups"`
	Summary     AggregationSummary `json:"summary"`
	QueryTime   time.Duration     `json:"query_time"`
	TotalDocs   int               `json:"total_docs"`
}

// GroupResult represents results for a single group
type GroupResult struct {
	GroupKey    string                 `json:"group_key"`
	GroupValue  string                 `json:"group_value"`
	Count       int                    `json:"count"`
	Metrics     map[string]interface{} `json:"metrics"`
	SampleFiles []SearchResult         `json:"sample_files,omitempty"`
}

// AggregationSummary provides summary statistics
type AggregationSummary struct {
	TotalGroups   int                    `json:"total_groups"`
	TotalResults  int                    `json:"total_results"`
	TopGroups     []GroupResult          `json:"top_groups"`
	Metrics       map[string]interface{} `json:"metrics"`
}

// NewSearchSubscriber creates a new search subscriber
func NewSearchSubscriber() *SearchSubscriber {
	return &SearchSubscriber{
		subscriptions: make(map[string]*SearchSubscription),
		nextID:        1,
	}
}

// Subscribe creates a new search subscription
func (ss *SearchSubscriber) Subscribe(ctx context.Context, query *FullTextSearchRequest, options SubscriptionOptions) (*SearchSubscription, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	
	// Set default options
	if options.PollInterval == 0 {
		options.PollInterval = 5 * time.Second
	}
	if options.MaxNotifications == 0 {
		options.MaxNotifications = 100
	}
	
	// Create subscription
	subCtx, cancel := context.WithCancel(ctx)
	id := fmt.Sprintf("sub_%d", ss.nextID)
	ss.nextID++
	
	subscription := &SearchSubscription{
		ID:          id,
		Query:       query,
		Channel:     make(chan SearchNotification, options.MaxNotifications),
		LastResults: []SearchResult{},
		Created:     time.Now(),
		LastUpdate:  time.Now(),
		Active:      true,
		Context:     subCtx,
		Cancel:      cancel,
		Options:     options,
	}
	
	ss.subscriptions[id] = subscription
	
	return subscription, nil
}

// Unsubscribe removes a search subscription
func (ss *SearchSubscriber) Unsubscribe(subscriptionID string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	
	sub, exists := ss.subscriptions[subscriptionID]
	if !exists {
		return fmt.Errorf("subscription %s not found", subscriptionID)
	}
	
	sub.Active = false
	sub.Cancel()
	close(sub.Channel)
	delete(ss.subscriptions, subscriptionID)
	
	return nil
}

// GetSubscription returns a subscription by ID
func (ss *SearchSubscriber) GetSubscription(subscriptionID string) (*SearchSubscription, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	
	sub, exists := ss.subscriptions[subscriptionID]
	if !exists {
		return nil, fmt.Errorf("subscription %s not found", subscriptionID)
	}
	
	return sub, nil
}

// ListSubscriptions returns all active subscriptions
func (ss *SearchSubscriber) ListSubscriptions() []SearchSubscription {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	
	subs := make([]SearchSubscription, 0, len(ss.subscriptions))
	for _, sub := range ss.subscriptions {
		if sub.Active {
			// Copy subscription without the channel and context
			subCopy := *sub
			subCopy.Channel = nil
			subCopy.Context = nil
			subCopy.Cancel = nil
			subs = append(subs, subCopy)
		}
	}
	
	return subs
}

// NotifyUpdate notifies subscriptions of search index updates
func (ss *SearchSubscriber) NotifyUpdate(advancedSearch *AdvancedSearch, updatedPath string, updateType string) {
	ss.mu.RLock()
	activeSubscriptions := make([]*SearchSubscription, 0, len(ss.subscriptions))
	for _, sub := range ss.subscriptions {
		if sub.Active {
			activeSubscriptions = append(activeSubscriptions, sub)
		}
	}
	ss.mu.RUnlock()
	
	// Process each subscription in a goroutine to avoid blocking
	for _, sub := range activeSubscriptions {
		go ss.processSubscriptionUpdate(advancedSearch, sub, updatedPath, updateType)
	}
}

// processSubscriptionUpdate processes updates for a single subscription
func (ss *SearchSubscriber) processSubscriptionUpdate(advancedSearch *AdvancedSearch, sub *SearchSubscription, updatedPath string, updateType string) {
	select {
	case <-sub.Context.Done():
		return // Subscription cancelled
	default:
	}
	
	// Re-run the search query
	response, err := advancedSearch.FullTextSearch(sub.Query)
	if err != nil {
		notification := SearchNotification{
			SubscriptionID: sub.ID,
			Type:           NotificationError,
			Timestamp:      time.Now(),
			Message:        fmt.Sprintf("Search error: %v", err),
		}
		
		select {
		case sub.Channel <- notification:
		default:
			// Channel full, drop notification
		}
		return
	}
	
	// Compare with previous results
	changes := ss.compareResults(sub.LastResults, response.Results, sub.Options.OnlyChanges)
	
	// Update subscription
	sub.LastResults = response.Results
	sub.LastUpdate = time.Now()
	
	// Send notifications for changes
	for _, notification := range changes {
		notification.SubscriptionID = sub.ID
		notification.Timestamp = time.Now()
		
		select {
		case sub.Channel <- notification:
		default:
			// Channel full, drop notification
		}
	}
}

// compareResults compares old and new results to find changes
func (ss *SearchSubscriber) compareResults(oldResults, newResults []SearchResult, onlyChanges bool) []SearchNotification {
	var notifications []SearchNotification
	
	// Create maps for easier comparison
	oldMap := make(map[string]SearchResult)
	newMap := make(map[string]SearchResult)
	
	for _, result := range oldResults {
		oldMap[result.Document.Path] = result
	}
	
	for _, result := range newResults {
		newMap[result.Document.Path] = result
	}
	
	// Find new matches
	for path, newResult := range newMap {
		if _, existed := oldMap[path]; !existed {
			notifications = append(notifications, SearchNotification{
				Type:      NotificationNewMatch,
				NewResult: &newResult,
			})
		}
	}
	
	// Find removed matches
	for path := range oldMap {
		if _, exists := newMap[path]; !exists {
			notifications = append(notifications, SearchNotification{
				Type:        NotificationRemovedMatch,
				RemovedPath: path,
			})
		}
	}
	
	// Find updated matches (changed scores or content)
	for path, newResult := range newMap {
		if oldResult, existed := oldMap[path]; existed {
			if ss.resultChanged(oldResult, newResult) {
				notifications = append(notifications, SearchNotification{
					Type:          NotificationUpdatedMatch,
					UpdatedResult: &newResult,
				})
			}
		}
	}
	
	// If no specific changes but results are different, send full refresh
	if !onlyChanges && len(notifications) == 0 && len(oldResults) != len(newResults) {
		notifications = append(notifications, SearchNotification{
			Type:    NotificationFullRefresh,
			Results: newResults,
		})
	}
	
	return notifications
}

// resultChanged checks if a search result has changed significantly
func (ss *SearchSubscriber) resultChanged(old, new SearchResult) bool {
	// Check if score changed significantly (more than 1%)
	scoreDiff := new.Score - old.Score
	if scoreDiff < 0 {
		scoreDiff = -scoreDiff
	}
	if old.Score > 0 && (scoreDiff/old.Score) > 0.01 {
		return true
	}
	
	// Check if document size changed
	if old.Document.Size != new.Document.Size {
		return true
	}
	
	// Check if last modified time changed
	if !old.Document.LastModified.Equal(new.Document.LastModified) {
		return true
	}
	
	return false
}

// StartPolling starts background polling for all subscriptions
func (ss *SearchSubscriber) StartPolling(ctx context.Context, advancedSearch *AdvancedSearch) {
	ticker := time.NewTicker(1 * time.Second) // Check every second for subscriptions that need updates
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ss.pollSubscriptions(advancedSearch)
		}
	}
}

// pollSubscriptions checks which subscriptions need updates
func (ss *SearchSubscriber) pollSubscriptions(advancedSearch *AdvancedSearch) {
	ss.mu.RLock()
	now := time.Now()
	var subscriptionsToUpdate []*SearchSubscription
	
	for _, sub := range ss.subscriptions {
		if sub.Active && now.Sub(sub.LastUpdate) >= sub.Options.PollInterval {
			subscriptionsToUpdate = append(subscriptionsToUpdate, sub)
		}
	}
	ss.mu.RUnlock()
	
	// Update subscriptions that are due
	for _, sub := range subscriptionsToUpdate {
		go ss.processSubscriptionUpdate(advancedSearch, sub, "", "poll")
	}
}

// NewSearchAggregator creates a new search aggregator
func NewSearchAggregator() *SearchAggregator {
	return &SearchAggregator{}
}

// Aggregate performs aggregation on search results
func (sa *SearchAggregator) Aggregate(advancedSearch *AdvancedSearch, req *AggregationRequest) (*AggregationResponse, error) {
	start := time.Now()
	
	// First, get the search results
	searchResponse, err := advancedSearch.FullTextSearch(req.Query)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	
	// Group results
	groups := sa.groupResults(searchResponse.Results, req.GroupBy)
	
	// Calculate aggregations for each group
	for i := range groups {
		groups[i].Metrics = sa.calculateGroupMetrics(groups[i], req.Aggregations)
	}
	
	// Calculate summary
	summary := sa.calculateSummary(groups, searchResponse.Results)
	
	return &AggregationResponse{
		Groups:    groups,
		Summary:   summary,
		QueryTime: time.Since(start),
		TotalDocs: len(searchResponse.Results),
	}, nil
}

// groupResults groups search results by specified fields
func (sa *SearchAggregator) groupResults(results []SearchResult, groupBy []string) []GroupResult {
	if len(groupBy) == 0 {
		// No grouping, return all results as one group
		return []GroupResult{
			{
				GroupKey:    "all",
				GroupValue:  "All Results",
				Count:       len(results),
				SampleFiles: results[:min(len(results), 5)], // Max 5 samples
			},
		}
	}
	
	groups := make(map[string][]SearchResult)
	
	for _, result := range results {
		groupKey := sa.generateGroupKey(result, groupBy)
		groups[groupKey] = append(groups[groupKey], result)
	}
	
	// Convert to GroupResult slice
	var groupResults []GroupResult
	for key, groupFiles := range groups {
		group := GroupResult{
			GroupKey:    groupBy[0], // Primary grouping field
			GroupValue:  key,
			Count:       len(groupFiles),
			SampleFiles: groupFiles[:min(len(groupFiles), 5)],
			Metrics:     make(map[string]interface{}),
		}
		groupResults = append(groupResults, group)
	}
	
	return groupResults
}

// generateGroupKey generates a grouping key for a result
func (sa *SearchAggregator) generateGroupKey(result SearchResult, groupBy []string) string {
	if len(groupBy) == 0 {
		return "default"
	}
	
	field := groupBy[0]
	switch field {
	case "extension":
		ext := filepath.Ext(result.Document.Path)
		if ext == "" {
			return "no_extension"
		}
		return ext[1:] // Remove the dot
		
	case "size_range":
		size := result.Document.Size
		if size < 1024 {
			return "< 1KB"
		} else if size < 10*1024 {
			return "1KB - 10KB"
		} else if size < 100*1024 {
			return "10KB - 100KB"
		} else if size < 1024*1024 {
			return "100KB - 1MB"
		} else {
			return "> 1MB"
		}
		
	case "date_range":
		now := time.Now()
		diff := now.Sub(result.Document.LastModified)
		if diff < 24*time.Hour {
			return "Today"
		} else if diff < 7*24*time.Hour {
			return "This Week"
		} else if diff < 30*24*time.Hour {
			return "This Month"
		} else if diff < 365*24*time.Hour {
			return "This Year"
		} else {
			return "Older"
		}
		
	case "path":
		// Group by directory
		dir := filepath.Dir(result.Document.Path)
		if dir == "." {
			return "root"
		}
		return dir
		
	default:
		return "unknown"
	}
}

// calculateGroupMetrics calculates metrics for a group
func (sa *SearchAggregator) calculateGroupMetrics(group GroupResult, aggregations []string) map[string]interface{} {
	metrics := make(map[string]interface{})
	
	if len(group.SampleFiles) == 0 {
		return metrics
	}
	
	for _, agg := range aggregations {
		switch agg {
		case "count":
			metrics["count"] = group.Count
			
		case "avg_score":
			totalScore := 0.0
			for _, result := range group.SampleFiles {
				totalScore += result.Score
			}
			metrics["avg_score"] = totalScore / float64(len(group.SampleFiles))
			
		case "avg_size":
			totalSize := int64(0)
			for _, result := range group.SampleFiles {
				totalSize += result.Document.Size
			}
			metrics["avg_size"] = float64(totalSize) / float64(len(group.SampleFiles))
			
		case "total_size":
			totalSize := int64(0)
			for _, result := range group.SampleFiles {
				totalSize += result.Document.Size
			}
			metrics["total_size"] = totalSize
			
		case "max_score":
			maxScore := 0.0
			for _, result := range group.SampleFiles {
				if result.Score > maxScore {
					maxScore = result.Score
				}
			}
			metrics["max_score"] = maxScore
			
		case "min_score":
			minScore := float64(999999)
			for _, result := range group.SampleFiles {
				if result.Score < minScore {
					minScore = result.Score
				}
			}
			metrics["min_score"] = minScore
		}
	}
	
	return metrics
}

// calculateSummary calculates summary statistics
func (sa *SearchAggregator) calculateSummary(groups []GroupResult, results []SearchResult) AggregationSummary {
	summary := AggregationSummary{
		TotalGroups:  len(groups),
		TotalResults: len(results),
		Metrics:      make(map[string]interface{}),
	}
	
	// Sort groups by count for top groups
	sortedGroups := make([]GroupResult, len(groups))
	copy(sortedGroups, groups)
	
	sort.Slice(sortedGroups, func(i, j int) bool {
		return sortedGroups[i].Count > sortedGroups[j].Count
	})
	
	// Top 5 groups
	topCount := min(5, len(sortedGroups))
	summary.TopGroups = sortedGroups[:topCount]
	
	// Overall metrics
	if len(results) > 0 {
		totalScore := 0.0
		totalSize := int64(0)
		for _, result := range results {
			totalScore += result.Score
			totalSize += result.Document.Size
		}
		summary.Metrics["avg_score"] = totalScore / float64(len(results))
		summary.Metrics["avg_size"] = float64(totalSize) / float64(len(results))
		summary.Metrics["total_size"] = totalSize
	}
	
	return summary
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

