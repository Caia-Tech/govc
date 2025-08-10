package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/caiatech/govc/api"
	"github.com/caiatech/govc/client"
)

func main() {
	// Example usage of GoVC streaming capabilities
	
	// Configuration
	baseURL := "http://localhost:8080"
	repoID := "example-repo"
	
	// Create optimized client options
	clientOptions := &api.ClientOptions{
		BaseURL:    baseURL,
		Timeout:    300 * time.Second,
		BinaryMode: true,
		EnableGzip: true,
		ConnectionPool: &api.ConnectionPool{
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     120 * time.Second,
			DialTimeout:         10 * time.Second,
			KeepAliveTimeout:    30 * time.Second,
		},
	}
	
	// Create streaming client
	streamClient := client.NewStreamingClient(baseURL, repoID, clientOptions)
	
	ctx := context.Background()
	
	// Example 1: Upload a large file using streaming
	fmt.Println("=== Example 1: Streaming Upload ===")
	
	// Create sample large content (simulate a 10MB file)
	largeContent := strings.Repeat("This is sample content for streaming upload test. ", 200000)
	contentReader := strings.NewReader(largeContent)
	
	uploadOpts := &client.UploadOptions{
		ChunkSize:       64 * 1024, // 64KB chunks
		EnableGzip:      true,
		VerifyUpload:    true,
		ProgressCallback: func(bytesUploaded, totalBytes int64) {
			fmt.Printf("Upload progress: %d bytes uploaded\n", bytesUploaded)
		},
	}
	
	hash, err := streamClient.UploadBlob(ctx, contentReader, uploadOpts)
	if err != nil {
		log.Printf("Upload failed: %v", err)
	} else {
		fmt.Printf("Upload completed! Hash: %s\n", hash)
	}
	
	// Example 2: Download using streaming reader
	fmt.Println("\n=== Example 2: Streaming Download ===")
	
	if hash != "" {
		downloadOpts := &client.DownloadOptions{
			ChunkSize:    32 * 1024, // 32KB chunks
			EnableGzip:   true,
			VerifyHashes: true,
		}
		
		// Create output file
		outputFile, err := os.Create("/tmp/downloaded_blob.txt")
		if err != nil {
			log.Printf("Failed to create output file: %v", err)
		} else {
			defer outputFile.Close()
			
			// Download with progress monitoring
			err = streamClient.DownloadBlob(ctx, hash, outputFile, downloadOpts)
			if err != nil {
				log.Printf("Download failed: %v", err)
			} else {
				fmt.Printf("Download completed to /tmp/downloaded_blob.txt\n")
			}
		}
	}
	
	// Example 3: Manual streaming reader with progress
	fmt.Println("\n=== Example 3: Manual Streaming Reader ===")
	
	if hash != "" {
		reader, err := streamClient.NewStreamReader(ctx, hash, &client.DownloadOptions{
			ChunkSize: 16 * 1024, // 16KB chunks
		})
		if err != nil {
			log.Printf("Failed to create stream reader: %v", err)
		} else {
			defer reader.Close()
			
			buffer := make([]byte, 8192)
			totalRead := int64(0)
			
			fmt.Printf("Starting manual read of blob...\n")
			for {
				n, err := reader.Read(buffer)
				if n > 0 {
					totalRead += int64(n)
					fmt.Printf("Read %d bytes (total: %d)\n", n, totalRead)
				}
				
				if err == io.EOF {
					fmt.Printf("Finished reading blob. Total: %d bytes\n", totalRead)
					break
				}
				if err != nil {
					log.Printf("Read error: %v", err)
					break
				}
				
				// Get progress every few chunks
				if totalRead%65536 == 0 {
					progress, err := reader.GetProgress()
					if err == nil {
						fmt.Printf("Stream progress: %.1f%% complete (%.2f MB/s)\n",
							float64(progress.BytesStreamed)/float64(progress.TotalBytes)*100,
							progress.TransferRate)
					}
				}
			}
		}
	}
	
	// Example 4: WebSocket streaming
	fmt.Println("\n=== Example 4: WebSocket Streaming ===")
	
	wsStream, err := streamClient.NewWebSocketStream(ctx)
	if err != nil {
		log.Printf("Failed to create WebSocket stream: %v", err)
	} else {
		defer wsStream.Close()
		
		if hash != "" {
			fmt.Printf("Starting WebSocket stream for blob %s\n", hash[:8])
			
			progressCallback := func(progress *api.StreamProgress) {
				fmt.Printf("WebSocket progress: %d/%d bytes (%.1f%%) - %s\n",
					progress.BytesStreamed, progress.TotalBytes,
					float64(progress.BytesStreamed)/float64(progress.TotalBytes)*100,
					progress.Status)
			}
			
			err = wsStream.StreamBlob(hash, 32*1024, progressCallback)
			if err != nil {
				log.Printf("WebSocket streaming failed: %v", err)
			} else {
				fmt.Printf("WebSocket streaming completed successfully\n")
			}
		}
	}
	
	// Example 5: Streaming with range requests
	fmt.Println("\n=== Example 5: Range Request Streaming ===")
	
	if hash != "" {
		// Download only the first 1KB of the blob
		rangeOpts := &client.DownloadOptions{
			ChunkSize:   512,  // Small chunks
			StartOffset: 0,    // Start from beginning
			EndOffset:   1023, // First 1KB only
			EnableGzip:  false,
		}
		
		reader, err := streamClient.NewStreamReader(ctx, hash, rangeOpts)
		if err != nil {
			log.Printf("Failed to create range reader: %v", err)
		} else {
			defer reader.Close()
			
			var output strings.Builder
			buffer := make([]byte, 256)
			
			for {
				n, err := reader.Read(buffer)
				if n > 0 {
					output.Write(buffer[:n])
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					log.Printf("Range read error: %v", err)
					break
				}
			}
			
			fmt.Printf("Range request result (first 100 chars): %.100s...\n", 
				output.String())
		}
	}
	
	fmt.Println("\n=== Streaming Examples Complete ===")
}

// Helper functions for demonstration

func createLargeTestFile(sizeMB int) *strings.Reader {
	// Create a test file of specified size
	content := strings.Repeat("A", 1024*1024) // 1MB of 'A's
	fullContent := strings.Repeat(content, sizeMB)
	return strings.NewReader(fullContent)
}

func printProgressBar(current, total int64) {
	percent := float64(current) / float64(total) * 100
	bar := strings.Repeat("=", int(percent/2))
	spaces := strings.Repeat(" ", 50-len(bar))
	fmt.Printf("\r[%s%s] %.1f%% (%d/%d bytes)", 
		bar, spaces, percent, current, total)
}