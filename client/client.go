// Package client provides Go clients for govc server
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	
	"github.com/Caia-Tech/govc/api"
	pb "github.com/Caia-Tech/govc/api/proto"
)

// StreamingClient provides streaming capabilities for large operations
type StreamingClient struct {
	baseURL    string
	repoID     string
	httpClient *http.Client
	options    *api.ClientOptions
}

// NewStreamingClient creates a new streaming client
func NewStreamingClient(baseURL, repoID string, options *api.ClientOptions) *StreamingClient {
	if options == nil {
		options = &api.ClientOptions{
			Timeout: 30 * time.Second,
		}
	}
	
	return &StreamingClient{
		baseURL: baseURL,
		repoID:  repoID,
		httpClient: &http.Client{
			Timeout: options.Timeout,
		},
		options: options,
	}
}

// StreamAdd streams a file addition to the repository
func (c *StreamingClient) StreamAdd(ctx context.Context, path string, content []byte) error {
	url := fmt.Sprintf("%s/api/v1/repos/%s/files", c.baseURL, c.repoID)
	
	reqBody := map[string]interface{}{
		"path":    path,
		"content": content,
	}
	
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to add file: %s", string(body))
	}
	
	return nil
}

// StreamCommit creates a commit
func (c *StreamingClient) StreamCommit(ctx context.Context, message string) error {
	url := fmt.Sprintf("%s/api/v1/repos/%s/commits", c.baseURL, c.repoID)
	
	reqBody := map[string]interface{}{
		"message": message,
		"author": map[string]string{
			"name":  "Test User",
			"email": "test@example.com",
		},
	}
	
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to commit: %s", string(body))
	}
	
	return nil
}

// GRPCClient provides gRPC client for govc server
type GRPCClient struct {
	conn   *grpc.ClientConn
	client pb.GoVCServiceClient
}

// NewGRPCClient creates a new gRPC client
func NewGRPCClient(address string) (*GRPCClient, error) {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	
	return &GRPCClient{
		conn:   conn,
		client: pb.NewGoVCServiceClient(conn),
	}, nil
}

// Close closes the gRPC connection
func (c *GRPCClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// CreateRepository creates a new repository
func (c *GRPCClient) CreateRepository(ctx context.Context, id string) error {
	_, err := c.client.CreateRepository(ctx, &pb.CreateRepositoryRequest{
		Id: id,
	})
	return err
}

// AddFile adds a file to the repository
func (c *GRPCClient) AddFile(ctx context.Context, repoID, path string, content []byte) error {
	_, err := c.client.AddFile(ctx, &pb.AddFileRequest{
		RepositoryId:  repoID,
		Path:    path,
		Content: content,
	})
	return err
}

// Commit creates a commit
func (c *GRPCClient) Commit(ctx context.Context, repoID, message string) (*pb.CommitResponse, error) {
	return c.client.Commit(ctx, &pb.CommitRequest{
		RepositoryId:  repoID,
		Message: message,
		Author: "Test User",
	})
}

// GetCommit retrieves a commit by hash
func (c *GRPCClient) GetCommit(ctx context.Context, repoID, hash string) (*pb.GetCommitResponse, error) {
	return c.client.GetCommit(ctx, &pb.GetCommitRequest{
		RepositoryId: repoID,
		Hash:   hash,
	})
}

// ListCommits lists commits in a repository
func (c *GRPCClient) ListCommits(ctx context.Context, repoID string, limit int32) (*pb.ListCommitsResponse, error) {
	return c.client.ListCommits(ctx, &pb.ListCommitsRequest{
		RepositoryId: repoID,
		Limit:  limit,
	})
}