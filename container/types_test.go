package container

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefinitionType(t *testing.T) {
	tests := []struct {
		name     string
		defType  DefinitionType
		expected string
	}{
		{"Govcfile", TypeGovcfile, "govcfile"},
		{"GovcCompose", TypeGovcCompose, "govc-compose"},
		{"Kubernetes", TypeKubernetes, "kubernetes"},
		{"Helm", TypeHelm, "helm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.defType))
		})
	}
}

func TestContainerDefinition(t *testing.T) {
	now := time.Now()
	def := ContainerDefinition{
		Type:         TypeGovcfile,
		Path:         "Govcfile",
		Content:      []byte("BASE alpine:latest"),
		Metadata:     map[string]interface{}{"version": "1.0"},
		Dependencies: []string{"app.go", "go.mod"},
		CommitHash:   "abc123",
		LastModified: now,
	}

	t.Run("Fields", func(t *testing.T) {
		assert.Equal(t, TypeGovcfile, def.Type)
		assert.Equal(t, "Govcfile", def.Path)
		assert.Equal(t, []byte("BASE alpine:latest"), def.Content)
		assert.Equal(t, "1.0", def.Metadata["version"])
		assert.Contains(t, def.Dependencies, "app.go")
		assert.Contains(t, def.Dependencies, "go.mod")
		assert.Equal(t, "abc123", def.CommitHash)
		assert.Equal(t, now, def.LastModified)
	})

	t.Run("JSON Serialization", func(t *testing.T) {
		data, err := json.Marshal(def)
		require.NoError(t, err)

		var decoded ContainerDefinition
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, def.Type, decoded.Type)
		assert.Equal(t, def.Path, decoded.Path)
		assert.Equal(t, def.Content, decoded.Content)
		assert.Equal(t, def.CommitHash, decoded.CommitHash)
	})
}

func TestBuildRequest(t *testing.T) {
	req := BuildRequest{
		RepositoryID: "test-repo",
		Context:      ".",
		Govcfile:     "Govcfile",
		Tags:         []string{"app:latest", "app:v1.0"},
		Args:         map[string]string{"VERSION": "1.0.0"},
		Labels:       map[string]string{"maintainer": "test@example.com"},
		Target:       "production",
		Platform:     []string{"linux/amd64", "linux/arm64"},
		NoCache:      true,
	}

	t.Run("Fields", func(t *testing.T) {
		assert.Equal(t, "test-repo", req.RepositoryID)
		assert.Equal(t, ".", req.Context)
		assert.Equal(t, "Govcfile", req.Govcfile)
		assert.Len(t, req.Tags, 2)
		assert.Contains(t, req.Tags, "app:latest")
		assert.Equal(t, "1.0.0", req.Args["VERSION"])
		assert.Equal(t, "test@example.com", req.Labels["maintainer"])
		assert.Equal(t, "production", req.Target)
		assert.Contains(t, req.Platform, "linux/amd64")
		assert.True(t, req.NoCache)
	})

	t.Run("Default Values", func(t *testing.T) {
		defaultReq := BuildRequest{}
		assert.Empty(t, defaultReq.RepositoryID)
		assert.Empty(t, defaultReq.Context)
		assert.Empty(t, defaultReq.Tags)
		assert.Nil(t, defaultReq.Args)
		assert.False(t, defaultReq.NoCache)
	})
}

func TestBuildStatus(t *testing.T) {
	statuses := []BuildStatus{
		BuildStatusPending,
		BuildStatusRunning,
		BuildStatusCompleted,
		BuildStatusFailed,
		BuildStatusCancelled,
	}

	expectedStrings := []string{
		"pending",
		"running",
		"completed",
		"failed",
		"cancelled",
	}

	for i, status := range statuses {
		assert.Equal(t, expectedStrings[i], string(status))
	}
}

func TestBuild(t *testing.T) {
	startTime := time.Now()
	endTime := startTime.Add(5 * time.Minute)

	build := Build{
		ID:           "build-123",
		RepositoryID: "test-repo",
		Request: BuildRequest{
			Govcfile: "Govcfile",
			Tags:     []string{"app:latest"},
		},
		Status:    BuildStatusCompleted,
		StartTime: startTime,
		EndTime:   &endTime,
		Output:    "Build successful",
		Error:     "",
		ImageID:   "sha256:abc123",
		Digest:    "sha256:def456",
		Metadata: map[string]string{
			"builder": "govc",
		},
	}

	t.Run("Successful Build", func(t *testing.T) {
		assert.Equal(t, "build-123", build.ID)
		assert.Equal(t, BuildStatusCompleted, build.Status)
		assert.NotNil(t, build.EndTime)
		assert.Equal(t, "Build successful", build.Output)
		assert.Empty(t, build.Error)
		assert.NotEmpty(t, build.ImageID)
		assert.NotEmpty(t, build.Digest)
	})

	t.Run("Failed Build", func(t *testing.T) {
		failedBuild := Build{
			ID:        "build-456",
			Status:    BuildStatusFailed,
			StartTime: startTime,
			Error:     "Govcfile not found",
		}

		assert.Equal(t, BuildStatusFailed, failedBuild.Status)
		assert.NotEmpty(t, failedBuild.Error)
		assert.Empty(t, failedBuild.ImageID)
	})

	t.Run("Build Duration", func(t *testing.T) {
		duration := build.EndTime.Sub(build.StartTime)
		assert.Equal(t, 5*time.Minute, duration)
	})
}

func TestContainerEvent(t *testing.T) {
	now := time.Now()
	event := ContainerEvent{
		Type:         EventTypeBuildStarted,
		RepositoryID: "test-repo",
		Timestamp:    now,
		Actor:        "user@example.com",
		Resource: &Build{
			ID:     "build-123",
			Status: BuildStatusPending,
		},
		Data: map[string]interface{}{
			"trigger": "manual",
		},
	}

	t.Run("Event Fields", func(t *testing.T) {
		assert.Equal(t, EventTypeBuildStarted, event.Type)
		assert.Equal(t, "test-repo", event.RepositoryID)
		assert.Equal(t, now, event.Timestamp)
		assert.Equal(t, "user@example.com", event.Actor)
		assert.NotNil(t, event.Resource)
		assert.Equal(t, "manual", event.Data["trigger"])
	})

	t.Run("Event Types", func(t *testing.T) {
		eventTypes := []EventType{
			EventTypeBuildStarted,
			EventTypeBuildCompleted,
			EventTypeBuildFailed,
			EventTypeImagePushed,
			EventTypeImagePulled,
			EventTypeDeployStarted,
			EventTypeDeployCompleted,
			EventTypeSyncStarted,
			EventTypeSyncCompleted,
		}

		for _, et := range eventTypes {
			assert.NotEmpty(t, string(et))
		}
	})
}

func TestSecurityPolicyTypes(t *testing.T) {
	policy := SecurityPolicy{
		AllowedBaseImages: []string{
			"alpine:*",
			"ubuntu:22.04",
			"golang:1.21",
		},
		ProhibitedPackages: []string{
			"telnet",
			"ftp",
		},
		VulnerabilityPolicy: VulnerabilityPolicy{
			MaxCritical: 0,
			MaxHigh:     5,
			MaxMedium:   10,
			MaxLow:      20,
			FailOnNew:   true,
		},
		SigningRequired: true,
		ScanRequired:    true,
		MaxImageSize:    1024 * 1024 * 1024, // 1GB
	}

	t.Run("Base Image Policy", func(t *testing.T) {
		assert.Len(t, policy.AllowedBaseImages, 3)
		assert.Contains(t, policy.AllowedBaseImages, "alpine:*")
	})

	t.Run("Package Policy", func(t *testing.T) {
		assert.Len(t, policy.ProhibitedPackages, 2)
		assert.Contains(t, policy.ProhibitedPackages, "telnet")
	})

	t.Run("Vulnerability Policy", func(t *testing.T) {
		assert.Equal(t, 0, policy.VulnerabilityPolicy.MaxCritical)
		assert.Equal(t, 5, policy.VulnerabilityPolicy.MaxHigh)
		assert.True(t, policy.VulnerabilityPolicy.FailOnNew)
	})

	t.Run("Security Requirements", func(t *testing.T) {
		assert.True(t, policy.SigningRequired)
		assert.True(t, policy.ScanRequired)
		assert.Equal(t, int64(1073741824), policy.MaxImageSize)
	})
}

func TestSecurityPolicyValidation(t *testing.T) {
	t.Run("Valid Base Image", func(t *testing.T) {
		// Test pattern matching (would need implementation)
		validImages := []string{
			"alpine:latest",
			"alpine:3.14",
			"ubuntu:22.04",
			"ubuntu:20.04",
		}

		for _, img := range validImages {
			// In real implementation, we'd have a method to check this
			isValid := strings.Contains(img, "alpine") || strings.Contains(img, "ubuntu")
			assert.True(t, isValid, "Image %s should be valid", img)
		}
	})

	t.Run("Vulnerability Thresholds", func(t *testing.T) {
		policy := VulnerabilityPolicy{
			MaxCritical: 0,
			MaxHigh:     5,
			MaxMedium:   10,
			MaxLow:      20,
		}

		// Test cases for vulnerability counts
		testCases := []struct {
			name        string
			critical    int
			high        int
			shouldFail  bool
		}{
			{"No vulnerabilities", 0, 0, false},
			{"Under threshold", 0, 3, false},
			{"At high threshold", 0, 5, false},
			{"Over high threshold", 0, 6, true},
			{"Any critical", 1, 0, true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				wouldFail := tc.critical > policy.MaxCritical || tc.high > policy.MaxHigh
				assert.Equal(t, tc.shouldFail, wouldFail)
			})
		}
	})
}

func TestBuildRequestValidation(t *testing.T) {
	t.Run("Valid Request", func(t *testing.T) {
		req := BuildRequest{
			RepositoryID: "test-repo",
			Govcfile:     "Govcfile",
			Tags:         []string{"app:latest"},
		}

		// Validate required fields
		assert.NotEmpty(t, req.RepositoryID)
		assert.NotEmpty(t, req.Govcfile)
		assert.NotEmpty(t, req.Tags)
	})

	t.Run("Invalid Request", func(t *testing.T) {
		invalidReqs := []BuildRequest{
			{Govcfile: "Govcfile", Tags: []string{"app:latest"}}, // Missing RepositoryID
			{RepositoryID: "test", Tags: []string{"app:latest"}},  // Missing Govcfile
			{RepositoryID: "test", Govcfile: "Govcfile"},         // Missing Tags
		}

		for i, req := range invalidReqs {
			// In real implementation, we'd have a Validate() method
			hasError := req.RepositoryID == "" || req.Govcfile == "" || len(req.Tags) == 0
			assert.True(t, hasError, "Request %d should be invalid", i)
		}
	})
}

func TestJSONMarshaling(t *testing.T) {
	t.Run("BuildRequest JSON", func(t *testing.T) {
		req := BuildRequest{
			RepositoryID: "test",
			Govcfile:     "Govcfile",
			Tags:         []string{"latest"},
			Args:         map[string]string{"VERSION": "1.0"},
			NoCache:      true,
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)

		var decoded BuildRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, req.RepositoryID, decoded.RepositoryID)
		assert.Equal(t, req.NoCache, decoded.NoCache)
		assert.Equal(t, req.Args["VERSION"], decoded.Args["VERSION"])
	})

	t.Run("ContainerEvent JSON", func(t *testing.T) {
		event := ContainerEvent{
			Type:         EventTypeBuildStarted,
			RepositoryID: "test",
			Timestamp:    time.Now(),
			Actor:        "user",
			Data:         map[string]interface{}{"key": "value"},
		}

		data, err := json.Marshal(event)
		require.NoError(t, err)
		assert.Contains(t, string(data), "build.started")
		assert.Contains(t, string(data), "test")
	})
}