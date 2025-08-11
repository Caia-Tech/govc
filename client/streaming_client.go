package client

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Caia-Tech/govc/api"
	"github.com/gorilla/websocket"
)

// StreamingClient provides high-level streaming operations
type StreamingClient struct {
	client  *api.OptimizedHTTPClient
	baseURL string
	repoID  string
}

// NewStreamingClient creates a new streaming client
func NewStreamingClient(baseURL, repoID string, clientOptions *api.ClientOptions) *StreamingClient {
	return &StreamingClient{
		client:  api.NewOptimizedHTTPClient(clientOptions),
		baseURL: baseURL,
		repoID:  repoID,
	}
}

// StreamReader provides an io.Reader interface for streaming blob content
type StreamReader struct {
	streamID    string
	totalSize   int64
	chunkCount  int
	currentPos  int64
	currentChunk int
	buffer      []byte
	bufferPos   int
	client      *StreamingClient
	ctx         context.Context
	closed      bool
}

// DownloadOptions configures blob download streaming
type DownloadOptions struct {
	ChunkSize    int   `json:"chunk_size,omitempty"`
	StartOffset  int64 `json:"start_offset,omitempty"`
	EndOffset    int64 `json:"end_offset,omitempty"`
	EnableGzip   bool  `json:"enable_gzip,omitempty"`
	VerifyHashes bool  `json:"verify_hashes,omitempty"`
	MaxRetries   int   `json:"max_retries,omitempty"`
}

// UploadOptions configures blob upload streaming
type UploadOptions struct {
	ChunkSize       int  `json:"chunk_size,omitempty"`
	EnableGzip      bool `json:"enable_gzip,omitempty"`
	VerifyUpload    bool `json:"verify_upload,omitempty"`
	ProgressCallback func(bytesUploaded, totalBytes int64)
}

// ProgressCallback is called during streaming operations
type ProgressCallback func(progress *api.StreamProgress)

// NewStreamReader creates a streaming reader for a blob
func (sc *StreamingClient) NewStreamReader(ctx context.Context, hash string, opts *DownloadOptions) (*StreamReader, error) {
	if opts == nil {
		opts = &DownloadOptions{}
	}

	// Start the stream
	streamReq := &api.StreamRequest{
		Hash:        hash,
		ChunkSize:   opts.ChunkSize,
		StartOffset: opts.StartOffset,
		EndOffset:   opts.EndOffset,
		EnableGzip:  opts.EnableGzip,
		StreamID:    fmt.Sprintf("stream_%d_%s", time.Now().Unix(), hash[:8]),
	}

	endpoint := fmt.Sprintf("/api/v1/repos/%s/stream/blob/start", sc.repoID)
	respData, err := sc.client.Post(ctx, endpoint, streamReq)
	if err != nil {
		return nil, fmt.Errorf("failed to start stream: %w", err)
	}

	var streamResp api.StreamResponse
	if err := json.Unmarshal(respData, &streamResp); err != nil {
		return nil, fmt.Errorf("failed to parse stream response: %w", err)
	}

	return &StreamReader{
		streamID:   streamResp.StreamID,
		totalSize:  streamResp.TotalSize,
		chunkCount: streamResp.ChunkCount,
		client:     sc,
		ctx:        ctx,
	}, nil
}

// Read implements io.Reader for streaming blob content
func (sr *StreamReader) Read(p []byte) (n int, err error) {
	if sr.closed {
		return 0, io.EOF
	}

	if sr.currentPos >= sr.totalSize {
		sr.closed = true
		return 0, io.EOF
	}

	// If buffer is empty or exhausted, fetch next chunk
	if len(sr.buffer) == 0 || sr.bufferPos >= len(sr.buffer) {
		if err := sr.fetchNextChunk(); err != nil {
			return 0, err
		}
	}

	// Copy from buffer to output
	bytesToCopy := len(p)
	remainingInBuffer := len(sr.buffer) - sr.bufferPos
	if bytesToCopy > remainingInBuffer {
		bytesToCopy = remainingInBuffer
	}

	copy(p, sr.buffer[sr.bufferPos:sr.bufferPos+bytesToCopy])
	sr.bufferPos += bytesToCopy
	sr.currentPos += int64(bytesToCopy)

	return bytesToCopy, nil
}

// fetchNextChunk retrieves the next chunk from the server
func (sr *StreamReader) fetchNextChunk() error {
	if sr.currentChunk >= sr.chunkCount {
		return io.EOF
	}

	// Build chunk request URL
	endpoint := fmt.Sprintf("/api/v1/repos/%s/stream/blob/%s/chunk", 
		sr.client.repoID, sr.streamID)
	
	params := url.Values{}
	params.Set("chunk", strconv.Itoa(sr.currentChunk))
	params.Set("hash", sr.streamID) // In real implementation, store hash separately
	params.Set("chunk_size", "65536")
	
	fullURL := endpoint + "?" + params.Encode()
	respData, err := sr.client.client.Get(sr.ctx, fullURL)
	if err != nil {
		return fmt.Errorf("failed to fetch chunk %d: %w", sr.currentChunk, err)
	}

	var chunk api.StreamChunk
	if err := json.Unmarshal(respData, &chunk); err != nil {
		return fmt.Errorf("failed to parse chunk response: %w", err)
	}

	// Verify chunk integrity if enabled
	expectedChecksum := fmt.Sprintf("%x", md5.Sum(chunk.Data))
	if chunk.Checksum != expectedChecksum {
		return fmt.Errorf("chunk %d checksum mismatch: expected %s, got %s", 
			sr.currentChunk, expectedChecksum, chunk.Checksum)
	}

	sr.buffer = chunk.Data
	sr.bufferPos = 0
	sr.currentChunk++

	return nil
}

// Close closes the stream reader
func (sr *StreamReader) Close() error {
	sr.closed = true
	// Optionally cancel the stream on the server
	endpoint := fmt.Sprintf("/api/v1/repos/%s/stream/blob/%s", 
		sr.client.repoID, sr.streamID)
	sr.client.client.Delete(sr.ctx, endpoint)
	return nil
}

// GetProgress returns the current streaming progress
func (sr *StreamReader) GetProgress() (*api.StreamProgress, error) {
	endpoint := fmt.Sprintf("/api/v1/repos/%s/stream/blob/%s/progress", 
		sr.client.repoID, sr.streamID)
	
	respData, err := sr.client.client.Get(sr.ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get progress: %w", err)
	}

	var progress api.StreamProgress
	if err := json.Unmarshal(respData, &progress); err != nil {
		return nil, fmt.Errorf("failed to parse progress: %w", err)
	}

	return &progress, nil
}

// StreamingWriter provides streaming upload functionality
type StreamingWriter struct {
	client       *StreamingClient
	buffer       *bytes.Buffer
	writer       *multipart.Writer
	chunkSize    int
	totalWritten int64
	opts         *UploadOptions
	ctx          context.Context
}

// NewStreamingWriter creates a new streaming writer for blob uploads
func (sc *StreamingClient) NewStreamingWriter(ctx context.Context, opts *UploadOptions) *StreamingWriter {
	if opts == nil {
		opts = &UploadOptions{ChunkSize: 64 * 1024}
	}

	buffer := &bytes.Buffer{}
	writer := multipart.NewWriter(buffer)

	return &StreamingWriter{
		client:    sc,
		buffer:    buffer,
		writer:    writer,
		chunkSize: opts.ChunkSize,
		opts:      opts,
		ctx:       ctx,
	}
}

// Write implements io.Writer for streaming uploads
func (sw *StreamingWriter) Write(p []byte) (n int, err error) {
	// Create a form field for this chunk
	chunkNum := int(sw.totalWritten / int64(sw.chunkSize))
	fieldName := fmt.Sprintf("chunk_%d", chunkNum)
	
	part, err := sw.writer.CreateFormField(fieldName)
	if err != nil {
		return 0, fmt.Errorf("failed to create form field: %w", err)
	}

	bytesWritten, err := part.Write(p)
	if err != nil {
		return 0, fmt.Errorf("failed to write chunk: %w", err)
	}

	sw.totalWritten += int64(bytesWritten)

	// Call progress callback if provided
	if sw.opts.ProgressCallback != nil {
		sw.opts.ProgressCallback(sw.totalWritten, sw.totalWritten) // Total unknown during upload
	}

	return bytesWritten, nil
}

// Close completes the streaming upload
func (sw *StreamingWriter) Close() (string, error) {
	if err := sw.writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Send the accumulated chunks
	endpoint := fmt.Sprintf("/api/v1/repos/%s/stream/blob/upload", sw.client.repoID)
	
	req, err := http.NewRequestWithContext(sw.ctx, "POST", 
		sw.client.baseURL+endpoint, sw.buffer)
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Content-Type", sw.writer.FormDataContentType())
	
	// Use the underlying HTTP client
	httpClient := &http.Client{Timeout: 300 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed with status %d: %s", 
			resp.StatusCode, string(body))
	}

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read upload response: %w", err)
	}

	var uploadResp struct {
		Hash   string `json:"hash"`
		Size   int64  `json:"size"`
		Chunks int    `json:"chunks"`
	}
	
	if err := json.Unmarshal(respData, &uploadResp); err != nil {
		return "", fmt.Errorf("failed to parse upload response: %w", err)
	}

	return uploadResp.Hash, nil
}

// WebSocketStream provides WebSocket-based streaming
type WebSocketStream struct {
	conn     *websocket.Conn
	client   *StreamingClient
	streamID string
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewWebSocketStream creates a WebSocket streaming connection
func (sc *StreamingClient) NewWebSocketStream(ctx context.Context) (*WebSocketStream, error) {
	// Build WebSocket URL
	wsURL := sc.baseURL
	if wsURL[:4] == "http" {
		wsURL = "ws" + wsURL[4:] // Convert http(s) to ws(s)
	}
	wsURL += fmt.Sprintf("/api/v1/repos/%s/stream/websocket", sc.repoID)

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 30 * time.Second

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect WebSocket: %w", err)
	}

	streamCtx, cancel := context.WithCancel(ctx)
	
	return &WebSocketStream{
		conn:     conn,
		client:   sc,
		streamID: fmt.Sprintf("ws_stream_%d", time.Now().Unix()),
		ctx:      streamCtx,
		cancel:   cancel,
	}, nil
}

// StreamBlob streams a blob over WebSocket
func (ws *WebSocketStream) StreamBlob(hash string, chunkSize int, callback ProgressCallback) error {
	defer ws.Close()

	streamReq := &api.StreamRequest{
		Hash:      hash,
		ChunkSize: chunkSize,
		StreamID:  ws.streamID,
	}

	// Send stream request
	if err := ws.conn.WriteJSON(streamReq); err != nil {
		return fmt.Errorf("failed to send stream request: %w", err)
	}

	for {
		select {
		case <-ws.ctx.Done():
			return ws.ctx.Err()
		default:
			var message json.RawMessage
			if err := ws.conn.ReadJSON(&message); err != nil {
				return fmt.Errorf("failed to read WebSocket message: %w", err)
			}

			// Try to parse as progress update first
			var progress api.StreamProgress
			if err := json.Unmarshal(message, &progress); err == nil {
				if callback != nil {
					callback(&progress)
				}
				if progress.Status == "completed" {
					return nil
				}
				continue
			}

			// Try to parse as chunk
			var chunk api.StreamChunk
			if err := json.Unmarshal(message, &chunk); err == nil {
				// Process chunk data here
				if chunk.IsLast {
					return nil
				}
				continue
			}

			// Try to parse as error
			var errorResp map[string]string
			if err := json.Unmarshal(message, &errorResp); err == nil {
				if errMsg, exists := errorResp["error"]; exists {
					return fmt.Errorf("server error: %s", errMsg)
				}
			}
		}
	}
}

// Close closes the WebSocket connection
func (ws *WebSocketStream) Close() error {
	if ws.cancel != nil {
		ws.cancel()
	}
	return ws.conn.Close()
}

// DownloadBlob downloads a blob using streaming with progress reporting
func (sc *StreamingClient) DownloadBlob(ctx context.Context, hash string, writer io.Writer, opts *DownloadOptions) error {
	reader, err := sc.NewStreamReader(ctx, hash, opts)
	if err != nil {
		return fmt.Errorf("failed to create stream reader: %w", err)
	}
	defer reader.Close()

	// Copy with progress reporting
	buffer := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			if _, writeErr := writer.Write(buffer[:n]); writeErr != nil {
				return fmt.Errorf("failed to write data: %w", writeErr)
			}
		}
		
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read stream: %w", err)
		}
	}

	return nil
}

// UploadBlob uploads a blob using streaming
func (sc *StreamingClient) UploadBlob(ctx context.Context, reader io.Reader, opts *UploadOptions) (string, error) {
	writer := sc.NewStreamingWriter(ctx, opts)
	
	// Copy data from reader to streaming writer
	buffer := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			if _, writeErr := writer.Write(buffer[:n]); writeErr != nil {
				return "", fmt.Errorf("failed to write chunk: %w", writeErr)
			}
		}
		
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
	}

	// Close and get the final hash
	hash, err := writer.Close()
	if err != nil {
		return "", fmt.Errorf("failed to complete upload: %w", err)
	}

	return hash, nil
}