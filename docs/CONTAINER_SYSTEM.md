# govc Container System Design

## Overview

The govc Container System integrates three complementary approaches to create a unified, memory-first container platform that leverages govc's existing memory-first repository infrastructure for complete container lifecycle management.

## Architecture Layers

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        govc Container Platform                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                           Layer 3: GitOps Controller                       │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────────┐  │
│  │   Kubernetes    │  │   Monitoring    │  │    Canary & Rollback        │  │
│  │   Integration   │  │   & Sync        │  │    Management               │  │
│  │                 │  │                 │  │                             │  │
│  │ • Deploy K8s    │  │ • Health Check  │  │ • Parallel Reality Testing  │  │
│  │ • Auto-sync     │  │ • Event Stream  │  │ • Traffic Splitting         │  │
│  │ • Drift Detect  │  │ • Metrics       │  │ • Automated Rollback        │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────────────────┘  │
├─────────────────────────────────────────────────────────────────────────────┤
│                        Layer 2: Container Registry                         │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────────┐  │
│  │   Image Store   │  │   Build Engine  │  │    Layer Management         │  │
│  │                 │  │                 │  │                             │  │
│  │ • Memory-first  │  │ • Govcfile      │  │ • Deduplication             │  │
│  │ • Compression   │  │ • Multi-stage   │  │ • Garbage Collection        │  │
│  │ • Encryption    │  │ • Parallel      │  │ • Content Addressing        │  │
│  │ • Replication   │  │ • Caching       │  │ • Vulnerability Scanning    │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────────────────┘  │
├─────────────────────────────────────────────────────────────────────────────┤
│                    Layer 1: Repository-Native Management                   │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────────┐  │
│  │  Definition     │  │   Versioning    │  │    Event-Driven Builds     │  │
│  │  Storage        │  │                 │  │                             │  │
│  │                 │  │ • Govcfile      │  │ • Commit Triggers           │  │
│  │ • Govcfiles     │  │ • Compose       │  │ • Branch Policies           │  │
│  │ • Compose Files │  │ • Manifests     │  │ • Hook System               │  │
│  │ • K8s Manifests │  │ • Configuration │  │ • Pipeline Automation       │  │
│  │ • Helm Charts   │  │ • Dependencies  │  │ • Notification System       │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                        ┌───────────▼────────────┐
                        │    govc Core Engine    │
                        │                        │
                        │ • Memory-First Storage │
                        │ • Parallel Realities   │
                        │ • Transaction System   │
                        │ • Event Streaming       │
                        │ • Repository Pooling    │
                        │ • Authentication/RBAC   │
                        │ • Metrics & Monitoring  │
                        └────────────────────────┘
```

## Core Components

### 1. Repository-Native Container Management

**Purpose**: Store and version all container-related files using govc's memory-first repository system.

**Components**:
- **Definition Storage**: Govcfiles, govc-compose.yml, Kubernetes manifests, Helm charts stored as repository files
- **Version Control**: Full commit history for all container definitions using govc's native versioning
- **Branch-Based Environments**: Different branches for dev/staging/prod configurations
- **Event-Driven Builds**: Automatic container builds triggered by govc commits

**Key Features**:
```go
type ContainerDefinition struct {
    Type        DefinitionType  // govcfile, govc-compose, k8s, helm
    Path        string         // Path in repository
    Content     []byte         // File content
    Metadata    Metadata       // Build instructions, labels
    Dependencies []string      // Related files
}

type DefinitionType string
const (
    TypeGovcfile DefinitionType = "govcfile"
    TypeCompose    DefinitionType = "compose"
    TypeKubernetes DefinitionType = "kubernetes"
    TypeHelm       DefinitionType = "helm"
)
```

### 2. Container Registry Integration

**Purpose**: Provide a complete container registry with memory-first performance and Git-like versioning.

**Components**:
- **Image Storage**: Memory-first storage with optional disk persistence
- **Layer Deduplication**: Content-addressable storage for efficient layer sharing
- **Build Engine**: Integrated container build system with Govcfile support
- **Security Scanning**: Vulnerability detection and policy enforcement

**Key Features**:
```go
type ContainerImage struct {
    Repository string        // e.g., "myapp"
    Tag        string        // e.g., "v1.0.0"
    Digest     string        // Content hash
    Layers     []ImageLayer  // Ordered layers
    Manifest   ImageManifest // OCI manifest
    Config     ImageConfig   // Runtime configuration
    CreatedAt  time.Time
    Size       int64
}

type ImageLayer struct {
    Digest      string    // SHA256 hash
    Size        int64     // Compressed size
    MediaType   string    // application/vnd.govc.image.rootfs.diff.tar.gzip
    URLs        []string  // Download URLs
    Annotations map[string]string
}

type BuildRequest struct {
    RepositoryID string            // govc repository ID
    Context      string            // Build context path
    Govcfile     string            // Govcfile path
    Tags         []string          // Image tags
    Args         map[string]string // Build arguments
    Labels       map[string]string // Image labels
    Target       string            // Multi-stage target
    Platform     []string          // Target platforms
}
```

### 3. Kubernetes GitOps Controller

**Purpose**: Automatically deploy and manage Kubernetes resources based on Git commits.

**Components**:
- **Sync Engine**: Monitors Git repositories for changes and applies them to clusters
- **Drift Detection**: Identifies when cluster state differs from Git state
- **Rollback System**: Automatic rollbacks on deployment failures
- **Canary Deployments**: Progressive rollouts using parallel realities

**Key Features**:
```go
type GitOpsApplication struct {
    Name           string            // Application name
    RepositoryID   string            // govc repository ID
    Path           string            // Path to manifests
    Branch         string            // Target branch
    Namespace      string            // K8s namespace
    Cluster        string            // Target cluster
    SyncPolicy     SyncPolicy        // Auto-sync configuration
    HealthStatus   HealthStatus      // Application health
    LastSync       time.Time         // Last successful sync
    Annotations    map[string]string // User annotations
}

type SyncPolicy struct {
    Automated   bool          // Enable auto-sync
    SelfHeal    bool          // Auto-correct drift
    Prune       bool          // Remove orphaned resources
    SyncOptions []SyncOption  // Additional sync options
}

type CanaryDeployment struct {
    ApplicationID string        // GitOps application
    RealityName   string        // Parallel reality name
    TrafficSplit  TrafficSplit  // Traffic distribution
    SuccessRate   float64       // Success threshold
    Duration      time.Duration // Canary duration
    Status        CanaryStatus  // Current status
}
```

## API Design

### Container Definition Management

```http
# Store container definitions
POST /api/v1/repos/{repo_id}/containers/definitions
PUT  /api/v1/repos/{repo_id}/containers/definitions/{path}
GET  /api/v1/repos/{repo_id}/containers/definitions
GET  /api/v1/repos/{repo_id}/containers/definitions/{path}
DELETE /api/v1/repos/{repo_id}/containers/definitions/{path}

# Build triggers
POST /api/v1/repos/{repo_id}/containers/build
GET  /api/v1/repos/{repo_id}/containers/builds
GET  /api/v1/repos/{repo_id}/containers/builds/{build_id}
DELETE /api/v1/repos/{repo_id}/containers/builds/{build_id}
```

### Container Registry Operations

```http
# Image management
GET    /v2/{repository}/manifests/{tag}
PUT    /v2/{repository}/manifests/{tag}
DELETE /v2/{repository}/manifests/{tag}
GET    /v2/{repository}/blobs/{digest}
PUT    /v2/{repository}/blobs/uploads/
PATCH  /v2/{repository}/blobs/uploads/{uuid}
PUT    /v2/{repository}/blobs/uploads/{uuid}

# govc-specific registry endpoints
GET  /api/v1/registry/images
POST /api/v1/registry/images/{repository}/build
GET  /api/v1/registry/images/{repository}/tags
GET  /api/v1/registry/images/{repository}/history
POST /api/v1/registry/images/{repository}/scan
GET  /api/v1/registry/stats
```

### GitOps Management

```http
# Application management
POST   /api/v1/gitops/applications
GET    /api/v1/gitops/applications
GET    /api/v1/gitops/applications/{app_id}
PUT    /api/v1/gitops/applications/{app_id}
DELETE /api/v1/gitops/applications/{app_id}

# Sync operations
POST /api/v1/gitops/applications/{app_id}/sync
POST /api/v1/gitops/applications/{app_id}/rollback
GET  /api/v1/gitops/applications/{app_id}/history
GET  /api/v1/gitops/applications/{app_id}/diff

# Canary deployments
POST /api/v1/gitops/applications/{app_id}/canary
GET  /api/v1/gitops/applications/{app_id}/canary/{canary_id}
POST /api/v1/gitops/applications/{app_id}/canary/{canary_id}/promote
POST /api/v1/gitops/applications/{app_id}/canary/{canary_id}/abort
```

## Integration with govc Core Features

### Parallel Realities for Container Testing

```go
// Create parallel realities for testing different container configurations
realities := repo.ParallelRealities([]string{"canary", "blue", "green"})

// Test canary deployment
canary := realities[0]
canary.Apply(map[string][]byte{
    "k8s/deployment.yaml": newDeploymentConfig,
})

// Build and deploy in canary reality
buildResult := containerSystem.Build(canary, "Govcfile")
deployResult := gitopsController.Deploy(canary, "production")

// Monitor canary metrics
if canary.Benchmark().SuccessRate > 0.99 {
    repo.Merge("canary", "main")
    gitopsController.PromoteCanary(deployResult.ID)
}
```

### Event-Driven Pipeline

```go
// Watch for container-related commits
repo.Watch(func(event govc.CommitEvent) {
    if event.Changes("Govcfile") || event.Changes("*.govcfile") {
        // Trigger container build
        buildID := containerSystem.TriggerBuild(BuildRequest{
            RepositoryID: event.RepositoryID,
            Context:      path.Dir(event.ChangedFiles[0]),
            Govcfile:     event.ChangedFiles[0],
        })
        
        // Notify build started
        event.Respond(BuildStartedEvent{BuildID: buildID})
    }
    
    if event.Changes("k8s/*.yaml") || event.Changes("*.k8s.yaml") {
        // Trigger GitOps sync
        syncID := gitopsController.TriggerSync(SyncRequest{
            RepositoryID: event.RepositoryID,
            Path:         "k8s/",
            Branch:       event.Branch,
        })
        
        // Notify sync started
        event.Respond(SyncStartedEvent{SyncID: syncID})
    }
})
```

### Transactional Container Operations

```go
// Atomic container definition updates
tx := repo.Transaction()
tx.Add("Govcfile", newGovcfileContent)
tx.Add("govc-compose.yml", newComposeContent)
tx.Add("k8s/deployment.yaml", newK8sManifest)

// Validate before commit
if err := containerSystem.ValidateDefinitions(tx); err != nil {
    tx.Rollback()
    return err
}

// Commit triggers build pipeline
commitID := tx.Commit("Update container stack")
```

## Security Model

### Authentication & Authorization

```go
// Extend existing RBAC with container permissions
const (
    PermissionContainerBuild  = "container:build"
    PermissionContainerPush   = "container:push"
    PermissionContainerPull   = "container:pull"
    PermissionContainerDeploy = "container:deploy"
    PermissionRegistryRead    = "registry:read"
    PermissionRegistryWrite   = "registry:write"
    PermissionGitOpsRead      = "gitops:read"
    PermissionGitOpsWrite     = "gitops:write"
)

// Repository-level permissions
rbac.GrantRepositoryPermission("dev-team", "myapp", PermissionContainerBuild)
rbac.GrantRepositoryPermission("ops-team", "myapp", PermissionContainerDeploy)
```

### Image Security

```go
type SecurityPolicy struct {
    AllowedBaseImages    []string          // Approved base images
    ProhibitedPackages   []string          // Banned packages
    VulnerabilityPolicy  VulnerabilityPolicy // Vulnerability handling
    SigningRequired      bool              // Require image signing
    ScanRequired         bool              // Require security scan
    MaxImageSize         int64             // Maximum image size
}

type VulnerabilityPolicy struct {
    MaxCritical int // Maximum critical vulnerabilities
    MaxHigh     int // Maximum high vulnerabilities
    FailOnNew   bool // Fail build on new vulnerabilities
}
```

## Storage Architecture

### Memory-First Image Storage

```go
type ImageStore struct {
    layers    map[string]*ImageLayer    // Layer cache
    manifests map[string]*ImageManifest // Manifest cache
    lru       *LRUCache                 // Eviction policy
    disk      *DiskBackend              // Persistent storage
    maxMemory int64                     // Memory limit
}

// Intelligent caching based on access patterns
func (is *ImageStore) Get(digest string) (*ImageLayer, error) {
    // Try memory first
    if layer, ok := is.layers[digest]; ok {
        is.lru.Use(digest)
        return layer, nil
    }
    
    // Load from disk and cache
    layer, err := is.disk.Load(digest)
    if err == nil {
        is.cacheLayer(digest, layer)
    }
    return layer, err
}
```

### Layer Deduplication

```go
type LayerManager struct {
    store      *ImageStore
    index      map[string][]string // Content hash -> layer digests
    references map[string]int      // Reference counting
}

func (lm *LayerManager) StoreLayer(content []byte) (string, error) {
    contentHash := sha256.Sum256(content)
    digest := "sha256:" + hex.EncodeToString(contentHash[:])
    
    // Check for existing layer with same content
    if existing := lm.index[string(contentHash[:])]; len(existing) > 0 {
        lm.references[existing[0]]++
        return existing[0], nil
    }
    
    // Store new layer
    return lm.store.StoreLayer(digest, content)
}
```

## Monitoring & Observability

### Container Metrics

```go
// Extend existing Prometheus metrics
type ContainerMetrics struct {
    BuildsTotal        *prometheus.CounterVec   // Total builds
    BuildDuration      *prometheus.HistogramVec // Build time
    ImagesTotal        prometheus.Gauge         // Total images
    LayersTotal        prometheus.Gauge         // Total layers
    StorageUsage       prometheus.Gauge         // Storage usage
    DeploymentsTotal   *prometheus.CounterVec   // Total deployments
    DeploymentDuration *prometheus.HistogramVec // Deployment time
    SyncDuration       *prometheus.HistogramVec // GitOps sync time
}
```

### Event Logging

```go
type ContainerEvent struct {
    Type        EventType         `json:"type"`
    RepositoryID string           `json:"repository_id"`
    Timestamp   time.Time         `json:"timestamp"`
    Actor       string            `json:"actor"`
    Resource    ResourceRef       `json:"resource"`
    Data        map[string]interface{} `json:"data"`
}

const (
    EventTypeBuildStarted   EventType = "build.started"
    EventTypeBuildCompleted EventType = "build.completed"
    EventTypeBuildFailed    EventType = "build.failed"
    EventTypeImagePushed    EventType = "image.pushed"
    EventTypeImagePulled    EventType = "image.pulled"
    EventTypeDeployStarted  EventType = "deploy.started"
    EventTypeDeployCompleted EventType = "deploy.completed"
    EventTypeSyncStarted    EventType = "sync.started"
    EventTypeSyncCompleted  EventType = "sync.completed"
)
```

## Implementation Phases

### Phase 1: Repository-Native Foundation (2-3 weeks)
- [ ] Container definition storage and versioning
- [ ] Basic build trigger system
- [ ] API endpoints for definition management
- [ ] Integration with existing govc event system

### Phase 2: Container Registry (3-4 weeks)
- [ ] OCI-compliant registry implementation
- [ ] Memory-first image storage
- [ ] Layer deduplication and caching
- [ ] Build engine integration
- [ ] Security scanning framework

### Phase 3: GitOps Controller (2-3 weeks)
- [ ] Kubernetes client integration
- [ ] Sync engine implementation
- [ ] Drift detection and remediation
- [ ] Application health monitoring

### Phase 4: Advanced Features (2-3 weeks)
- [ ] Canary deployments with parallel realities
- [ ] Multi-cluster support
- [ ] Advanced security policies
- [ ] Performance optimization

### Phase 5: Production Readiness (1-2 weeks)
- [ ] Comprehensive testing
- [ ] Documentation
- [ ] Performance benchmarking
- [ ] Security audit

## Configuration Example

```yaml
# config.yaml
container_system:
  enabled: true
  
  # Repository-native management
  definitions:
    auto_build: true
    build_timeout: "10m"
    parallel_builds: 5
    
  # Container registry
  registry:
    storage:
      backend: "memory-first"  # memory-first, disk, s3
      max_memory: "2GB"
      disk_path: "/var/lib/govc/registry"
    
    security:
      scan_enabled: true
      vulnerability_policy:
        max_critical: 0
        max_high: 5
      signing_required: false
      
  # GitOps controller  
  gitops:
    clusters:
      - name: "production"
        kubeconfig: "/etc/kubeconfig/prod"
        namespace: "default"
      - name: "staging"  
        kubeconfig: "/etc/kubeconfig/staging"
        namespace: "staging"
    
    sync:
      interval: "30s"
      timeout: "5m"
      auto_sync: true
      self_heal: true
      prune: true
```

## Migration Path

For existing govc installations:

1. **Backward Compatibility**: All existing features remain unchanged
2. **Opt-in Activation**: Container system is disabled by default
3. **Gradual Migration**: Teams can migrate container definitions incrementally
4. **Data Import**: Tools to import existing Govcfiles, Dockerfiles (legacy), and manifests into govc repositories

This design leverages govc's existing strengths (memory-first operations, parallel realities, transactional commits) while adding comprehensive container management capabilities that work seamlessly together without any external Git dependencies.