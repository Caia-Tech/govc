package optimize

import (
	"sync"
	"sync/atomic"
	"time"
)

// MemoryPool manages reusable memory buffers to reduce allocations
type MemoryPool struct {
	small  *BufferPool // 4KB buffers
	medium *BufferPool // 64KB buffers
	large  *BufferPool // 1MB buffers
	huge   *BufferPool // 16MB buffers
	
	stats PoolStats
}

// BufferPool manages a pool of byte slices
type BufferPool struct {
	pool sync.Pool
	size int
	
	gets    atomic.Uint64
	puts    atomic.Uint64
	news    atomic.Uint64
	inUse   atomic.Int64
}

// PoolStats tracks pool performance metrics
type PoolStats struct {
	TotalGets       atomic.Uint64
	TotalPuts       atomic.Uint64
	TotalNews       atomic.Uint64
	CurrentInUse    atomic.Int64
	LastGC          atomic.Int64
	GCCount         atomic.Uint64
}

const (
	SmallBufferSize  = 4 * 1024        // 4KB
	MediumBufferSize = 64 * 1024       // 64KB
	LargeBufferSize  = 1024 * 1024     // 1MB
	HugeBufferSize   = 16 * 1024 * 1024 // 16MB
)

// NewMemoryPool creates a new memory pool
func NewMemoryPool() *MemoryPool {
	mp := &MemoryPool{
		small:  newBufferPool(SmallBufferSize),
		medium: newBufferPool(MediumBufferSize),
		large:  newBufferPool(LargeBufferSize),
		huge:   newBufferPool(HugeBufferSize),
	}
	
	// Start GC monitor
	go mp.gcMonitor()
	
	return mp
}

// newBufferPool creates a buffer pool of specified size
func newBufferPool(size int) *BufferPool {
	bp := &BufferPool{
		size: size,
	}
	
	bp.pool.New = func() interface{} {
		bp.news.Add(1)
		return make([]byte, size)
	}
	
	return bp
}

// Get retrieves a buffer from the appropriate pool
func (mp *MemoryPool) Get(size int) []byte {
	mp.stats.TotalGets.Add(1)
	
	var pool *BufferPool
	switch {
	case size <= SmallBufferSize:
		pool = mp.small
	case size <= MediumBufferSize:
		pool = mp.medium
	case size <= LargeBufferSize:
		pool = mp.large
	case size <= HugeBufferSize:
		pool = mp.huge
	default:
		// For very large buffers, allocate directly
		return make([]byte, size)
	}
	
	pool.gets.Add(1)
	pool.inUse.Add(1)
	mp.stats.CurrentInUse.Add(1)
	
	buf := pool.pool.Get().([]byte)
	if len(buf) < size {
		// Buffer too small, allocate new one
		return make([]byte, size)
	}
	
	return buf[:size]
}

// Put returns a buffer to the pool
func (mp *MemoryPool) Put(buf []byte) {
	if buf == nil || len(buf) == 0 {
		return
	}
	
	mp.stats.TotalPuts.Add(1)
	
	var pool *BufferPool
	cap := cap(buf)
	switch {
	case cap <= SmallBufferSize:
		pool = mp.small
	case cap <= MediumBufferSize:
		pool = mp.medium
	case cap <= LargeBufferSize:
		pool = mp.large
	case cap <= HugeBufferSize:
		pool = mp.huge
	default:
		// Don't pool very large buffers
		return
	}
	
	pool.puts.Add(1)
	pool.inUse.Add(-1)
	mp.stats.CurrentInUse.Add(-1)
	
	// Clear the buffer before returning to pool
	for i := range buf {
		buf[i] = 0
	}
	
	pool.pool.Put(buf[:cap])
}

// GetString gets a buffer and returns it as a string builder
type StringBuilder struct {
	buf []byte
	pos int
	mp  *MemoryPool
}

// NewStringBuilder creates a string builder backed by pooled memory
func (mp *MemoryPool) NewStringBuilder(estimatedSize int) *StringBuilder {
	return &StringBuilder{
		buf: mp.Get(estimatedSize),
		pos: 0,
		mp:  mp,
	}
}

// Write appends bytes to the builder
func (sb *StringBuilder) Write(p []byte) (int, error) {
	needed := sb.pos + len(p)
	if needed > len(sb.buf) {
		// Need to grow
		newSize := len(sb.buf) * 2
		if newSize < needed {
			newSize = needed
		}
		
		newBuf := sb.mp.Get(newSize)
		copy(newBuf, sb.buf[:sb.pos])
		sb.mp.Put(sb.buf)
		sb.buf = newBuf
	}
	
	copy(sb.buf[sb.pos:], p)
	sb.pos += len(p)
	return len(p), nil
}

// String returns the built string
func (sb *StringBuilder) String() string {
	return string(sb.buf[:sb.pos])
}

// Release returns the buffer to the pool
func (sb *StringBuilder) Release() {
	sb.mp.Put(sb.buf)
	sb.buf = nil
	sb.pos = 0
}

// gcMonitor periodically triggers GC to return memory to OS
func (mp *MemoryPool) gcMonitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		inUse := mp.stats.CurrentInUse.Load()
		if inUse == 0 {
			// No buffers in use, safe to GC
			mp.stats.LastGC.Store(time.Now().Unix())
			mp.stats.GCCount.Add(1)
			
			// Clear pools to return memory to OS
			mp.small.pool = sync.Pool{New: mp.small.pool.New}
			mp.medium.pool = sync.Pool{New: mp.medium.pool.New}
			mp.large.pool = sync.Pool{New: mp.large.pool.New}
			mp.huge.pool = sync.Pool{New: mp.huge.pool.New}
		}
	}
}

// Stats returns current pool statistics
func (mp *MemoryPool) Stats() PoolStatsSnapshot {
	return PoolStatsSnapshot{
		TotalGets:    mp.stats.TotalGets.Load(),
		TotalPuts:    mp.stats.TotalPuts.Load(),
		TotalNews:    mp.stats.TotalNews.Load(),
		CurrentInUse: mp.stats.CurrentInUse.Load(),
		LastGC:       time.Unix(mp.stats.LastGC.Load(), 0),
		GCCount:      mp.stats.GCCount.Load(),
		
		SmallPoolStats: BufferPoolStats{
			Gets:  mp.small.gets.Load(),
			Puts:  mp.small.puts.Load(),
			News:  mp.small.news.Load(),
			InUse: mp.small.inUse.Load(),
		},
		MediumPoolStats: BufferPoolStats{
			Gets:  mp.medium.gets.Load(),
			Puts:  mp.medium.puts.Load(),
			News:  mp.medium.news.Load(),
			InUse: mp.medium.inUse.Load(),
		},
		LargePoolStats: BufferPoolStats{
			Gets:  mp.large.gets.Load(),
			Puts:  mp.large.puts.Load(),
			News:  mp.large.news.Load(),
			InUse: mp.large.inUse.Load(),
		},
		HugePoolStats: BufferPoolStats{
			Gets:  mp.huge.gets.Load(),
			Puts:  mp.huge.puts.Load(),
			News:  mp.huge.news.Load(),
			InUse: mp.huge.inUse.Load(),
		},
	}
}

// PoolStatsSnapshot is a point-in-time snapshot of pool statistics
type PoolStatsSnapshot struct {
	TotalGets    uint64
	TotalPuts    uint64
	TotalNews    uint64
	CurrentInUse int64
	LastGC       time.Time
	GCCount      uint64
	
	SmallPoolStats  BufferPoolStats
	MediumPoolStats BufferPoolStats
	LargePoolStats  BufferPoolStats
	HugePoolStats   BufferPoolStats
}

// BufferPoolStats contains statistics for a single buffer pool
type BufferPoolStats struct {
	Gets  uint64
	Puts  uint64
	News  uint64
	InUse int64
}

// EfficiencyRatio returns the ratio of gets to news (higher is better)
func (s PoolStatsSnapshot) EfficiencyRatio() float64 {
	if s.TotalNews == 0 {
		return 0
	}
	return float64(s.TotalGets) / float64(s.TotalNews)
}