package govc

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventBus_Subscribe tests basic subscription functionality
func TestEventBus_Subscribe(t *testing.T) {
	eb := NewEventBus()
	require.NoError(t, eb.Start())
	defer eb.Stop()
	
	var receivedEvents []RepoEvent
	var mu sync.Mutex
	
	// Subscribe to all .txt files
	unsubscribe := eb.Subscribe("*.txt", func(path string, oldValue, newValue []byte) {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, RepoEvent{
			Path:     path,
			OldValue: oldValue,
			NewValue: newValue,
		})
	})
	defer unsubscribe()
	
	// Publish events
	eb.Publish(RepoEvent{
		Type:     FileAdded,
		Path:     "test.txt",
		NewValue: []byte("content"),
	})
	
	eb.Publish(RepoEvent{
		Type:     FileAdded,
		Path:     "ignore.log", // Should not match
		NewValue: []byte("log content"),
	})
	
	eb.Publish(RepoEvent{
		Type:     FileModified,
		Path:     "another.txt",
		OldValue: []byte("old"),
		NewValue: []byte("new"),
	})
	
	// Wait for event processing
	time.Sleep(50 * time.Millisecond)
	
	mu.Lock()
	defer mu.Unlock()
	
	// Should receive 2 events (test.txt and another.txt) - order may vary due to concurrent processing
	assert.Equal(t, 2, len(receivedEvents))
	paths := []string{receivedEvents[0].Path, receivedEvents[1].Path}
	assert.Contains(t, paths, "test.txt")
	assert.Contains(t, paths, "another.txt")
}

// TestEventBus_Unsubscribe tests unsubscription functionality
func TestEventBus_Unsubscribe(t *testing.T) {
	eb := NewEventBus()
	require.NoError(t, eb.Start())
	defer eb.Stop()
	
	var eventCount int64
	
	unsubscribe := eb.Subscribe("*", func(path string, oldValue, newValue []byte) {
		atomic.AddInt64(&eventCount, 1)
	})
	
	// Publish event
	eb.Publish(RepoEvent{Type: FileAdded, Path: "test.txt"})
	time.Sleep(50 * time.Millisecond)
	
	assert.Equal(t, int64(1), atomic.LoadInt64(&eventCount))
	
	// Unsubscribe
	unsubscribe()
	time.Sleep(10 * time.Millisecond)
	
	// Publish another event
	eb.Publish(RepoEvent{Type: FileAdded, Path: "test2.txt"})
	time.Sleep(50 * time.Millisecond)
	
	// Should still be 1 (no new events received)
	assert.Equal(t, int64(1), atomic.LoadInt64(&eventCount))
}

// TestEventBus_PatternMatching tests various pattern matching scenarios
func TestEventBus_PatternMatching(t *testing.T) {
	eb := NewEventBus()
	require.NoError(t, eb.Start())
	defer eb.Stop()
	
	testCases := []struct {
		pattern string
		paths   []string
		matches []bool
	}{
		{
			pattern: "*.txt",
			paths:   []string{"file.txt", "test.log", "doc.txt"},
			matches: []bool{true, false, true},
		},
		{
			pattern: "users/*",
			paths:   []string{"users/alice.json", "users/bob.txt", "admin/alice.json"},
			matches: []bool{true, true, false},
		},
		{
			pattern: "players/*/position",
			paths:   []string{"players/alice/position", "players/bob/health", "world/position"},
			matches: []bool{true, false, false},
		},
		{
			pattern: "config.json",
			paths:   []string{"config.json", "app/config.json", "config.yaml"},
			matches: []bool{true, false, false},
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.pattern, func(t *testing.T) {
			var receivedPaths []string
			var mu sync.Mutex
			
			unsubscribe := eb.Subscribe(tc.pattern, func(path string, oldValue, newValue []byte) {
				mu.Lock()
				receivedPaths = append(receivedPaths, path)
				mu.Unlock()
			})
			defer unsubscribe()
			
			// Publish events for all test paths
			for _, path := range tc.paths {
				eb.Publish(RepoEvent{Type: FileAdded, Path: path})
			}
			
			time.Sleep(50 * time.Millisecond)
			
			mu.Lock()
			defer mu.Unlock()
			
			// Count expected matches
			expectedCount := 0
			for _, match := range tc.matches {
				if match {
					expectedCount++
				}
			}
			
			assert.Equal(t, expectedCount, len(receivedPaths), 
				"Pattern %s should match %d paths", tc.pattern, expectedCount)
		})
	}
}

// TestEventBus_EventTypes tests event type filtering
func TestEventBus_EventTypes(t *testing.T) {
	eb := NewEventBus()
	require.NoError(t, eb.Start())
	defer eb.Stop()
	
	var receivedTypes []EventType
	var mu sync.Mutex
	
	// Subscribe only to file additions and modifications
	unsubscribe := eb.SubscribeWithTypes("*.txt", []EventType{FileAdded, FileModified}, 
		func(event RepoEvent) error {
			mu.Lock()
			receivedTypes = append(receivedTypes, event.Type)
			mu.Unlock()
			return nil
		})
	defer unsubscribe()
	
	// Publish various event types
	events := []RepoEvent{
		{Type: FileAdded, Path: "test.txt"},
		{Type: FileModified, Path: "test.txt"},
		{Type: FileDeleted, Path: "test.txt"},
		{Type: CommitCreated, Path: "test.txt"},
		{Type: FileAdded, Path: "test.log"}, // Wrong pattern
	}
	
	for _, event := range events {
		eb.Publish(event)
	}
	
	time.Sleep(50 * time.Millisecond)
	
	mu.Lock()
	defer mu.Unlock()
	
	// Should receive only FileAdded and FileModified for .txt files
	assert.Equal(t, 2, len(receivedTypes))
	assert.Contains(t, receivedTypes, FileAdded)
	assert.Contains(t, receivedTypes, FileModified)
}

// TestEventBus_ConcurrentSubscribers tests multiple concurrent subscribers
func TestEventBus_ConcurrentSubscribers(t *testing.T) {
	eb := NewEventBus()
	require.NoError(t, eb.Start())
	defer eb.Stop()
	
	numSubscribers := 10
	eventsPerSubscriber := 100
	var totalReceived int64
	var subscribeWg sync.WaitGroup
	
	// Create multiple subscribers and wait for them to be ready
	var wg sync.WaitGroup
	for i := 0; i < numSubscribers; i++ {
		wg.Add(1)
		subscribeWg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			var count int64
			unsubscribe := eb.Subscribe("*", func(path string, oldValue, newValue []byte) {
				atomic.AddInt64(&count, 1)
			})
			defer unsubscribe()
			
			// Signal that this subscriber is ready
			subscribeWg.Done()
			
			// Wait for all events
			time.Sleep(200 * time.Millisecond)
			atomic.AddInt64(&totalReceived, atomic.LoadInt64(&count))
		}(i)
	}
	
	// Wait for all subscribers to be set up
	subscribeWg.Wait()
	time.Sleep(10 * time.Millisecond) // Small buffer to ensure subscription is fully processed
	
	// Publish events
	for i := 0; i < eventsPerSubscriber; i++ {
		eb.Publish(RepoEvent{
			Type: FileAdded,
			Path: fmt.Sprintf("file_%d.txt", i),
		})
	}
	
	wg.Wait()
	
	// Each subscriber should receive all events
	expectedTotal := int64(numSubscribers * eventsPerSubscriber)
	assert.Equal(t, expectedTotal, atomic.LoadInt64(&totalReceived))
}

// TestEventBus_HighFrequencyEvents tests high-frequency event publishing
func TestEventBus_HighFrequencyEvents(t *testing.T) {
	eb := NewEventBus()
	require.NoError(t, eb.Start())
	defer eb.Stop()
	
	var receivedCount int64
	var errorCount int64
	
	unsubscribe := eb.Subscribe("*", func(path string, oldValue, newValue []byte) {
		atomic.AddInt64(&receivedCount, 1)
		
		// Simulate processing time
		time.Sleep(time.Microsecond)
	})
	defer unsubscribe()
	
	// Publish events at high frequency
	numEvents := 1000
	go func() {
		for i := 0; i < numEvents; i++ {
			eb.Publish(RepoEvent{
				Type: FileAdded,
				Path: fmt.Sprintf("file_%d.txt", i),
			})
		}
	}()
	
	// Wait for processing
	time.Sleep(500 * time.Millisecond)
	
	received := atomic.LoadInt64(&receivedCount)
	errors := atomic.LoadInt64(&errorCount)
	
	t.Logf("High frequency test: %d/%d events received, %d errors", received, numEvents, errors)
	
	// Should receive most events (some may be dropped due to buffering)
	assert.GreaterOrEqual(t, received, int64(numEvents)*8/10) // At least 80%
	assert.Equal(t, int64(0), errors) // No errors expected
}

// TestEventBus_SubscriberFailure tests handling of subscriber errors
func TestEventBus_SubscriberFailure(t *testing.T) {
	eb := NewEventBus()
	require.NoError(t, eb.Start())
	defer eb.Stop()
	
	var successCount int64
	var errorCount int64
	
	// Subscriber that fails on certain events
	unsubscribe := eb.SubscribeWithTypes("*", []EventType{FileAdded}, func(event RepoEvent) error {
		if event.Path == "error.txt" {
			atomic.AddInt64(&errorCount, 1)
			return fmt.Errorf("simulated error")
		}
		atomic.AddInt64(&successCount, 1)
		return nil
	})
	defer unsubscribe()
	
	// Publish mix of successful and error events
	events := []string{"good.txt", "error.txt", "another.txt", "error.txt", "final.txt"}
	for _, path := range events {
		eb.Publish(RepoEvent{Type: FileAdded, Path: path})
	}
	
	time.Sleep(100 * time.Millisecond)
	
	success := atomic.LoadInt64(&successCount)
	errors := atomic.LoadInt64(&errorCount)
	
	assert.Equal(t, int64(3), success) // 3 successful events
	assert.Equal(t, int64(2), errors)  // 2 error events
	
	// Check subscription statistics
	subscriptions := eb.GetSubscriptions()
	assert.Equal(t, 1, len(subscriptions))
	assert.Equal(t, int64(5), subscriptions[0].EventCount)
	assert.Equal(t, int64(2), subscriptions[0].ErrorCount)
}

// TestEventBus_Stats tests statistics tracking
func TestEventBus_Stats(t *testing.T) {
	eb := NewEventBus()
	require.NoError(t, eb.Start())
	defer eb.Stop()
	
	// Create multiple subscriptions
	unsubscribe1 := eb.Subscribe("*.txt", func(path string, oldValue, newValue []byte) {})
	unsubscribe2 := eb.Subscribe("*.log", func(path string, oldValue, newValue []byte) {})
	defer unsubscribe1()
	defer unsubscribe2()
	
	// Publish events
	events := []RepoEvent{
		{Type: FileAdded, Path: "test.txt"},
		{Type: FileAdded, Path: "app.log"},
		{Type: FileModified, Path: "test.txt"},
		{Type: FileDeleted, Path: "old.log"},
	}
	
	for _, event := range events {
		eb.Publish(event)
	}
	
	time.Sleep(100 * time.Millisecond)
	
	stats := eb.GetStats()
	assert.Equal(t, 2, stats.ActiveSubscriptions)
	assert.Equal(t, int64(4), stats.TotalEvents)
	assert.GreaterOrEqual(t, stats.TotalDelivered, int64(4)) // At least 4 deliveries
}

// TestEventBus_StopAndRestart tests stopping and restarting the event bus
func TestEventBus_StopAndRestart(t *testing.T) {
	eb := NewEventBus()
	
	// Start event bus
	require.NoError(t, eb.Start())
	
	var eventCount int64
	unsubscribe := eb.Subscribe("*", func(path string, oldValue, newValue []byte) {
		atomic.AddInt64(&eventCount, 1)
	})
	
	// Publish event
	eb.Publish(RepoEvent{Type: FileAdded, Path: "test.txt"})
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(1), atomic.LoadInt64(&eventCount))
	
	// Stop event bus
	require.NoError(t, eb.Stop())
	
	// Try to publish event (should be ignored)
	eb.Publish(RepoEvent{Type: FileAdded, Path: "test2.txt"})
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(1), atomic.LoadInt64(&eventCount)) // Should remain 1
	
	// Restart event bus
	require.NoError(t, eb.Start())
	
	// Create new subscription (old one was closed)
	unsubscribe2 := eb.Subscribe("*", func(path string, oldValue, newValue []byte) {
		atomic.AddInt64(&eventCount, 1)
	})
	defer unsubscribe2()
	
	// Publish event
	eb.Publish(RepoEvent{Type: FileAdded, Path: "test3.txt"})
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(2), atomic.LoadInt64(&eventCount)) // Should be 2 now
	
	unsubscribe() // This should not panic even though bus was restarted
	eb.Stop()
}

// TestRepository_Subscribe tests repository-level subscription
func TestRepository_Subscribe(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	var receivedEvents []string
	var mu sync.Mutex
	
	// Subscribe to position updates
	unsubscribe := repo.Subscribe("players/*/position", func(path string, oldValue, newValue []byte) {
		mu.Lock()
		receivedEvents = append(receivedEvents, path)
		mu.Unlock()
	})
	defer unsubscribe()
	
	// Publish events through repository
	repo.publishEvent(FileModified, "players/alice/position", []byte("0,0"), []byte("10,20"), nil)
	repo.publishEvent(FileModified, "players/bob/position", []byte("5,5"), []byte("15,25"), nil)
	repo.publishEvent(FileModified, "players/alice/health", []byte("100"), []byte("90"), nil) // Should not match
	
	time.Sleep(50 * time.Millisecond)
	
	mu.Lock()
	defer mu.Unlock()
	
	assert.Equal(t, 2, len(receivedEvents))
	assert.Contains(t, receivedEvents, "players/alice/position")
	assert.Contains(t, receivedEvents, "players/bob/position")
}

// TestRepository_OnCommit tests commit event subscription
func TestRepository_OnCommit(t *testing.T) {
	repo := NewRepository()
	defer repo.eventBus.Stop()
	
	var receivedCommits []string
	var receivedFiles [][]string
	var mu sync.Mutex
	
	unsubscribe := repo.OnCommit(func(commitHash string, files []string) {
		mu.Lock()
		receivedCommits = append(receivedCommits, commitHash)
		receivedFiles = append(receivedFiles, files)
		mu.Unlock()
	})
	defer unsubscribe()
	
	// Publish commit events
	repo.publishCommitEvent("commit123", []string{"file1.txt", "file2.txt"})
	repo.publishCommitEvent("commit456", []string{"file3.txt"})
	
	time.Sleep(50 * time.Millisecond)
	
	mu.Lock()
	defer mu.Unlock()
	
	assert.Equal(t, 2, len(receivedCommits))
	assert.Contains(t, receivedCommits, "commit123")
	assert.Contains(t, receivedCommits, "commit456")
	
	// Check that the correct files are associated with the correct commits (order may vary)
	filesMap := make(map[string][]string)
	for i, commit := range receivedCommits {
		filesMap[commit] = receivedFiles[i]
	}
	assert.Equal(t, []string{"file1.txt", "file2.txt"}, filesMap["commit123"])
	assert.Equal(t, []string{"file3.txt"}, filesMap["commit456"])
}

// TestEventBus_BufferOverflow tests handling of buffer overflow scenarios
func TestEventBus_BufferOverflow(t *testing.T) {
	eb := NewEventBus()
	require.NoError(t, eb.Start())
	defer eb.Stop()
	
	// Create slow subscriber
	var processedCount int64
	unsubscribe := eb.Subscribe("*", func(path string, oldValue, newValue []byte) {
		atomic.AddInt64(&processedCount, 1)
		// Slow processing
		time.Sleep(10 * time.Millisecond)
	})
	defer unsubscribe()
	
	// Publish many events quickly
	numEvents := 200
	for i := 0; i < numEvents; i++ {
		eb.Publish(RepoEvent{
			Type: FileAdded,
			Path: fmt.Sprintf("file_%d.txt", i),
		})
	}
	
	// Give some time to process
	time.Sleep(500 * time.Millisecond)
	
	processed := atomic.LoadInt64(&processedCount)
	t.Logf("Buffer overflow test: processed %d/%d events", processed, numEvents)
	
	// Should handle gracefully, may not process all due to buffering
	assert.Greater(t, processed, int64(0))
	
	// Check subscription stats
	subscriptions := eb.GetSubscriptions()
	if len(subscriptions) > 0 {
		stats := subscriptions[0]
		t.Logf("Subscription stats: events=%d, errors=%d", stats.EventCount, stats.ErrorCount)
		// Some events may be dropped due to buffer overflow, but no errors should occur
	}
}