package container

import (
	"time"
)

// DefinitionType represents the type of container definition
type DefinitionType string

const (
	// TypeGovcfile represents a Govcfile (govc-native container definition)
	TypeGovcfile DefinitionType = "govcfile"
	// TypeGovcCompose represents a govc compose file for multi-container apps
	TypeGovcCompose DefinitionType = "govc-compose"
	// TypeKubernetes represents Kubernetes manifests
	TypeKubernetes DefinitionType = "kubernetes"
	// TypeHelm represents Helm charts
	TypeHelm DefinitionType = "helm"
)

// ContainerDefinition represents a container-related file in a repository
type ContainerDefinition struct {
	Type         DefinitionType         `json:"type"`
	Path         string                 `json:"path"`
	Content      []byte                 `json:"content"`
	Metadata     map[string]interface{} `json:"metadata"`
	Dependencies []string               `json:"dependencies"`
	CommitHash   string                 `json:"commit_hash"`
	LastModified time.Time              `json:"last_modified"`
}

// BuildRequest represents a request to build a container image
type BuildRequest struct {
	RepositoryID    string            `json:"repository_id"`
	Context         string            `json:"context"`
	Govcfile        string            `json:"govcfile"`        // Path to Govcfile (was Dockerfile)
	Containerfile   string            `json:"containerfile"`   // Deprecated: use Govcfile
	Tags            []string          `json:"tags"`
	Args            map[string]string `json:"args"`
	Labels          map[string]string `json:"labels"`
	Target          string            `json:"target"`
	Platform        []string          `json:"platform"`
	NoCache         bool              `json:"no_cache"`
}

// BuildStatus represents the status of a build
type BuildStatus string

const (
	BuildStatusPending   BuildStatus = "pending"
	BuildStatusRunning   BuildStatus = "running"
	BuildStatusCompleted BuildStatus = "completed"
	BuildStatusFailed    BuildStatus = "failed"
	BuildStatusCancelled BuildStatus = "cancelled"
)

// Build represents a container build operation
type Build struct {
	ID           string            `json:"id"`
	RepositoryID string            `json:"repository_id"`
	Request      BuildRequest      `json:"request"`
	Status       BuildStatus       `json:"status"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      *time.Time        `json:"end_time"`
	Output       string            `json:"output"`
	Error        string            `json:"error"`
	ImageID      string            `json:"image_id"`
	Digest       string            `json:"digest"`
	Metadata     map[string]string `json:"metadata"`
}

// ContainerEvent represents an event in the container system
type ContainerEvent struct {
	Type         EventType              `json:"type"`
	RepositoryID string                 `json:"repository_id"`
	Timestamp    time.Time              `json:"timestamp"`
	Actor        string                 `json:"actor"`
	Resource     interface{}            `json:"resource"`
	Data         map[string]interface{} `json:"data"`
}

// EventType represents the type of container event
type EventType string

const (
	EventTypeBuildStarted    EventType = "build.started"
	EventTypeBuildCompleted  EventType = "build.completed"
	EventTypeBuildFailed     EventType = "build.failed"
	EventTypeImagePushed     EventType = "image.pushed"
	EventTypeImagePulled     EventType = "image.pulled"
	EventTypeDeployStarted   EventType = "deploy.started"
	EventTypeDeployCompleted EventType = "deploy.completed"
	EventTypeSyncStarted     EventType = "sync.started"
	EventTypeSyncCompleted   EventType = "sync.completed"
)

// SecurityPolicy defines security constraints for container operations
type SecurityPolicy struct {
	AllowedBaseImages    []string            `json:"allowed_base_images"`
	ProhibitedPackages   []string            `json:"prohibited_packages"`
	VulnerabilityPolicy  VulnerabilityPolicy `json:"vulnerability_policy"`
	SigningRequired      bool                `json:"signing_required"`
	ScanRequired         bool                `json:"scan_required"`
	MaxImageSize         int64               `json:"max_image_size"`
}

// VulnerabilityPolicy defines vulnerability handling rules
type VulnerabilityPolicy struct {
	MaxCritical int  `json:"max_critical"`
	MaxHigh     int  `json:"max_high"`
	MaxMedium   int  `json:"max_medium"`
	MaxLow      int  `json:"max_low"`
	FailOnNew   bool `json:"fail_on_new"`
}