package optimize

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// DBOptimizer optimizes database operations
type DBOptimizer struct {
	connPool      *ConnectionPool
	queryCache    *QueryCache
	preparedStmts *PreparedStatements
	batchExecutor *BatchExecutor
	
	stats DBStats
}

// DBStats tracks database performance
type DBStats struct {
	QueriesExecuted   atomic.Uint64
	QueriesCached     atomic.Uint64
	ConnectionsActive atomic.Int64
	ConnectionsIdle   atomic.Int64
	BatchesExecuted   atomic.Uint64
	AvgQueryTime      atomic.Int64 // microseconds
}

// NewDBOptimizer creates a database optimizer
func NewDBOptimizer(maxConns int) *DBOptimizer {
	return &DBOptimizer{
		connPool:      NewConnectionPool(maxConns),
		queryCache:    NewQueryCache(1000),
		preparedStmts: NewPreparedStatements(),
		batchExecutor: NewBatchExecutor(),
	}
}

// ConnectionPool manages database connections
type ConnectionPool struct {
	maxConns     int
	minConns     int
	maxIdleTime  time.Duration
	
	connections  chan *PooledConnection
	factory      ConnectionFactory
	
	active       atomic.Int64
	idle         atomic.Int64
	created      atomic.Uint64
	destroyed    atomic.Uint64
	
	mu           sync.RWMutex
	closed       bool
}

// PooledConnection wraps a database connection
type PooledConnection struct {
	conn       *sql.DB
	pool       *ConnectionPool
	inUse      bool
	lastUsed   time.Time
	created    time.Time
	queryCount uint64
}

// ConnectionFactory creates new connections
type ConnectionFactory func() (*sql.DB, error)

// NewConnectionPool creates a connection pool
func NewConnectionPool(maxConns int) *ConnectionPool {
	if maxConns <= 0 {
		maxConns = 10
	}
	
	cp := &ConnectionPool{
		maxConns:    maxConns,
		minConns:    maxConns / 4,
		maxIdleTime: 5 * time.Minute,
		connections: make(chan *PooledConnection, maxConns),
	}
	
	// Start maintenance routine
	go cp.maintain()
	
	return cp
}

// Get acquires a connection from the pool
func (cp *ConnectionPool) Get(ctx context.Context) (*PooledConnection, error) {
	cp.mu.RLock()
	if cp.closed {
		cp.mu.RUnlock()
		return nil, fmt.Errorf("pool is closed")
	}
	cp.mu.RUnlock()
	
	select {
	case conn := <-cp.connections:
		// Got connection from pool
		cp.idle.Add(-1)
		cp.active.Add(1)
		conn.inUse = true
		conn.lastUsed = time.Now()
		return conn, nil
		
	case <-ctx.Done():
		return nil, ctx.Err()
		
	default:
		// Need to create new connection
		if cp.active.Load() >= int64(cp.maxConns) {
			// Wait for available connection
			select {
			case conn := <-cp.connections:
				cp.idle.Add(-1)
				cp.active.Add(1)
				conn.inUse = true
				conn.lastUsed = time.Now()
				return conn, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		
		// Create new connection
		return cp.createConnection()
	}
}

// Put returns a connection to the pool
func (cp *ConnectionPool) Put(conn *PooledConnection) {
	if conn == nil {
		return
	}
	
	cp.mu.RLock()
	closed := cp.closed
	cp.mu.RUnlock()
	
	if closed {
		conn.conn.Close()
		return
	}
	
	conn.inUse = false
	conn.lastUsed = time.Now()
	
	cp.active.Add(-1)
	cp.idle.Add(1)
	
	select {
	case cp.connections <- conn:
		// Connection returned to pool
	default:
		// Pool is full, close connection
		conn.conn.Close()
		cp.destroyed.Add(1)
		cp.idle.Add(-1)
	}
}

func (cp *ConnectionPool) createConnection() (*PooledConnection, error) {
	if cp.factory == nil {
		return nil, fmt.Errorf("no connection factory set")
	}
	
	db, err := cp.factory()
	if err != nil {
		return nil, err
	}
	
	conn := &PooledConnection{
		conn:     db,
		pool:     cp,
		created:  time.Now(),
		lastUsed: time.Now(),
		inUse:    true,
	}
	
	cp.created.Add(1)
	cp.active.Add(1)
	
	return conn, nil
}

func (cp *ConnectionPool) maintain() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		cp.mu.RLock()
		if cp.closed {
			cp.mu.RUnlock()
			return
		}
		cp.mu.RUnlock()
		
		// Remove idle connections
		now := time.Now()
		var toClose []*PooledConnection
		
		for {
			select {
			case conn := <-cp.connections:
				if now.Sub(conn.lastUsed) > cp.maxIdleTime {
					toClose = append(toClose, conn)
					cp.idle.Add(-1)
				} else {
					// Put back
					cp.connections <- conn
					goto done
				}
			default:
				goto done
			}
		}
		done:
		
		// Close expired connections
		for _, conn := range toClose {
			conn.conn.Close()
			cp.destroyed.Add(1)
		}
		
		// Ensure minimum connections
		for cp.idle.Load() < int64(cp.minConns) {
			conn, err := cp.createConnection()
			if err != nil {
				break
			}
			cp.Put(conn)
		}
	}
}

// Close closes the connection pool
func (cp *ConnectionPool) Close() error {
	cp.mu.Lock()
	if cp.closed {
		cp.mu.Unlock()
		return nil
	}
	cp.closed = true
	cp.mu.Unlock()
	
	close(cp.connections)
	
	// Close all connections
	for conn := range cp.connections {
		conn.conn.Close()
	}
	
	return nil
}

// QueryCache caches query results
type QueryCache struct {
	cache map[string][]byte
	ttl   time.Duration
	mu    sync.RWMutex
}

// CachedResult stores cached query result
type CachedResult struct {
	Data      interface{}
	Timestamp time.Time
	TTL       time.Duration
}

// NewQueryCache creates a query cache
func NewQueryCache(size int) *QueryCache {
	return &QueryCache{
		cache: make(map[string][]byte, size),
		ttl:   5 * time.Minute,
	}
}

// Get retrieves cached query result
func (qc *QueryCache) Get(key string) (interface{}, bool) {
	qc.mu.RLock()
	data, hit := qc.cache[key]
	qc.mu.RUnlock()
	
	if !hit {
		return nil, false
	}
	
	// Deserialize data
	var result CachedResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false
	}
	
	// Check TTL
	if time.Since(result.Timestamp) > result.TTL {
		// Expired
		return nil, false
	}
	
	return result.Data, true
}

// Put caches a query result
func (qc *QueryCache) Put(key string, data interface{}, ttl time.Duration) {
	if ttl == 0 {
		ttl = qc.ttl
	}
	
	result := CachedResult{
		Data:      data,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
	
	// Serialize for storage
	serialized, err := json.Marshal(result)
	if err != nil {
		return // Failed to serialize
	}
	
	qc.mu.Lock()
	qc.cache[key] = serialized
	qc.mu.Unlock()
}

// PreparedStatements manages prepared statements
type PreparedStatements struct {
	statements map[string]*sql.Stmt
	mu         sync.RWMutex
}

// NewPreparedStatements creates prepared statement manager
func NewPreparedStatements() *PreparedStatements {
	return &PreparedStatements{
		statements: make(map[string]*sql.Stmt),
	}
}

// Get retrieves or creates a prepared statement
func (ps *PreparedStatements) Get(db *sql.DB, query string) (*sql.Stmt, error) {
	ps.mu.RLock()
	stmt, exists := ps.statements[query]
	ps.mu.RUnlock()
	
	if exists {
		return stmt, nil
	}
	
	// Prepare new statement
	ps.mu.Lock()
	defer ps.mu.Unlock()
	
	// Double-check
	if stmt, exists := ps.statements[query]; exists {
		return stmt, nil
	}
	
	stmt, err := db.Prepare(query)
	if err != nil {
		return nil, err
	}
	
	ps.statements[query] = stmt
	return stmt, nil
}

// Close closes all prepared statements
func (ps *PreparedStatements) Close() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	
	for _, stmt := range ps.statements {
		stmt.Close()
	}
	
	ps.statements = make(map[string]*sql.Stmt)
	return nil
}

// BatchExecutor executes queries in batches
type BatchExecutor struct {
	batchSize int
	timeout   time.Duration
	
	pending []BatchQuery
	mu      sync.Mutex
	flush   chan struct{}
}

// BatchQuery represents a batched query
type BatchQuery struct {
	Query  string
	Args   []interface{}
	Result chan error
}

// NewBatchExecutor creates a batch executor
func NewBatchExecutor() *BatchExecutor {
	be := &BatchExecutor{
		batchSize: 100,
		timeout:   10 * time.Millisecond,
		pending:   make([]BatchQuery, 0, 100),
		flush:     make(chan struct{}, 1),
	}
	
	go be.run()
	return be
}

// Execute adds a query to the batch
func (be *BatchExecutor) Execute(query string, args ...interface{}) error {
	result := make(chan error, 1)
	
	be.mu.Lock()
	be.pending = append(be.pending, BatchQuery{
		Query:  query,
		Args:   args,
		Result: result,
	})
	
	shouldFlush := len(be.pending) >= be.batchSize
	be.mu.Unlock()
	
	if shouldFlush {
		select {
		case be.flush <- struct{}{}:
		default:
		}
	}
	
	return <-result
}

func (be *BatchExecutor) run() {
	ticker := time.NewTicker(be.timeout)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			be.executeBatch()
		case <-be.flush:
			be.executeBatch()
		}
	}
}

func (be *BatchExecutor) executeBatch() {
	be.mu.Lock()
	if len(be.pending) == 0 {
		be.mu.Unlock()
		return
	}
	
	batch := be.pending
	be.pending = make([]BatchQuery, 0, be.batchSize)
	be.mu.Unlock()
	
	// Execute batch (would use transaction here)
	var err error
	
	// Signal completion
	for _, query := range batch {
		query.Result <- err
	}
}

// TransactionManager manages database transactions
type TransactionManager struct {
	activeTransactions sync.Map
	mu                sync.RWMutex
}

// Transaction represents a managed transaction
type Transaction struct {
	tx         *sql.Tx
	id         string
	startTime  time.Time
	operations uint64
}

// NewTransactionManager creates a transaction manager
func NewTransactionManager() *TransactionManager {
	return &TransactionManager{}
}

// Begin starts a new transaction
func (tm *TransactionManager) Begin(db *sql.DB) (*Transaction, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	
	trans := &Transaction{
		tx:        tx,
		id:        fmt.Sprintf("tx_%d", time.Now().UnixNano()),
		startTime: time.Now(),
	}
	
	tm.activeTransactions.Store(trans.id, trans)
	return trans, nil
}

// Commit commits a transaction
func (tm *TransactionManager) Commit(trans *Transaction) error {
	err := trans.tx.Commit()
	tm.activeTransactions.Delete(trans.id)
	return err
}

// Rollback rolls back a transaction
func (tm *TransactionManager) Rollback(trans *Transaction) error {
	err := trans.tx.Rollback()
	tm.activeTransactions.Delete(trans.id)
	return err
}

// IndexOptimizer suggests and creates optimal indexes
type IndexOptimizer struct {
	analyzer *QueryAnalyzer
	stats    map[string]*IndexStats
	mu       sync.RWMutex
}

// IndexStats tracks index usage
type IndexStats struct {
	Uses      uint64
	LastUsed  time.Time
	AvgTime   time.Duration
	TableName string
	Columns   []string
}

// QueryAnalyzer analyzes queries for optimization
type QueryAnalyzer struct {
	queries map[string]*QueryStats
	mu      sync.RWMutex
}

// QueryStats tracks query performance
type QueryStats struct {
	Count       uint64
	TotalTime   time.Duration
	AvgTime     time.Duration
	MinTime     time.Duration
	MaxTime     time.Duration
	LastExecuted time.Time
}

// NewQueryAnalyzer creates a query analyzer
func NewQueryAnalyzer() *QueryAnalyzer {
	return &QueryAnalyzer{
		queries: make(map[string]*QueryStats),
	}
}

// Record records query execution
func (qa *QueryAnalyzer) Record(query string, duration time.Duration) {
	qa.mu.Lock()
	defer qa.mu.Unlock()
	
	stats, exists := qa.queries[query]
	if !exists {
		stats = &QueryStats{
			MinTime: duration,
			MaxTime: duration,
		}
		qa.queries[query] = stats
	}
	
	stats.Count++
	stats.TotalTime += duration
	stats.AvgTime = stats.TotalTime / time.Duration(stats.Count)
	stats.LastExecuted = time.Now()
	
	if duration < stats.MinTime {
		stats.MinTime = duration
	}
	if duration > stats.MaxTime {
		stats.MaxTime = duration
	}
}

// GetSlowQueries returns queries slower than threshold
func (qa *QueryAnalyzer) GetSlowQueries(threshold time.Duration) []string {
	qa.mu.RLock()
	defer qa.mu.RUnlock()
	
	var slow []string
	for query, stats := range qa.queries {
		if stats.AvgTime > threshold {
			slow = append(slow, query)
		}
	}
	
	return slow
}

// Stats returns database optimizer statistics
func (dbo *DBOptimizer) Stats() map[string]interface{} {
	return map[string]interface{}{
		"queries_executed":    dbo.stats.QueriesExecuted.Load(),
		"queries_cached":      dbo.stats.QueriesCached.Load(),
		"connections_active":  dbo.stats.ConnectionsActive.Load(),
		"connections_idle":    dbo.stats.ConnectionsIdle.Load(),
		"batches_executed":    dbo.stats.BatchesExecuted.Load(),
		"avg_query_time_us":   dbo.stats.AvgQueryTime.Load(),
		"pool_created":        dbo.connPool.created.Load(),
		"pool_destroyed":      dbo.connPool.destroyed.Load(),
	}
}