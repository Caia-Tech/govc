# govc Multi-Database Support Implementation Plan

## Overview

This document outlines the comprehensive plan for implementing multi-database support in govc, enabling users to choose the best storage backend for their specific use cases while maintaining a consistent API.

## Architecture

### Core Design Principles

1. **Adapter Pattern**: Each database implementation follows a common interface
2. **Factory Pattern**: Centralized database instantiation based on configuration
3. **Decorator Pattern**: Layer caching, compression, and encryption
4. **Strategy Pattern**: Different stores for different data types (objects vs metadata)

### Interface Hierarchy

```
DataStore (main interface)
├── ObjectStore (Git objects)
├── MetadataStore (repos, users, config)
├── TransactionStore (ACID operations)
└── QueryStore (complex queries)
```

## Supported Databases

### Tier 1 - Essential (Must Have)

| Database | Type | Purpose | Status |
|----------|------|---------|--------|
| **Memory** | In-Memory | Development, Testing | ✅ Exists (needs refactor) |
| **SQLite** | Embedded SQL | Single-user, lightweight | 🔄 Pending |
| **BadgerDB** | Embedded KV | High-performance Git objects | 🔄 Pending |
| **PostgreSQL** | Network SQL | Enterprise, multi-user | 🔄 Pending |

### Tier 2 - Important (Should Have)

| Database | Type | Purpose | Status |
|----------|------|---------|--------|
| **MongoDB** | Network NoSQL | Scalable, document-based | 🔄 Pending |
| **Redis** | Network Cache | Caching, sessions, pub/sub | 🔄 Pending |
| **Custom LSM** | Embedded | Git-optimized, learning | 🔄 Pending |

### Tier 3 - Optional (Community Driven)

| Database | Type | Purpose | Status |
|----------|------|---------|--------|
| **BoltDB** | Embedded KV | Alternative to BadgerDB | 🔄 Future |
| **CockroachDB** | Distributed SQL | Global scale | 🔄 Future |
| **ScyllaDB** | Distributed NoSQL | Ultra-high performance | 🔄 Future |

## Implementation Phases

### Phase 1: Foundation (Week 1)

#### 1.1 Core Interfaces

```go
// datastore/interface.go
type DataStore interface {
    // Lifecycle
    Initialize(config Config) error
    Close() error
    HealthCheck() error
    
    // Object operations
    ObjectStore
    
    // Metadata operations
    MetadataStore
    
    // Transaction support
    TransactionStore
    
    // Metrics
    GetMetrics() Metrics
}

type ObjectStore interface {
    GetObject(hash string) ([]byte, error)
    PutObject(hash string, data []byte) error
    DeleteObject(hash string) error
    HasObject(hash string) (bool, error)
    ListObjects(prefix string, limit int) ([]string, error)
    
    // Batch operations
    GetObjects(hashes []string) (map[string][]byte, error)
    PutObjects(objects map[string][]byte) error
}

type MetadataStore interface {
    // Repository management
    SaveRepository(repo *Repository) error
    GetRepository(id string) (*Repository, error)
    ListRepositories(filter RepositoryFilter) ([]*Repository, error)
    DeleteRepository(id string) error
    
    // User management
    SaveUser(user *User) error
    GetUser(id string) (*User, error)
    ListUsers(filter UserFilter) ([]*User, error)
    
    // Audit logging
    LogEvent(event *AuditEvent) error
    QueryEvents(filter EventFilter) ([]*AuditEvent, error)
    
    // Configuration
    GetConfig(key string) (string, error)
    SetConfig(key string, value string) error
}

type TransactionStore interface {
    BeginTx(ctx context.Context, opts *TxOptions) (Transaction, error)
}

type Transaction interface {
    Commit() error
    Rollback() error
    
    // All ObjectStore methods
    ObjectStore
    
    // All MetadataStore methods
    MetadataStore
}
```

#### 1.2 Configuration System

```go
// datastore/config.go
type Config struct {
    Type        string                 `yaml:"type"`
    ConnString  string                 `yaml:"connection"`
    Options     map[string]interface{} `yaml:"options"`
    
    // Advanced configurations
    Sharding    *ShardingConfig        `yaml:"sharding"`
    Replication *ReplicationConfig     `yaml:"replication"`
    Cache       *CacheConfig           `yaml:"cache"`
}

type StoreRegistry struct {
    factories map[string]Factory
}

func (r *StoreRegistry) Register(name string, factory Factory) {
    r.factories[name] = factory
}

func (r *StoreRegistry) Create(config Config) (DataStore, error) {
    factory, exists := r.factories[config.Type]
    if !exists {
        return nil, fmt.Errorf("unknown datastore type: %s", config.Type)
    }
    return factory.Create(config)
}
```

### Phase 2: Memory Store Refactor (Week 1)

#### 2.1 Refactor Existing Implementation

```go
// datastore/memory/memory.go
type MemoryStore struct {
    mu          sync.RWMutex
    objects     map[string][]byte
    repos       map[string]*Repository
    users       map[string]*User
    events      []AuditEvent
    config      map[string]string
    
    // Metrics
    reads       atomic.Uint64
    writes      atomic.Uint64
    deletes     atomic.Uint64
}

// Features:
// - Thread-safe operations
// - Optional persistence to disk
// - Configurable size limits
// - LRU eviction policy
```

### Phase 3: SQLite Implementation (Week 1-2)

#### 3.1 Schema Design

```sql
-- repositories table
CREATE TABLE repositories (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    metadata JSONB
);

-- objects table (for small objects, large ones go to filesystem)
CREATE TABLE objects (
    hash TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    size INTEGER NOT NULL,
    data BLOB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- refs table
CREATE TABLE refs (
    repository_id TEXT NOT NULL,
    name TEXT NOT NULL,
    hash TEXT NOT NULL,
    type TEXT NOT NULL,
    PRIMARY KEY (repository_id, name),
    FOREIGN KEY (repository_id) REFERENCES repositories(id)
);

-- users table
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    email TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    metadata JSONB
);

-- audit_events table
CREATE TABLE audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id TEXT,
    action TEXT NOT NULL,
    resource TEXT,
    details JSONB,
    INDEX idx_timestamp (timestamp),
    INDEX idx_user_id (user_id)
);
```

### Phase 4: BadgerDB Implementation (Week 2)

#### 4.1 Key Design

```go
// datastore/badger/badger.go
type BadgerStore struct {
    db *badger.DB
    
    // Key prefixes for namespacing
    prefixes struct {
        objects  []byte // "o:"
        refs     []byte // "r:"
        repos    []byte // "repo:"
        users    []byte // "u:"
        events   []byte // "e:"
    }
}

// Key schema:
// Objects: o:<hash>
// Refs: r:<repo_id>:<ref_name>
// Repos: repo:<repo_id>
// Users: u:<user_id>
// Events: e:<timestamp>:<uuid>

// Optimizations:
// - Bloom filters for existence checks
// - Compression for large objects
// - Batch writes for performance
// - Stream API for large scans
```

### Phase 5: PostgreSQL Implementation (Week 2-3)

#### 5.1 Schema Design

```sql
-- Use JSONB for flexibility with indexes for performance

-- repositories table
CREATE TABLE repositories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    path TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_repo_name ON repositories(name);
CREATE INDEX idx_repo_metadata ON repositories USING GIN (metadata);

-- objects table with partitioning for scale
CREATE TABLE objects (
    hash TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    size BIGINT NOT NULL,
    data BYTEA,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
) PARTITION BY HASH (hash);

-- Create partitions
CREATE TABLE objects_0 PARTITION OF objects FOR VALUES WITH (modulus 4, remainder 0);
CREATE TABLE objects_1 PARTITION OF objects FOR VALUES WITH (modulus 4, remainder 1);
CREATE TABLE objects_2 PARTITION OF objects FOR VALUES WITH (modulus 4, remainder 2);
CREATE TABLE objects_3 PARTITION OF objects FOR VALUES WITH (modulus 4, remainder 3);

-- commits table for efficient traversal
CREATE TABLE commits (
    hash TEXT PRIMARY KEY,
    tree_hash TEXT NOT NULL,
    parent_hashes TEXT[],
    author JSONB NOT NULL,
    committer JSONB NOT NULL,
    message TEXT,
    created_at TIMESTAMPTZ,
    metadata JSONB DEFAULT '{}'
);

CREATE INDEX idx_commit_parents ON commits USING GIN (parent_hashes);
CREATE INDEX idx_commit_created ON commits(created_at);

-- refs table
CREATE TABLE refs (
    repository_id UUID NOT NULL,
    name TEXT NOT NULL,
    hash TEXT NOT NULL,
    type TEXT NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (repository_id, name),
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
);
```

#### 5.2 Advanced Features

```go
// PostgreSQL-specific optimizations
type PostgresStore struct {
    db *sql.DB
    
    // Prepared statements cache
    stmts map[string]*sql.Stmt
    
    // Connection pool configuration
    maxConns int
    
    // Features
    useCopyFrom bool  // Bulk inserts
    useNotify   bool  // Real-time events
}

// Leverage PostgreSQL features:
// - CTEs for complex queries
// - LISTEN/NOTIFY for real-time updates
// - Advisory locks for distributed coordination
// - Materialized views for analytics
```

### Phase 6: MongoDB Implementation (Week 3)

#### 6.1 Collection Design

```javascript
// objects collection
{
    _id: "sha1_hash",
    type: "commit|tree|blob|tag",
    size: 1234,
    data: BinData(...),
    metadata: {
        compressed: true,
        encryption: "aes256"
    },
    created_at: ISODate("2024-01-01")
}

// repositories collection
{
    _id: "repo_id",
    name: "my-repo",
    refs: {
        "refs/heads/main": "commit_sha",
        "refs/heads/feature": "commit_sha"
    },
    metadata: {
        description: "...",
        visibility: "private",
        settings: {}
    },
    stats: {
        commits: 1000,
        branches: 10,
        contributors: 5
    },
    created_at: ISODate("2024-01-01")
}

// commits collection (denormalized for performance)
{
    _id: "commit_sha",
    tree: "tree_sha",
    parents: ["parent_sha1", "parent_sha2"],
    author: {
        name: "John Doe",
        email: "john@example.com",
        timestamp: ISODate("2024-01-01")
    },
    message: "Commit message",
    // Denormalized data for fast queries
    files_changed: ["path/to/file1", "path/to/file2"],
    stats: {
        additions: 100,
        deletions: 50
    }
}
```

#### 6.2 MongoDB Features

```go
// datastore/mongodb/mongodb.go
type MongoStore struct {
    client   *mongo.Client
    database *mongo.Database
    
    // Collections
    objects  *mongo.Collection
    repos    *mongo.Collection
    commits  *mongo.Collection
    users    *mongo.Collection
    
    // Features
    useGridFS   bool  // For large objects
    useStreams  bool  // Change streams for real-time
    useSharding bool  // For horizontal scaling
}

// MongoDB-specific features:
// - Aggregation pipelines for analytics
// - Change streams for real-time updates
// - GridFS for large binary objects
// - Sharding for horizontal scale
// - Text search indexes
```

### Phase 7: Redis Implementation (Week 3-4)

#### 7.1 Key Design

```go
// datastore/redis/redis.go
type RedisStore struct {
    client *redis.Client
    
    // Key patterns
    patterns struct {
        object   string // "obj:{hash}"
        repo     string // "repo:{id}"
        user     string // "user:{id}"
        session  string // "session:{token}"
        cache    string // "cache:{key}"
    }
    
    // TTL configurations
    ttls struct {
        session time.Duration
        cache   time.Duration
    }
}

// Redis-specific features:
// - Pub/Sub for real-time events
// - Sorted sets for leaderboards
// - HyperLogLog for cardinality
// - Streams for event sourcing
// - Lua scripts for atomic operations
```

### Phase 8: Custom LSM Implementation (Week 4)

#### 8.1 Design

```go
// datastore/custom/lsm.go
type LSMTree struct {
    // In-memory components
    memtable    *MemTable
    immutables  []*MemTable
    
    // On-disk components
    levels      [][]*SSTable
    
    // Write-ahead log
    wal         *WAL
    
    // Background workers
    compactor   *Compactor
    
    // Metadata
    manifest    *Manifest
    
    // Optimizations
    bloomFilters map[string]*BloomFilter
    blockCache   *LRUCache
}

// Git-specific optimizations:
// - Delta compression for similar objects
// - Pack files for space efficiency
// - Thin packs for network transfer
// - Bitmap indexes for reachability
```

## Configuration Examples

### Development Configuration

```yaml
# Simple in-memory setup
datastore:
  type: memory
  options:
    max_size: 1GB
    persist_on_shutdown: false
```

### Small Team Configuration

```yaml
# SQLite for simplicity
datastore:
  type: sqlite
  connection: ./data/govc.db
  options:
    journal_mode: WAL
    synchronous: NORMAL
    cache_size: 10000
```

### Production Configuration

```yaml
# Hybrid setup with different stores
datastore:
  # Git objects in BadgerDB
  objects:
    type: badger
    connection: /var/lib/govc/objects
    options:
      compression: snappy
      encryption: aes256
      value_log_file_size: 1GB
      
  # Metadata in PostgreSQL
  metadata:
    type: postgres
    connection: postgres://govc:password@db.example.com/govc
    options:
      pool_size: 25
      ssl_mode: require
      
  # Cache in Redis
  cache:
    type: redis
    connection: redis://cache.example.com:6379
    options:
      max_retries: 3
      pool_size: 10
```

### Enterprise Configuration

```yaml
# MongoDB for scale
datastore:
  type: mongodb
  connection: mongodb://cluster.example.com/govc
  options:
    replica_set: rs0
    write_concern: majority
    read_preference: secondaryPreferred
    use_gridfs: true
    
  # Redis cache layer
  cache:
    type: redis
    connection: redis://cache.example.com:6379
    options:
      cluster: true
      read_timeout: 100ms
      write_timeout: 100ms
```

## Migration Support

### Migration Tools

```go
// datastore/migration/migrator.go
type Migrator struct {
    source DataStore
    target DataStore
    
    // Options
    batchSize   int
    parallel    int
    verify      bool
    resume      bool
}

// Migration strategies:
// 1. Offline migration (full dump and restore)
// 2. Online migration (dual writes)
// 3. Incremental migration (change data capture)
// 4. Lazy migration (on-demand)
```

### Migration Commands

```bash
# Export from one store
govc datastore export --source sqlite://old.db --output backup.tar.gz

# Import to another store
govc datastore import --target postgres://new.db --input backup.tar.gz

# Live migration
govc datastore migrate --source sqlite://old.db --target badger://new.db --live

# Verify migration
govc datastore verify --source sqlite://old.db --target badger://new.db
```

## Testing Strategy

### Compliance Test Suite

Every datastore implementation must pass:

```go
// datastore/tests/compliance.go
type ComplianceTestSuite struct {
    // Basic operations
    TestBasicCRUD()
    TestBatchOperations()
    TestTransactions()
    
    // Concurrency
    TestConcurrentWrites()
    TestConcurrentReads()
    TestRaceConditions()
    
    // Performance
    TestLargeObjects()
    TestManySmallObjects()
    TestQueryPerformance()
    
    // Reliability
    TestCrashRecovery()
    TestDataIntegrity()
    TestBackupRestore()
}
```

### Performance Benchmarks

```go
// datastore/tests/benchmark.go
type BenchmarkSuite struct {
    // Object operations
    BenchmarkObjectWrite()
    BenchmarkObjectRead()
    BenchmarkObjectScan()
    
    // Metadata operations
    BenchmarkMetadataQuery()
    BenchmarkComplexJoins()
    
    // Concurrent operations
    BenchmarkParallelWrites()
    BenchmarkParallelReads()
    
    // Git-specific
    BenchmarkCommitTraversal()
    BenchmarkBranchOperations()
    BenchmarkDiffGeneration()
}
```

## Performance Requirements

### Minimum Performance Targets

| Operation | Target | Notes |
|-----------|--------|-------|
| Object Write | < 1ms | For objects < 1MB |
| Object Read | < 0.5ms | From cache |
| Object Read | < 5ms | From disk |
| Metadata Query | < 10ms | Simple queries |
| Complex Query | < 100ms | Joins, aggregations |
| Transaction | < 50ms | Full ACID transaction |
| Batch Write (1000) | < 100ms | 1000 small objects |

### Scalability Targets

- Support 1M+ objects per repository
- Handle 10K+ concurrent connections
- Scale to 1000+ repositories
- Process 100K+ operations/second

## Implementation Timeline

### Week 1
- [ ] Design and implement core interfaces
- [ ] Refactor memory store to new interface
- [ ] Create factory and configuration system
- [ ] Set up compliance test suite

### Week 2
- [ ] Implement SQLite adapter
- [ ] Implement BadgerDB adapter
- [ ] Begin PostgreSQL adapter
- [ ] Create migration framework

### Week 3
- [ ] Complete PostgreSQL adapter
- [ ] Implement MongoDB adapter
- [ ] Begin Redis adapter
- [ ] Performance benchmarking

### Week 4
- [ ] Complete Redis adapter
- [ ] Begin custom LSM implementation
- [ ] Integration testing
- [ ] Documentation

### Week 5
- [ ] Complete custom LSM implementation
- [ ] Performance optimization
- [ ] Production testing
- [ ] Final documentation

## Success Criteria

1. **Compatibility**: All stores pass compliance tests
2. **Performance**: Meet or exceed performance targets
3. **Migration**: Seamless migration between any stores
4. **Documentation**: Complete user and developer docs
5. **Production Ready**: At least 4 stores production ready

## Directory Structure

```
datastore/
├── interface.go           # Core interfaces
├── factory.go            # Factory pattern
├── config.go             # Configuration types
├── metrics.go            # Metrics collection
├── memory/
│   ├── memory.go         # Memory implementation
│   └── memory_test.go
├── sqlite/
│   ├── sqlite.go         # SQLite implementation
│   ├── migrations/       # SQL migrations
│   └── sqlite_test.go
├── badger/
│   ├── badger.go         # BadgerDB implementation
│   └── badger_test.go
├── postgres/
│   ├── postgres.go       # PostgreSQL implementation
│   ├── migrations/       # SQL migrations
│   └── postgres_test.go
├── mongodb/
│   ├── mongodb.go        # MongoDB implementation
│   └── mongodb_test.go
├── redis/
│   ├── redis.go          # Redis implementation
│   └── redis_test.go
├── custom/
│   ├── lsm.go           # Custom LSM tree
│   ├── wal.go           # Write-ahead log
│   ├── compaction.go    # Compaction logic
│   └── custom_test.go
├── migration/
│   ├── migrator.go      # Migration tools
│   ├── export.go        # Export logic
│   └── import.go        # Import logic
└── tests/
    ├── compliance.go     # Compliance tests
    ├── benchmark.go      # Benchmarks
    └── integration.go    # Integration tests
```

## Next Steps

1. Review and approve this plan
2. Set up the directory structure
3. Begin with Phase 1: Core Interfaces
4. Implement stores in priority order
5. Continuous testing and benchmarking

## References

- [BadgerDB Documentation](https://dgraph.io/docs/badger)
- [PostgreSQL JSONB](https://www.postgresql.org/docs/current/datatype-json.html)
- [MongoDB Go Driver](https://docs.mongodb.com/drivers/go)
- [Redis Go Client](https://redis.uptrace.dev)
- [SQLite Go Driver](https://github.com/mattn/go-sqlite3)
- [LSM Tree Design](https://en.wikipedia.org/wiki/Log-structured_merge-tree)

---

*This document is a living guide and will be updated as implementation progresses.*