package govc

import (
	"context"
	"fmt"
	"net/url"
)

// ParallelReality represents an isolated branch universe
type ParallelReality struct {
	Name      string                 `json:"name"`
	Isolated  bool                   `json:"isolated"`
	Ephemeral bool                   `json:"ephemeral"`
	Metrics   map[string]interface{} `json:"metrics,omitempty"`
}

// ParallelRealitiesRequest represents a request to create parallel realities
type ParallelRealitiesRequest struct {
	Branches []string `json:"branches"`
}

// CreateParallelRealities creates multiple isolated branch universes
func (r *Repository) CreateParallelRealities(ctx context.Context, names []string) ([]*ParallelReality, error) {
	req := ParallelRealitiesRequest{
		Branches: names,
	}

	var response struct {
		Realities []*ParallelReality `json:"realities"`
		Count     int                `json:"count"`
	}

	err := r.client.post(ctx,
		fmt.Sprintf("/api/v1/repos/%s/parallel-realities", url.PathEscape(r.ID)),
		req, &response)

	return response.Realities, err
}

// BenchmarkResult represents performance metrics for a reality
type BenchmarkResult struct {
	Reality  string             `json:"reality"`
	Duration string             `json:"duration"`
	Metrics  map[string]float64 `json:"metrics"`
	Better   bool               `json:"better"`
}

// BenchmarkReality runs performance tests on a parallel reality
func (r *Repository) BenchmarkReality(ctx context.Context, realityName string) (*BenchmarkResult, error) {
	var result BenchmarkResult
	err := r.client.post(ctx,
		fmt.Sprintf("/api/v1/repos/%s/parallel-realities/%s/benchmark",
			url.PathEscape(r.ID), url.PathEscape(realityName)),
		nil, &result)

	return &result, err
}

// TimeTravel returns a historical snapshot of the repository
func (r *Repository) TimeTravel(ctx context.Context, timestamp string) (*HistoricalSnapshot, error) {
	var snapshot HistoricalSnapshot
	err := r.client.get(ctx,
		fmt.Sprintf("/api/v1/repos/%s/time-travel?time=%s",
			url.PathEscape(r.ID), url.QueryEscape(timestamp)),
		&snapshot)

	return &snapshot, err
}

// HistoricalSnapshot represents a point-in-time view of the repository
type HistoricalSnapshot struct {
	Time   string  `json:"time"`
	Commit *Commit `json:"commit"`
	Files  []struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	} `json:"files"`
}
