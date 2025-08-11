package optimize

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ConcurrentLimiter manages goroutine concurrency and resource usage
type ConcurrentLimiter struct {
	maxGoroutines int
	semaphore     chan struct{}
	
	// Stats
	activeGoroutines atomic.Int64
	totalStarted     atomic.Uint64
	totalCompleted   atomic.Uint64
	totalBlocked     atomic.Uint64
	totalErrors      atomic.Uint64
	
	// Worker pool
	workerPool *WorkerPool
}

// NewConcurrentLimiter creates a new concurrency limiter
func NewConcurrentLimiter(maxGoroutines int) *ConcurrentLimiter {
	if maxGoroutines <= 0 {
		maxGoroutines = runtime.NumCPU() * 2
	}
	
	cl := &ConcurrentLimiter{
		maxGoroutines: maxGoroutines,
		semaphore:     make(chan struct{}, maxGoroutines),
	}
	
	// Pre-fill semaphore
	for i := 0; i < maxGoroutines; i++ {
		cl.semaphore <- struct{}{}
	}
	
	// Create worker pool
	cl.workerPool = NewWorkerPool(maxGoroutines/2, maxGoroutines*2)
	
	return cl
}

// Do executes a function with concurrency limiting
func (cl *ConcurrentLimiter) Do(ctx context.Context, fn func() error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-cl.semaphore:
		// Acquired slot
	}
	
	cl.activeGoroutines.Add(1)
	cl.totalStarted.Add(1)
	
	defer func() {
		cl.semaphore <- struct{}{} // Release slot
		cl.activeGoroutines.Add(-1)
		cl.totalCompleted.Add(1)
	}()
	
	err := fn()
	if err != nil {
		cl.totalErrors.Add(1)
	}
	
	return err
}

// DoAsync executes a function asynchronously with limiting
func (cl *ConcurrentLimiter) DoAsync(ctx context.Context, fn func() error) <-chan error {
	errChan := make(chan error, 1)
	
	go func() {
		err := cl.Do(ctx, fn)
		errChan <- err
		close(errChan)
	}()
	
	return errChan
}

// TryDo attempts to execute a function without blocking
func (cl *ConcurrentLimiter) TryDo(fn func() error) (bool, error) {
	select {
	case <-cl.semaphore:
		// Acquired slot
		cl.activeGoroutines.Add(1)
		cl.totalStarted.Add(1)
		
		defer func() {
			cl.semaphore <- struct{}{} // Release slot
			cl.activeGoroutines.Add(-1)
			cl.totalCompleted.Add(1)
		}()
		
		err := fn()
		if err != nil {
			cl.totalErrors.Add(1)
		}
		
		return true, err
	default:
		// No slot available
		cl.totalBlocked.Add(1)
		return false, nil
	}
}

// Wait waits for all active goroutines to complete
func (cl *ConcurrentLimiter) Wait() {
	for cl.activeGoroutines.Load() > 0 {
		time.Sleep(10 * time.Millisecond)
	}
}

// WaitWithTimeout waits with a timeout
func (cl *ConcurrentLimiter) WaitWithTimeout(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	
	for cl.activeGoroutines.Load() > 0 {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	
	return true
}

// WorkerPool manages a pool of worker goroutines
type WorkerPool struct {
	minWorkers int
	maxWorkers int
	workQueue  chan func()
	
	activeWorkers atomic.Int32
	idleWorkers   atomic.Int32
	
	mu       sync.Mutex
	shutdown bool
	wg       sync.WaitGroup
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(minWorkers, maxWorkers int) *WorkerPool {
	if minWorkers <= 0 {
		minWorkers = 1
	}
	if maxWorkers < minWorkers {
		maxWorkers = minWorkers * 2
	}
	
	wp := &WorkerPool{
		minWorkers: minWorkers,
		maxWorkers: maxWorkers,
		workQueue:  make(chan func(), maxWorkers*2),
	}
	
	// Start minimum workers
	for i := 0; i < minWorkers; i++ {
		wp.startWorker()
	}
	
	// Start monitor
	go wp.monitor()
	
	return wp
}

// Submit submits work to the pool
func (wp *WorkerPool) Submit(fn func()) bool {
	wp.mu.Lock()
	if wp.shutdown {
		wp.mu.Unlock()
		return false
	}
	wp.mu.Unlock()
	
	select {
	case wp.workQueue <- fn:
		// Check if we need more workers
		if wp.idleWorkers.Load() == 0 && wp.activeWorkers.Load() < int32(wp.maxWorkers) {
			wp.startWorker()
		}
		return true
	default:
		// Queue full, start emergency worker if possible
		if wp.activeWorkers.Load() < int32(wp.maxWorkers) {
			wp.startWorker()
			wp.workQueue <- fn
			return true
		}
		return false
	}
}

// SubmitWait submits work and waits for completion
func (wp *WorkerPool) SubmitWait(fn func()) bool {
	done := make(chan struct{})
	
	submitted := wp.Submit(func() {
		fn()
		close(done)
	})
	
	if !submitted {
		return false
	}
	
	<-done
	return true
}

// startWorker starts a new worker goroutine
func (wp *WorkerPool) startWorker() {
	wp.wg.Add(1)
	wp.activeWorkers.Add(1)
	
	go func() {
		defer wp.wg.Done()
		defer wp.activeWorkers.Add(-1)
		
		idleTimer := time.NewTimer(30 * time.Second)
		defer idleTimer.Stop()
		
		for {
			wp.idleWorkers.Add(1)
			
			select {
			case work, ok := <-wp.workQueue:
				if !ok {
					// Channel closed, shutdown
					wp.idleWorkers.Add(-1)
					return
				}
				
				wp.idleWorkers.Add(-1)
				
				// Reset idle timer
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				idleTimer.Reset(30 * time.Second)
				
				// Execute work
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Handle panic
						}
					}()
					work()
				}()
				
			case <-idleTimer.C:
				wp.idleWorkers.Add(-1)
				
				// Check if we can shutdown this worker
				if wp.activeWorkers.Load() > int32(wp.minWorkers) {
					return
				}
				
				// Reset timer
				idleTimer.Reset(30 * time.Second)
			}
			
			// Check shutdown
			wp.mu.Lock()
			if wp.shutdown {
				wp.mu.Unlock()
				wp.idleWorkers.Add(-1)
				return
			}
			wp.mu.Unlock()
		}
	}()
}

// monitor adjusts the worker pool size based on load
func (wp *WorkerPool) monitor() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		wp.mu.Lock()
		if wp.shutdown {
			wp.mu.Unlock()
			return
		}
		wp.mu.Unlock()
		
		queueLen := len(wp.workQueue)
		activeWorkers := wp.activeWorkers.Load()
		idleWorkers := wp.idleWorkers.Load()
		
		// Scale up if queue is building up
		if queueLen > int(activeWorkers) && activeWorkers < int32(wp.maxWorkers) {
			newWorkers := (queueLen - int(activeWorkers)) / 2
			if newWorkers > wp.maxWorkers-int(activeWorkers) {
				newWorkers = wp.maxWorkers - int(activeWorkers)
			}
			
			for i := 0; i < newWorkers; i++ {
				wp.startWorker()
			}
		}
		
		// Scale down if too many idle workers
		if idleWorkers > int32(wp.minWorkers) && queueLen == 0 {
			// Workers will self-terminate after idle timeout
		}
	}
}

// Shutdown gracefully shuts down the pool
func (wp *WorkerPool) Shutdown(ctx context.Context) error {
	wp.mu.Lock()
	if wp.shutdown {
		wp.mu.Unlock()
		return nil
	}
	wp.shutdown = true
	wp.mu.Unlock()
	
	// Close work queue
	close(wp.workQueue)
	
	// Wait for workers with context
	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// BatchProcessor processes items in batches with concurrency control
type BatchProcessor struct {
	batchSize     int
	maxConcurrent int
	limiter       *ConcurrentLimiter
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(batchSize, maxConcurrent int) *BatchProcessor {
	return &BatchProcessor{
		batchSize:     batchSize,
		maxConcurrent: maxConcurrent,
		limiter:       NewConcurrentLimiter(maxConcurrent),
	}
}

// Process processes items in batches
func (bp *BatchProcessor) Process(ctx context.Context, items []interface{}, processFn func([]interface{}) error) error {
	if len(items) == 0 {
		return nil
	}
	
	// Split into batches
	batches := make([][]interface{}, 0)
	for i := 0; i < len(items); i += bp.batchSize {
		end := i + bp.batchSize
		if end > len(items) {
			end = len(items)
		}
		batches = append(batches, items[i:end])
	}
	
	// Process batches concurrently
	var wg sync.WaitGroup
	errChan := make(chan error, len(batches))
	
	for _, batch := range batches {
		batch := batch // Capture for closure
		wg.Add(1)
		
		go func() {
			defer wg.Done()
			err := bp.limiter.Do(ctx, func() error {
				return processFn(batch)
			})
			if err != nil {
				errChan <- err
			}
		}()
	}
	
	// Wait for completion
	wg.Wait()
	close(errChan)
	
	// Check for errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	
	return nil
}

// Stats returns limiter statistics
func (cl *ConcurrentLimiter) Stats() ConcurrencyStats {
	return ConcurrencyStats{
		ActiveGoroutines: cl.activeGoroutines.Load(),
		TotalStarted:     cl.totalStarted.Load(),
		TotalCompleted:   cl.totalCompleted.Load(),
		TotalBlocked:     cl.totalBlocked.Load(),
		TotalErrors:      cl.totalErrors.Load(),
		MaxGoroutines:    cl.maxGoroutines,
	}
}

// ConcurrencyStats contains concurrency statistics
type ConcurrencyStats struct {
	ActiveGoroutines int64
	TotalStarted     uint64
	TotalCompleted   uint64
	TotalBlocked     uint64
	TotalErrors      uint64
	MaxGoroutines    int
}