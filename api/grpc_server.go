package api

import (
	"context"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/Caia-Tech/govc"
	pb "github.com/Caia-Tech/govc/api/proto"
)

// GRPCServer implements the govc gRPC service
type GRPCServer struct {
	pb.UnimplementedGoVCServiceServer
	repo   *govc.Repository
	server *grpc.Server
	logger *log.Logger
}

// NewGRPCServer creates a new gRPC server
func NewGRPCServer(repo *govc.Repository, logger *log.Logger) *GRPCServer {
	if logger == nil {
		logger = log.Default()
	}
	
	return &GRPCServer{
		repo:   repo,
		logger: logger,
	}
}

// Start starts the gRPC server
func (s *GRPCServer) Start(address string) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	s.server = grpc.NewServer()
	pb.RegisterGoVCServiceServer(s.server, s)
	
	// Register reflection service for debugging
	reflection.Register(s.server)

	s.logger.Printf("gRPC server listening on %s", address)
	return s.server.Serve(lis)
}

// Stop stops the gRPC server
func (s *GRPCServer) Stop() {
	if s.server != nil {
		s.server.GracefulStop()
	}
}

// CreateRepository creates a new repository
func (s *GRPCServer) CreateRepository(ctx context.Context, req *pb.CreateRepositoryRequest) (*pb.CreateRepositoryResponse, error) {
	// In this simple implementation, we use a single repository
	// In production, you'd manage multiple repositories
	return &pb.CreateRepositoryResponse{
		Id: req.Id,
	}, nil
}

// AddFile adds a file to the repository
func (s *GRPCServer) AddFile(ctx context.Context, req *pb.AddFileRequest) (*pb.AddFileResponse, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}
	
	staging := s.repo.GetStagingArea()
	err := staging.Add(req.Path, req.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to add file: %w", err)
	}
	
	// Calculate hash for response
	hash := fmt.Sprintf("%x", req.Content)
	
	return &pb.AddFileResponse{
		Hash: hash[:8], // Just return first 8 chars for simplicity
		Size: int64(len(req.Content)),
	}, nil
}

// Commit creates a new commit
func (s *GRPCServer) Commit(ctx context.Context, req *pb.CommitRequest) (*pb.CommitResponse, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}
	
	commit, err := s.repo.Commit(req.Message)
	if err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}
	
	return &pb.CommitResponse{
		Hash:    commit.Hash(),
		Message: commit.Message,
		Author:  commit.Author.Name,
	}, nil
}

// GetCommit retrieves a commit by hash
func (s *GRPCServer) GetCommit(ctx context.Context, req *pb.GetCommitRequest) (*pb.GetCommitResponse, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}
	
	commit, err := s.repo.GetCommit(req.Hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}
	
	return &pb.GetCommitResponse{
		Hash:    commit.Hash(),
		Message: commit.Message,
		Author:  commit.Author.Name,
	}, nil
}

// ListCommits lists commits in the repository
func (s *GRPCServer) ListCommits(ctx context.Context, req *pb.ListCommitsRequest) (*pb.ListCommitsResponse, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}
	
	commits, err := s.repo.ListCommits("main", int(req.Limit))
	if err != nil {
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}
	
	pbCommits := make([]*pb.GetCommitResponse, len(commits))
	for i, commit := range commits {
		pbCommits[i] = &pb.GetCommitResponse{
			Hash:    commit.Hash(),
			Message: commit.Message,
			Author:  commit.Author.Name,
		}
	}
	
	return &pb.ListCommitsResponse{
		Commits: pbCommits,
		Total:   int32(len(commits)),
	}, nil
}


// CreateBranch creates a new branch
func (s *GRPCServer) CreateBranch(ctx context.Context, req *pb.CreateBranchRequest) (*pb.CreateBranchResponse, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}
	
	err := s.repo.CreateBranch(req.Name, req.FromCommit)
	if err != nil {
		return nil, fmt.Errorf("failed to create branch: %w", err)
	}
	
	return &pb.CreateBranchResponse{
		Name:       req.Name,
		CommitHash: req.FromCommit,
	}, nil
}

// ListBranches lists all branches
func (s *GRPCServer) ListBranches(ctx context.Context, req *pb.ListBranchesRequest) (*pb.ListBranchesResponse, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}
	
	branches, err := s.repo.ListBranches()
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}
	
	// Convert string branches to Branch messages
	pbBranches := make([]*pb.Branch, len(branches))
	for i, branch := range branches {
		pbBranches[i] = &pb.Branch{
			Name: branch,
			// We'd need to get more info for complete Branch message
			// For now, just set the name
		}
	}
	
	return &pb.ListBranchesResponse{
		Branches: pbBranches,
	}, nil
}

