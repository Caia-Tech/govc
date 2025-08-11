package client

import (
	"context"
	"fmt"
	"io"
	"time"

	pb "github.com/Caia-Tech/govc/api/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GRPCClient provides a client for GoVC gRPC services
type GRPCClient struct {
	conn   *grpc.ClientConn
	client pb.GoVCServiceClient
}

// NewGRPCClient creates a new gRPC client
func NewGRPCClient(address string) (*GRPCClient, error) {
	// Connect with optimized options
	conn, err := grpc.Dial(address, 
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(100*1024*1024), // 100MB max message size
			grpc.MaxCallSendMsgSize(100*1024*1024), // 100MB max message size
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	return &GRPCClient{
		conn:   conn,
		client: pb.NewGoVCServiceClient(conn),
	}, nil
}

// Close closes the gRPC connection
func (c *GRPCClient) Close() error {
	return c.conn.Close()
}

// Repository Operations

func (c *GRPCClient) CreateRepository(ctx context.Context, id, name, description string) (*pb.CreateRepositoryResponse, error) {
	req := &pb.CreateRepositoryRequest{
		Id:          id,
		Name:        name,
		Description: description,
		Metadata:    make(map[string]string),
	}
	
	return c.client.CreateRepository(ctx, req)
}

func (c *GRPCClient) GetRepository(ctx context.Context, id string) (*pb.GetRepositoryResponse, error) {
	req := &pb.GetRepositoryRequest{Id: id}
	return c.client.GetRepository(ctx, req)
}

func (c *GRPCClient) ListRepositories(ctx context.Context, limit, offset int32) (*pb.ListRepositoriesResponse, error) {
	req := &pb.ListRepositoriesRequest{
		Limit:  limit,
		Offset: offset,
	}
	return c.client.ListRepositories(ctx, req)
}

// Blob Operations

func (c *GRPCClient) StoreBlob(ctx context.Context, repositoryID string, content []byte) (*pb.StoreBlobResponse, error) {
	req := &pb.StoreBlobRequest{
		RepositoryId: repositoryID,
		Content:      content,
	}
	return c.client.StoreBlob(ctx, req)
}

func (c *GRPCClient) GetBlob(ctx context.Context, repositoryID, hash string) (*pb.GetBlobResponse, error) {
	req := &pb.GetBlobRequest{
		RepositoryId: repositoryID,
		Hash:         hash,
	}
	return c.client.GetBlob(ctx, req)
}

func (c *GRPCClient) StoreBlobWithDelta(ctx context.Context, repositoryID string, content []byte) (*pb.StoreBlobWithDeltaResponse, error) {
	req := &pb.StoreBlobWithDeltaRequest{
		RepositoryId:   repositoryID,
		Content:        content,
		UseCompression: true,
	}
	return c.client.StoreBlobWithDelta(ctx, req)
}

// File Operations

func (c *GRPCClient) AddFile(ctx context.Context, repositoryID, path string, content []byte) (*pb.AddFileResponse, error) {
	req := &pb.AddFileRequest{
		RepositoryId: repositoryID,
		Path:         path,
		Content:      content,
	}
	return c.client.AddFile(ctx, req)
}

func (c *GRPCClient) ReadFile(ctx context.Context, repositoryID, path string) (*pb.ReadFileResponse, error) {
	req := &pb.ReadFileRequest{
		RepositoryId: repositoryID,
		Path:         path,
	}
	return c.client.ReadFile(ctx, req)
}

func (c *GRPCClient) WriteFile(ctx context.Context, repositoryID, path string, content []byte) (*pb.WriteFileResponse, error) {
	req := &pb.WriteFileRequest{
		RepositoryId: repositoryID,
		Path:         path,
		Content:      content,
		CreateDirs:   true,
	}
	return c.client.WriteFile(ctx, req)
}

// Commit Operations

func (c *GRPCClient) Commit(ctx context.Context, repositoryID, message, author string, files []string) (*pb.CommitResponse, error) {
	req := &pb.CommitRequest{
		RepositoryId: repositoryID,
		Message:      message,
		Author:       author,
		Files:        files,
	}
	return c.client.Commit(ctx, req)
}

func (c *GRPCClient) GetCommit(ctx context.Context, repositoryID, hash string) (*pb.GetCommitResponse, error) {
	req := &pb.GetCommitRequest{
		RepositoryId: repositoryID,
		Hash:         hash,
	}
	return c.client.GetCommit(ctx, req)
}

func (c *GRPCClient) ListCommits(ctx context.Context, repositoryID string, limit, offset int32) (*pb.ListCommitsResponse, error) {
	req := &pb.ListCommitsRequest{
		RepositoryId: repositoryID,
		Limit:        limit,
		Offset:       offset,
	}
	return c.client.ListCommits(ctx, req)
}

// Search Operations

func (c *GRPCClient) FullTextSearch(ctx context.Context, repositoryID, query string, options *FullTextSearchOptions) (*pb.FullTextSearchResponse, error) {
	if options == nil {
		options = &FullTextSearchOptions{}
	}

	req := &pb.FullTextSearchRequest{
		RepositoryId:    repositoryID,
		Query:           query,
		FileTypes:       options.FileTypes,
		MaxSize:         options.MaxSize,
		MinScore:        options.MinScore,
		IncludeContent:  options.IncludeContent,
		HighlightLength: int32(options.HighlightLength),
		Limit:           int32(options.Limit),
		Offset:          int32(options.Offset),
		SortBy:          options.SortBy,
	}
	
	return c.client.FullTextSearch(ctx, req)
}

func (c *GRPCClient) ExecuteSQLQuery(ctx context.Context, repositoryID, sqlQuery string) (*pb.ExecuteSQLQueryResponse, error) {
	req := &pb.ExecuteSQLQueryRequest{
		RepositoryId: repositoryID,
		SqlQuery:     sqlQuery,
	}
	return c.client.ExecuteSQLQuery(ctx, req)
}

// Streaming Operations

func (c *GRPCClient) StreamBlob(ctx context.Context, repositoryID, hash string, chunkSize int32) (*BlobStream, error) {
	stream, err := c.client.StreamBlob(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	// Send initial request
	req := &pb.StreamBlobRequest{
		RepositoryId: repositoryID,
		Hash:         hash,
		ChunkSize:    chunkSize,
	}

	if err := stream.Send(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return &BlobStream{stream: stream}, nil
}

func (c *GRPCClient) UploadBlobStream(ctx context.Context, repositoryID string) (*BlobUploadStream, error) {
	stream, err := c.client.UploadBlobStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload stream: %w", err)
	}

	return &BlobUploadStream{
		stream:       stream,
		repositoryID: repositoryID,
	}, nil
}

// Batch Operations

func (c *GRPCClient) BatchOperations(ctx context.Context, repositoryID string, operations []*pb.BatchOperation, parallel bool) (*pb.BatchOperationsResponse, error) {
	req := &pb.BatchOperationsRequest{
		RepositoryId: repositoryID,
		Operations:   operations,
		Parallel:     parallel,
		Transaction:  false,
	}
	return c.client.BatchOperations(ctx, req)
}

// Health Operations

func (c *GRPCClient) GetHealth(ctx context.Context) (*pb.HealthResponse, error) {
	return c.client.GetHealth(ctx, &emptypb.Empty{})
}

func (c *GRPCClient) GetStatus(ctx context.Context, repositoryID string) (*pb.GetStatusResponse, error) {
	req := &pb.GetStatusRequest{RepositoryId: repositoryID}
	return c.client.GetStatus(ctx, req)
}

// Streaming types and methods

type FullTextSearchOptions struct {
	FileTypes       []string
	MaxSize         int64
	MinScore        float64
	IncludeContent  bool
	HighlightLength int
	Limit           int
	Offset          int
	SortBy          string
}

type BlobStream struct {
	stream pb.GoVCService_StreamBlobClient
}

func (s *BlobStream) Recv() (*pb.StreamBlobResponse, error) {
	return s.stream.Recv()
}

func (s *BlobStream) Close() error {
	return s.stream.CloseSend()
}

type BlobUploadStream struct {
	stream       pb.GoVCService_UploadBlobStreamClient
	repositoryID string
}

func (s *BlobUploadStream) SendMetadata(filename string, totalSize int64, chunkSize int32, contentType string) error {
	req := &pb.UploadBlobStreamRequest{
		RepositoryId: s.repositoryID,
		Request: &pb.UploadBlobStreamRequest_Metadata{
			Metadata: &pb.UploadMetadata{
				Filename:    filename,
				TotalSize:   totalSize,
				ChunkSize:   chunkSize,
				ContentType: contentType,
			},
		},
	}
	return s.stream.Send(req)
}

func (s *BlobUploadStream) SendChunk(streamID string, sequenceNum int32, data []byte, checksum string, isLast bool) error {
	chunk := &pb.StreamChunk{
		StreamId:    streamID,
		SequenceNum: sequenceNum,
		Data:        data,
		Checksum:    checksum,
		IsLast:      isLast,
	}

	req := &pb.UploadBlobStreamRequest{
		RepositoryId: s.repositoryID,
		Request: &pb.UploadBlobStreamRequest_Chunk{
			Chunk: chunk,
		},
	}
	return s.stream.Send(req)
}

func (s *BlobUploadStream) CloseAndRecv() (*pb.UploadBlobStreamResponse, error) {
	return s.stream.CloseAndRecv()
}

// Performance Benchmarking Utilities

type BenchmarkResult struct {
	Operation    string
	Duration     time.Duration
	BytesPerSec  float64
	OpsPerSec    float64
	Success      bool
	Error        error
}

func (c *GRPCClient) BenchmarkBlob(ctx context.Context, repositoryID string, data []byte, iterations int) *BenchmarkResult {
	start := time.Now()
	
	for i := 0; i < iterations; i++ {
		_, err := c.StoreBlobWithDelta(ctx, repositoryID, data)
		if err != nil {
			return &BenchmarkResult{
				Operation: "StoreBlobWithDelta",
				Duration:  time.Since(start),
				Success:   false,
				Error:     err,
			}
		}
	}
	
	duration := time.Since(start)
	totalBytes := int64(len(data) * iterations)
	
	return &BenchmarkResult{
		Operation:   "StoreBlobWithDelta",
		Duration:    duration,
		BytesPerSec: float64(totalBytes) / duration.Seconds(),
		OpsPerSec:   float64(iterations) / duration.Seconds(),
		Success:     true,
	}
}

func (c *GRPCClient) BenchmarkBatch(ctx context.Context, repositoryID string, operationCount int) *BenchmarkResult {
	operations := make([]*pb.BatchOperation, operationCount)
	
	for i := 0; i < operationCount; i++ {
		content := fmt.Sprintf("Test content for operation %d", i)
		operations[i] = &pb.BatchOperation{
			Id:     fmt.Sprintf("op_%d", i),
			Type:   "store_blob",
			Params: []byte(fmt.Sprintf(`{"content": "%s"}`, content)),
		}
	}
	
	start := time.Now()
	resp, err := c.BatchOperations(ctx, repositoryID, operations, true)
	duration := time.Since(start)
	
	if err != nil {
		return &BenchmarkResult{
			Operation: "BatchOperations",
			Duration:  duration,
			Success:   false,
			Error:     err,
		}
	}
	
	return &BenchmarkResult{
		Operation: "BatchOperations",
		Duration:  duration,
		OpsPerSec: float64(operationCount) / duration.Seconds(),
		Success:   resp.Success,
	}
}