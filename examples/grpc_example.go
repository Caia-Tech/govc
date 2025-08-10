package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/caiatech/govc"
	"github.com/caiatech/govc/api"
	pb "github.com/caiatech/govc/api/proto"
	"github.com/caiatech/govc/client"
)

func main() {
	// Example usage of GoVC gRPC API
	
	// Start gRPC server
	repo := govc.NewRepository()
	grpcServer := api.NewGRPCServer(repo, log.Default())
	
	// Start server in background
	go func() {
		if err := grpcServer.Start(":9090"); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()
	
	// Give server time to start
	time.Sleep(100 * time.Millisecond)
	defer grpcServer.Stop()
	
	// Create gRPC client
	grpcClient, err := client.NewGRPCClient("localhost:9090")
	if err != nil {
		log.Fatalf("Failed to create gRPC client: %v", err)
	}
	defer grpcClient.Close()
	
	ctx := context.Background()
	
	// Example 1: Health Check
	fmt.Println("=== Example 1: Health Check ===")
	health, err := grpcClient.GetHealth(ctx)
	if err != nil {
		log.Printf("Health check failed: %v", err)
	} else {
		fmt.Printf("Server status: %s (uptime: %.1fs)\n", health.Status, health.UptimeSeconds)
		for check, status := range health.Checks {
			fmt.Printf("  %s: %s\n", check, status)
		}
	}
	
	// Example 2: Repository Management
	fmt.Println("\n=== Example 2: Repository Management ===")
	repoResp, err := grpcClient.CreateRepository(ctx, "test-repo", "Test Repository", "A test repository for gRPC examples")
	if err != nil {
		log.Printf("Failed to create repository: %v", err)
	} else {
		fmt.Printf("Created repository: %s\n", repoResp.Name)
	}
	
	// List repositories
	repos, err := grpcClient.ListRepositories(ctx, 10, 0)
	if err != nil {
		log.Printf("Failed to list repositories: %v", err)
	} else {
		fmt.Printf("Found %d repositories:\n", repos.Total)
		for _, repo := range repos.Repositories {
			fmt.Printf("  - %s: %s\n", repo.Name, repo.Description)
		}
	}
	
	// Example 3: Blob Operations
	fmt.Println("\n=== Example 3: Blob Operations ===")
	testContent := []byte("Hello, gRPC World! This is a test blob with some content.")
	
	// Store blob
	blobResp, err := grpcClient.StoreBlobWithDelta(ctx, "test-repo", testContent)
	if err != nil {
		log.Printf("Failed to store blob: %v", err)
	} else {
		fmt.Printf("Stored blob: %s (size: %d bytes, compression: %.2f)\n", 
			blobResp.Hash[:8], blobResp.Size, blobResp.CompressionRatio)
	}
	
	if blobResp != nil {
		// Retrieve blob
		retrievedBlob, err := grpcClient.GetBlob(ctx, "test-repo", blobResp.Hash)
		if err != nil {
			log.Printf("Failed to retrieve blob: %v", err)
		} else {
			fmt.Printf("Retrieved blob: %s (size: %d bytes)\n", 
				retrievedBlob.Hash[:8], retrievedBlob.Size)
			fmt.Printf("Content preview: %.50s...\n", string(retrievedBlob.Content))
		}
	}
	
	// Example 4: File Operations
	fmt.Println("\n=== Example 4: File Operations ===")
	
	// Write files
	files := map[string]string{
		"README.md":      "# Test Repository\n\nThis is a test repository for gRPC examples.",
		"src/main.go":    "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello, World!\")\n}",
		"config.yaml":    "server:\n  host: localhost\n  port: 8080\ndatabase:\n  url: postgres://localhost/test",
	}
	
	for path, content := range files {
		writeResp, err := grpcClient.WriteFile(ctx, "test-repo", path, []byte(content))
		if err != nil {
			log.Printf("Failed to write file %s: %v", path, err)
		} else {
			fmt.Printf("Wrote file %s (%d bytes, hash: %s)\n", 
				path, writeResp.Size, writeResp.Hash[:8])
		}
	}
	
	// Read file back
	readResp, err := grpcClient.ReadFile(ctx, "test-repo", "README.md")
	if err != nil {
		log.Printf("Failed to read file: %v", err)
	} else {
		fmt.Printf("Read README.md (%d bytes):\n%s\n", readResp.Size, string(readResp.Content))
	}
	
	// Example 5: Commit Operations
	fmt.Println("\n=== Example 5: Commit Operations ===")
	
	commitResp, err := grpcClient.Commit(ctx, "test-repo", "Initial commit with test files", "gRPC Example", []string{"README.md", "src/main.go", "config.yaml"})
	if err != nil {
		log.Printf("Failed to create commit: %v", err)
	} else {
		fmt.Printf("Created commit: %s\n", commitResp.Hash[:8])
		fmt.Printf("Message: %s\n", commitResp.Message)
		fmt.Printf("Author: %s\n", commitResp.Author)
	}
	
	// List commits
	commitsResp, err := grpcClient.ListCommits(ctx, "test-repo", 5, 0)
	if err != nil {
		log.Printf("Failed to list commits: %v", err)
	} else {
		fmt.Printf("Found %d commits:\n", commitsResp.Total)
		for _, commit := range commitsResp.Commits {
			fmt.Printf("  %s: %s by %s\n", commit.Hash[:8], commit.Message, commit.Author)
		}
	}
	
	// Example 6: Search Operations
	fmt.Println("\n=== Example 6: Search Operations ===")
	
	// Full-text search
	searchOptions := &client.FullTextSearchOptions{
		IncludeContent:  false,
		HighlightLength: 100,
		Limit:           10,
		SortBy:          "score",
	}
	
	searchResp, err := grpcClient.FullTextSearch(ctx, "test-repo", "Hello", searchOptions)
	if err != nil {
		log.Printf("Failed to search: %v", err)
	} else {
		fmt.Printf("Search found %d results (%.2fms):\n", searchResp.Total, searchResp.QueryTimeMs)
		for _, result := range searchResp.Results {
			fmt.Printf("  %s (score: %.3f): %s\n", 
				result.Document.Path, result.Score, result.Document.Hash[:8])
		}
	}
	
	// SQL query
	sqlResp, err := grpcClient.ExecuteSQLQuery(ctx, "test-repo", "SELECT path, size FROM files WHERE path LIKE '%.go'")
	if err != nil {
		log.Printf("Failed to execute SQL: %v", err)
	} else {
		fmt.Printf("SQL query returned %d rows (%.2fms):\n", sqlResp.Total, sqlResp.QueryTimeMs)
		for _, row := range sqlResp.Rows {
			fmt.Printf("  %s\n", formatRow(row.Fields))
		}
	}
	
	// Example 7: Batch Operations
	fmt.Println("\n=== Example 7: Batch Operations ===")
	
	// Create batch operations
	operations := make([]*pb.BatchOperation, 5)
	for i := 0; i < 5; i++ {
		content := fmt.Sprintf("Batch content #%d created via gRPC", i+1)
		operations[i] = &pb.BatchOperation{
			Id:     fmt.Sprintf("batch_op_%d", i),
			Type:   "store_blob",
			Params: []byte(fmt.Sprintf(`{"content": "%s"}`, content)),
		}
	}
	
	batchResp, err := grpcClient.BatchOperations(ctx, "test-repo", operations, true)
	if err != nil {
		log.Printf("Failed to execute batch: %v", err)
	} else {
		fmt.Printf("Batch operations completed in %.2fms (success: %t)\n", 
			batchResp.ExecutionTimeMs, batchResp.Success)
		fmt.Printf("Results:\n")
		for _, result := range batchResp.Results {
			if result.Success {
				fmt.Printf("  ✓ %s: completed\n", result.Id)
			} else {
				fmt.Printf("  ✗ %s: %s\n", result.Id, result.Error)
			}
		}
	}
	
	// Example 8: Streaming Operations
	fmt.Println("\n=== Example 8: Streaming Operations ===")
	
	// Create large content for streaming
	largeContent := []byte(strings.Repeat("This is streaming content. ", 1000)) // ~27KB
	
	// Store the large blob first
	largeBlobResp, err := grpcClient.StoreBlobWithDelta(ctx, "test-repo", largeContent)
	if err != nil {
		log.Printf("Failed to store large blob: %v", err)
	} else {
		fmt.Printf("Stored large blob: %s (%d bytes)\n", largeBlobResp.Hash[:8], largeBlobResp.Size)
		
		// Stream the blob back
		stream, err := grpcClient.StreamBlob(ctx, "test-repo", largeBlobResp.Hash, 8192) // 8KB chunks
		if err != nil {
			log.Printf("Failed to create stream: %v", err)
		} else {
			defer stream.Close()
			
			fmt.Printf("Streaming blob in chunks:\n")
			var totalBytes int64 = 0
			chunkCount := 0
			
			for {
				resp, err := stream.Recv()
				if err != nil {
					if err.Error() == "EOF" {
						break
					}
					log.Printf("Stream error: %v", err)
					break
				}
				
				switch r := resp.Response.(type) {
				case *pb.StreamBlobResponse_Metadata:
					fmt.Printf("  Metadata: %d bytes, %d chunks\n", 
						r.Metadata.TotalSize, r.Metadata.ChunkCount)
				case *pb.StreamBlobResponse_Chunk:
					totalBytes += int64(len(r.Chunk.Data))
					chunkCount++
					if chunkCount%5 == 0 || r.Chunk.IsLast {
						fmt.Printf("  Chunk %d: %d bytes (total: %d bytes)\n", 
							r.Chunk.SequenceNum, len(r.Chunk.Data), totalBytes)
					}
					if r.Chunk.IsLast {
						fmt.Printf("  ✓ Streaming completed\n")
						break
					}
				case *pb.StreamBlobResponse_Progress:
					fmt.Printf("  Progress: %.1f%% (%s)\n", 
						float64(r.Progress.BytesStreamed)/float64(r.Progress.TotalBytes)*100,
						r.Progress.Status)
				}
			}
		}
	}
	
	// Example 9: Performance Benchmarking
	fmt.Println("\n=== Example 9: Performance Benchmarking ===")
	
	// Benchmark blob operations
	benchData := []byte("Benchmark test data for performance measurement.")
	blobBench := grpcClient.BenchmarkBlob(ctx, "test-repo", benchData, 100)
	
	if blobBench.Success {
		fmt.Printf("Blob Benchmark Results:\n")
		fmt.Printf("  Operations: 100 store operations\n")
		fmt.Printf("  Duration: %v\n", blobBench.Duration)
		fmt.Printf("  Throughput: %.1f ops/sec\n", blobBench.OpsPerSec)
		fmt.Printf("  Data rate: %.1f KB/sec\n", blobBench.BytesPerSec/1024)
	} else {
		fmt.Printf("Blob benchmark failed: %v\n", blobBench.Error)
	}
	
	// Benchmark batch operations
	batchBench := grpcClient.BenchmarkBatch(ctx, "test-repo", 50)
	
	if batchBench.Success {
		fmt.Printf("Batch Benchmark Results:\n")
		fmt.Printf("  Operations: 50 batch operations\n")
		fmt.Printf("  Duration: %v\n", batchBench.Duration)
		fmt.Printf("  Throughput: %.1f ops/sec\n", batchBench.OpsPerSec)
	} else {
		fmt.Printf("Batch benchmark failed: %v\n", batchBench.Error)
	}
	
	fmt.Println("\n=== gRPC Examples Complete ===")
}

func formatRow(fields map[string]string) string {
	var parts []string
	for key, value := range fields {
		parts = append(parts, fmt.Sprintf("%s: %s", key, value))
	}
	return strings.Join(parts, ", ")
}