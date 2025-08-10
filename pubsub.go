package govc

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// EventType represents the type of repository event
type EventType int

const (
	// File events
	FileAdded EventType = iota
	FileModified
	FileDeleted
	
	// Commit events
	CommitCreated
	CommitAmended
	
	// Branch events
	BranchCreated
	BranchDeleted
	BranchMerged
	BranchSwitched
	
	// Repository events
	RepositoryCompacted
	RepositoryCorrupted
	RepositoryRecovered
)

// String returns the string representation of the event type
func (et EventType) String() string {
	switch et {
	case FileAdded:
		return "FileAdded"
	case FileModified:
		return "FileModified"
	case FileDeleted:
		return "FileDeleted"
	case CommitCreated:
		return "CommitCreated"
	case CommitAmended:
		return "CommitAmended"
	case BranchCreated:
		return "BranchCreated"
	case BranchDeleted:
		return "BranchDeleted"
	case BranchMerged:
		return "BranchMerged"
	case BranchSwitched:
		return "BranchSwitched"
	case RepositoryCompacted:
		return "RepositoryCompacted"
	case RepositoryCorrupted:
		return "RepositoryCorrupted"
	case RepositoryRecovered:
		return "RepositoryRecovered"
	default:
		return "Unknown"
	}
}

// RepoEvent represents a repository event
type RepoEvent struct {
	ID        string                 `json:"id"`
	Type      EventType              `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source"`    // Branch or transaction ID
	Path      string                 `json:"path"`      // Affected file path (if applicable)
	OldValue  []byte                 `json:"old_value"` // Previous content
	NewValue  []byte                 `json:"new_value"` // New content
	Metadata  map[string]interface{} `json:"metadata"`  // Additional event data
}

// EventHandler is a function that processes events
type EventHandler func(event RepoEvent) error

// SimpleEventHandler is a simplified handler for file changes
type SimpleEventHandler func(path string, oldValue, newValue []byte)

// CommitEventHandler is a specialized handler for commit events
type CommitEventHandler func(commitHash string, files []string)

// UnsubscribeFunc is a function that cancels a subscription
type UnsubscribeFunc func()

// Subscription represents an active event subscription
type Subscription struct {
	ID           string            `json:"id"`
	Pattern      string            `json:"pattern"`      // File pattern: "*.json", "users/*"
	EventTypes   []EventType       `json:"event_types"`  // Which events to receive
	Handler      EventHandler      `json:"-"`            // Event handler function
	BufferSize   int               `json:"buffer_size"`  // Channel buffer size
	Active       bool              `json:"active"`       // Whether subscription is active
	Created      time.Time         `json:"created"`      // When subscription was created
	LastEvent    time.Time         `json:"last_event"`   // Last event delivery time
	EventCount   int64             `json:"event_count"`  // Total events delivered
	ErrorCount   int64             `json:"error_count"`  // Handler errors
	
	// Internal fields
	channel     chan RepoEvent  `json:"-"`
	done        chan struct{}   `json:"-"`
	mu          sync.RWMutex    `json:"-"`
}

// IsActive returns whether the subscription is active
func (s *Subscription) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Active
}

// IncrementEventCount atomically increments the event count
func (s *Subscription) IncrementEventCount() {
	atomic.AddInt64(&s.EventCount, 1)
	s.mu.Lock()
	s.LastEvent = time.Now()
	s.mu.Unlock()
}

// IncrementErrorCount atomically increments the error count
func (s *Subscription) IncrementErrorCount() {
	atomic.AddInt64(&s.ErrorCount, 1)
}

// GetStats returns subscription statistics
func (s *Subscription) GetStats() SubscriptionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return SubscriptionStats{
		ID:         s.ID,
		Pattern:    s.Pattern,
		EventCount: atomic.LoadInt64(&s.EventCount),
		ErrorCount: atomic.LoadInt64(&s.ErrorCount),
		LastEvent:  s.LastEvent,
		Active:     s.Active,
	}
}

// SubscriptionStats contains subscription statistics
type SubscriptionStats struct {
	ID         string    `json:"id"`
	Pattern    string    `json:"pattern"`
	EventCount int64     `json:"event_count"`
	ErrorCount int64     `json:"error_count"`
	LastEvent  time.Time `json:"last_event"`
	Active     bool      `json:"active"`
}

// PatternMatcher handles pattern matching for subscriptions
type PatternMatcher struct {
	mu       sync.RWMutex
	patterns map[string]*CompiledPattern
}

// CompiledPattern represents a compiled glob pattern
type CompiledPattern struct {
	Original string
	IsGlob   bool
	Parts    []string
}

// NewPatternMatcher creates a new pattern matcher
func NewPatternMatcher() *PatternMatcher {
	return &PatternMatcher{
		patterns: make(map[string]*CompiledPattern),
	}
}

// Match checks if a path matches a pattern
func (pm *PatternMatcher) Match(pattern, path string) bool {
	pm.mu.RLock()
	compiled, exists := pm.patterns[pattern]
	pm.mu.RUnlock()
	
	if !exists {
		compiled = pm.compilePattern(pattern)
		pm.mu.Lock()
		pm.patterns[pattern] = compiled
		pm.mu.Unlock()
	}
	
	return pm.matchCompiled(compiled, path)
}

// compilePattern compiles a glob pattern for efficient matching
func (pm *PatternMatcher) compilePattern(pattern string) *CompiledPattern {
	compiled := &CompiledPattern{
		Original: pattern,
		IsGlob:   strings.ContainsAny(pattern, "*?[]"),
	}
	
	if compiled.IsGlob {
		// Split pattern into parts for efficient matching
		compiled.Parts = strings.Split(pattern, "/")
	}
	
	return compiled
}

// matchCompiled performs the actual pattern matching
func (pm *PatternMatcher) matchCompiled(compiled *CompiledPattern, path string) bool {
	if !compiled.IsGlob {
		// Exact match
		return compiled.Original == path
	}
	
	// Use filepath.Match for glob patterns
	matched, err := filepath.Match(compiled.Original, path)
	if err != nil {
		return false
	}
	return matched
}

// EventBus manages event subscriptions and delivery
type EventBus struct {
	subscribers   map[string]*Subscription  // ID -> Subscription
	patternIndex  map[string][]*Subscription // Pattern -> Subscriptions
	eventTypeIndex map[EventType][]*Subscription // EventType -> Subscriptions
	matcher       *PatternMatcher
	mu            sync.RWMutex
	
	// Performance tracking
	totalEvents   int64
	totalDelivered int64
	totalErrors   int64
	
	// Worker pool for event delivery
	workerPool    *EventWorkerPool
	eventQueue    chan *eventDelivery
	
	// Control channels
	stopCh        chan struct{}
	running       int32
}

// eventDelivery represents an event to be delivered
type eventDelivery struct {
	event RepoEvent
	subscription *Subscription
}

// EventWorkerPool manages worker goroutines for event delivery
type EventWorkerPool struct {
	workers   int
	workQueue chan *eventDelivery
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewEventWorkerPool creates a new event worker pool
func NewEventWorkerPool(workers int, queueSize int) *EventWorkerPool {
	return &EventWorkerPool{
		workers:   workers,
		workQueue: make(chan *eventDelivery, queueSize),
		stopCh:    make(chan struct{}),
	}
}

// Start starts the worker pool
func (ewp *EventWorkerPool) Start() {
	for i := 0; i < ewp.workers; i++ {
		ewp.wg.Add(1)
		go ewp.worker(i)
	}
}

// Stop stops the worker pool
func (ewp *EventWorkerPool) Stop() {
	select {
	case <-ewp.stopCh:
		// Already stopped
		return
	default:
		close(ewp.stopCh)
	}
	ewp.wg.Wait()
}

// Submit submits an event delivery job
func (ewp *EventWorkerPool) Submit(delivery *eventDelivery) bool {
	select {
	case ewp.workQueue <- delivery:
		// Submitted successfully
		return true
	case <-ewp.stopCh:
		// Worker pool is stopping
		return false
	default:
		// Queue is full, try to deliver directly without worker
		ewp.processDelivery(delivery)
		return true
	}
}

// worker is a worker goroutine that processes event deliveries
func (ewp *EventWorkerPool) worker(id int) {
	defer ewp.wg.Done()
	
	for {
		select {
		case <-ewp.stopCh:
			return
		case delivery := <-ewp.workQueue:
			if delivery != nil {
				ewp.processDelivery(delivery)
			}
		}
	}
}

// processDelivery processes a single event delivery
func (ewp *EventWorkerPool) processDelivery(delivery *eventDelivery) {
	if !delivery.subscription.IsActive() {
		return
	}
	
	// Deliver event to subscription
	select {
	case delivery.subscription.channel <- delivery.event:
		delivery.subscription.IncrementEventCount()
	default:
		// Subscription channel is full, increment error count
		delivery.subscription.IncrementErrorCount()
	}
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	eb := &EventBus{
		subscribers:    make(map[string]*Subscription),
		patternIndex:   make(map[string][]*Subscription),
		eventTypeIndex: make(map[EventType][]*Subscription),
		matcher:        NewPatternMatcher(),
		eventQueue:     make(chan *eventDelivery, 10000), // Large buffer for high throughput
		stopCh:         make(chan struct{}),
	}
	
	// Create worker pool with 20 workers and larger queue
	eb.workerPool = NewEventWorkerPool(20, 5000)
	
	return eb
}

// Start starts the event bus
func (eb *EventBus) Start() error {
	if !atomic.CompareAndSwapInt32(&eb.running, 0, 1) {
		return fmt.Errorf("event bus already running")
	}
	
	// Create new worker pool if needed (after restart)
	if eb.workerPool == nil {
		eb.workerPool = NewEventWorkerPool(20, 5000)
		// Recreate stop channel
		eb.stopCh = make(chan struct{})
	}
	
	eb.workerPool.Start()
	return nil
}

// Stop stops the event bus
func (eb *EventBus) Stop() error {
	if !atomic.CompareAndSwapInt32(&eb.running, 1, 0) {
		return fmt.Errorf("event bus not running")
	}
	
	// Stop worker pool
	eb.workerPool.Stop()
	eb.workerPool = nil
	
	// Close stop channel
	select {
	case <-eb.stopCh:
		// Already closed
	default:
		close(eb.stopCh)
	}
	
	// Close all subscription channels
	eb.mu.Lock()
	for _, sub := range eb.subscribers {
		sub.mu.Lock()
		if sub.Active {
			sub.Active = false
			select {
			case <-sub.done:
				// Already closed
			default:
				close(sub.done)
			}
			close(sub.channel)
		}
		sub.mu.Unlock()
	}
	eb.mu.Unlock()
	
	return nil
}

// Subscribe creates a new subscription with the given pattern and event handler
func (eb *EventBus) Subscribe(pattern string, handler SimpleEventHandler) UnsubscribeFunc {
	return eb.SubscribeWithTypes(pattern, []EventType{FileAdded, FileModified, FileDeleted}, 
		func(event RepoEvent) error {
			handler(event.Path, event.OldValue, event.NewValue)
			return nil
		})
}

// SubscribeWithTypes creates a subscription for specific event types
func (eb *EventBus) SubscribeWithTypes(pattern string, eventTypes []EventType, handler EventHandler) UnsubscribeFunc {
	subscription := &Subscription{
		ID:         generateSubscriptionID(),
		Pattern:    pattern,
		EventTypes: eventTypes,
		Handler:    handler,
		BufferSize: 1000, // Default buffer size
		Active:     true,
		Created:    time.Now(),
		channel:    make(chan RepoEvent, 1000),
		done:       make(chan struct{}),
	}
	
	eb.mu.Lock()
	eb.subscribers[subscription.ID] = subscription
	
	// Add to pattern index
	eb.patternIndex[pattern] = append(eb.patternIndex[pattern], subscription)
	
	// Add to event type index
	for _, eventType := range eventTypes {
		eb.eventTypeIndex[eventType] = append(eb.eventTypeIndex[eventType], subscription)
	}
	eb.mu.Unlock()
	
	// Start handler goroutine
	go eb.handleSubscription(subscription)
	
	// Return unsubscribe function
	return func() {
		eb.unsubscribe(subscription.ID)
	}
}

// OnCommit subscribes to commit events
func (eb *EventBus) OnCommit(handler CommitEventHandler) UnsubscribeFunc {
	return eb.SubscribeWithTypes("*", []EventType{CommitCreated}, func(event RepoEvent) error {
		if files, ok := event.Metadata["files"].([]string); ok {
			handler(event.Source, files)
		}
		return nil
	})
}

// handleSubscription processes events for a subscription
func (eb *EventBus) handleSubscription(sub *Subscription) {
	for {
		select {
		case <-sub.done:
			return
		case event := <-sub.channel:
			if err := sub.Handler(event); err != nil {
				sub.IncrementErrorCount()
			}
		}
	}
}

// Publish publishes an event to all matching subscriptions
func (eb *EventBus) Publish(event RepoEvent) {
	if atomic.LoadInt32(&eb.running) == 0 {
		return // Event bus not running
	}
	
	atomic.AddInt64(&eb.totalEvents, 1)
	
	eb.mu.RLock()
	
	// Find subscriptions that match this event
	var matchingSubscriptions []*Subscription
	
	// Check event type subscriptions
	if subs, exists := eb.eventTypeIndex[event.Type]; exists {
		for _, sub := range subs {
			if eb.matcher.Match(sub.Pattern, event.Path) {
				matchingSubscriptions = append(matchingSubscriptions, sub)
			}
		}
	}
	
	eb.mu.RUnlock()
	
	// Deliver to matching subscriptions
	delivered := int64(0)
	for _, sub := range matchingSubscriptions {
		if sub.IsActive() {
			if eb.workerPool.Submit(&eventDelivery{
				event:        event,
				subscription: sub,
			}) {
				delivered++
			}
		}
	}
	
	atomic.AddInt64(&eb.totalDelivered, delivered)
}

// unsubscribe removes a subscription
func (eb *EventBus) unsubscribe(subscriptionID string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	
	sub, exists := eb.subscribers[subscriptionID]
	if !exists {
		return
	}
	
	// Mark as inactive
	sub.mu.Lock()
	sub.Active = false
	sub.mu.Unlock()
	
	// Remove from subscribers map
	delete(eb.subscribers, subscriptionID)
	
	// Remove from pattern index
	if subs, exists := eb.patternIndex[sub.Pattern]; exists {
		for i, s := range subs {
			if s.ID == subscriptionID {
				eb.patternIndex[sub.Pattern] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}
	
	// Remove from event type index
	for _, eventType := range sub.EventTypes {
		if subs, exists := eb.eventTypeIndex[eventType]; exists {
			for i, s := range subs {
				if s.ID == subscriptionID {
					eb.eventTypeIndex[eventType] = append(subs[:i], subs[i+1:]...)
					break
				}
			}
		}
	}
	
	// Signal handler to stop (only if still active)
	if sub.Active {
		select {
		case <-sub.done:
			// Already closed
		default:
			close(sub.done)
		}
	}
}

// GetSubscriptions returns all active subscriptions
func (eb *EventBus) GetSubscriptions() []SubscriptionStats {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	
	stats := make([]SubscriptionStats, 0, len(eb.subscribers))
	for _, sub := range eb.subscribers {
		stats = append(stats, sub.GetStats())
	}
	
	return stats
}

// GetStats returns event bus statistics
func (eb *EventBus) GetStats() EventBusStats {
	eb.mu.RLock()
	activeSubscriptions := len(eb.subscribers)
	eb.mu.RUnlock()
	
	return EventBusStats{
		ActiveSubscriptions: activeSubscriptions,
		TotalEvents:        atomic.LoadInt64(&eb.totalEvents),
		TotalDelivered:     atomic.LoadInt64(&eb.totalDelivered),
		TotalErrors:        atomic.LoadInt64(&eb.totalErrors),
	}
}

// EventBusStats contains event bus statistics
type EventBusStats struct {
	ActiveSubscriptions int   `json:"active_subscriptions"`
	TotalEvents        int64 `json:"total_events"`
	TotalDelivered     int64 `json:"total_delivered"`
	TotalErrors        int64 `json:"total_errors"`
}

// generateSubscriptionID generates a unique subscription ID
func generateSubscriptionID() string {
	return fmt.Sprintf("sub_%d_%d", time.Now().UnixNano(), atomic.AddInt64(&subscriptionCounter, 1))
}

var subscriptionCounter int64