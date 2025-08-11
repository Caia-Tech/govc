package optimize

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"sync"
)

// StreamProcessor handles streaming data processing with minimal memory usage
type StreamProcessor struct {
	pool      *MemoryPool
	chunkSize int
	workers   int
	
	mu    sync.RWMutex
	stats StreamStats
}

// StreamStats tracks streaming performance
type StreamStats struct {
	BytesProcessed uint64
	ChunksProcessed uint64
	Errors uint64
}

// NewStreamProcessor creates a new stream processor
func NewStreamProcessor(pool *MemoryPool, chunkSize int, workers int) *StreamProcessor {
	if chunkSize == 0 {
		chunkSize = 64 * 1024 // Default 64KB chunks
	}
	if workers == 0 {
		workers = 4
	}
	
	return &StreamProcessor{
		pool:      pool,
		chunkSize: chunkSize,
		workers:   workers,
	}
}

// ProcessFunc is a function that processes a chunk of data
type ProcessFunc func(chunk []byte) ([]byte, error)

// Process streams data through a processing function
func (sp *StreamProcessor) Process(ctx context.Context, reader io.Reader, writer io.Writer, fn ProcessFunc) error {
	// Create worker pool
	type workItem struct {
		chunk []byte
		index int
	}
	
	workChan := make(chan workItem, sp.workers*2)
	resultChan := make(chan struct {
		data  []byte
		index int
		err   error
	}, sp.workers*2)
	
	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < sp.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range workChan {
				result, err := fn(item.chunk)
				resultChan <- struct {
					data  []byte
					index int
					err   error
				}{
					data:  result,
					index: item.index,
					err:   err,
				}
			}
		}()
	}
	
	// Result writer
	errChan := make(chan error, 1)
	go func() {
		results := make(map[int][]byte)
		nextIndex := 0
		
		for result := range resultChan {
			if result.err != nil {
				errChan <- result.err
				return
			}
			
			// Store result
			results[result.index] = result.data
			
			// Write any consecutive results
			for {
				if data, ok := results[nextIndex]; ok {
					if _, err := writer.Write(data); err != nil {
						errChan <- err
						return
					}
					sp.pool.Put(data)
					delete(results, nextIndex)
					nextIndex++
				} else {
					break
				}
			}
		}
		errChan <- nil
	}()
	
	// Read and dispatch chunks
	index := 0
	for {
		select {
		case <-ctx.Done():
			close(workChan)
			return ctx.Err()
		default:
		}
		
		chunk := sp.pool.Get(sp.chunkSize)
		n, err := reader.Read(chunk)
		
		if n > 0 {
			// Send chunk for processing
			workChan <- workItem{
				chunk: chunk[:n],
				index: index,
			}
			index++
			
			sp.mu.Lock()
			sp.stats.BytesProcessed += uint64(n)
			sp.stats.ChunksProcessed++
			sp.mu.Unlock()
		}
		
		if err == io.EOF {
			break
		}
		if err != nil {
			close(workChan)
			return err
		}
	}
	
	// Close work channel and wait for workers
	close(workChan)
	wg.Wait()
	close(resultChan)
	
	// Wait for writer
	return <-errChan
}

// CompressStream compresses data with streaming and minimal memory
func (sp *StreamProcessor) CompressStream(reader io.Reader, writer io.Writer) error {
	gw := gzip.NewWriter(writer)
	defer gw.Close()
	
	buf := sp.pool.Get(sp.chunkSize)
	defer sp.pool.Put(buf)
	
	_, err := io.CopyBuffer(gw, reader, buf)
	return err
}

// DecompressStream decompresses data with streaming and minimal memory
func (sp *StreamProcessor) DecompressStream(reader io.Reader, writer io.Writer) error {
	gr, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}
	defer gr.Close()
	
	buf := sp.pool.Get(sp.chunkSize)
	defer sp.pool.Put(buf)
	
	_, err = io.CopyBuffer(writer, gr, buf)
	return err
}

// ChunkedReader provides a chunked reading interface
type ChunkedReader struct {
	reader    io.Reader
	chunkSize int
	pool      *MemoryPool
}

// NewChunkedReader creates a new chunked reader
func (sp *StreamProcessor) NewChunkedReader(reader io.Reader) *ChunkedReader {
	return &ChunkedReader{
		reader:    reader,
		chunkSize: sp.chunkSize,
		pool:      sp.pool,
	}
}

// ReadChunk reads the next chunk
func (cr *ChunkedReader) ReadChunk() ([]byte, error) {
	buf := cr.pool.Get(cr.chunkSize)
	n, err := cr.reader.Read(buf)
	
	if err != nil && err != io.EOF {
		cr.pool.Put(buf)
		return nil, err
	}
	
	if n == 0 {
		cr.pool.Put(buf)
		return nil, io.EOF
	}
	
	return buf[:n], err
}

// ChunkedWriter provides a chunked writing interface
type ChunkedWriter struct {
	writer    io.Writer
	chunkSize int
	pool      *MemoryPool
	buffer    []byte
	pos       int
}

// NewChunkedWriter creates a new chunked writer
func (sp *StreamProcessor) NewChunkedWriter(writer io.Writer) *ChunkedWriter {
	buf := sp.pool.Get(sp.chunkSize)
	return &ChunkedWriter{
		writer:    writer,
		chunkSize: sp.chunkSize,
		pool:      sp.pool,
		buffer:    buf,
		pos:       0,
	}
}

// Write implements io.Writer
func (cw *ChunkedWriter) Write(p []byte) (int, error) {
	written := 0
	
	for len(p) > 0 {
		// How much can we write to buffer?
		space := len(cw.buffer) - cw.pos
		if space == 0 {
			// Buffer full, flush it
			if err := cw.Flush(); err != nil {
				return written, err
			}
			space = len(cw.buffer)
		}
		
		// Write what we can
		n := len(p)
		if n > space {
			n = space
		}
		
		copy(cw.buffer[cw.pos:], p[:n])
		cw.pos += n
		written += n
		p = p[n:]
	}
	
	return written, nil
}

// Flush writes buffered data to the underlying writer
func (cw *ChunkedWriter) Flush() error {
	if cw.pos == 0 {
		return nil
	}
	
	_, err := cw.writer.Write(cw.buffer[:cw.pos])
	cw.pos = 0
	return err
}

// Close flushes and returns the buffer to the pool
func (cw *ChunkedWriter) Close() error {
	if err := cw.Flush(); err != nil {
		return err
	}
	
	cw.pool.Put(cw.buffer)
	cw.buffer = nil
	return nil
}

// Pipeline creates a processing pipeline
type Pipeline struct {
	stages []ProcessFunc
	sp     *StreamProcessor
}

// NewPipeline creates a new processing pipeline
func (sp *StreamProcessor) NewPipeline(stages ...ProcessFunc) *Pipeline {
	return &Pipeline{
		stages: stages,
		sp:     sp,
	}
}

// Process runs data through the pipeline
func (p *Pipeline) Process(ctx context.Context, reader io.Reader, writer io.Writer) error {
	if len(p.stages) == 0 {
		// No processing, just copy
		buf := p.sp.pool.Get(p.sp.chunkSize)
		defer p.sp.pool.Put(buf)
		_, err := io.CopyBuffer(writer, reader, buf)
		return err
	}
	
	// Create intermediate buffers for pipeline stages
	buffers := make([]*bytes.Buffer, len(p.stages)-1)
	for i := range buffers {
		buffers[i] = new(bytes.Buffer)
	}
	
	// Process through stages
	var currentReader io.Reader = reader
	var currentWriter io.Writer
	
	for i, stage := range p.stages {
		if i == len(p.stages)-1 {
			// Last stage writes to final writer
			currentWriter = writer
		} else {
			// Intermediate stages write to buffers
			currentWriter = buffers[i]
		}
		
		err := p.sp.Process(ctx, currentReader, currentWriter, stage)
		if err != nil {
			return err
		}
		
		// Next stage reads from this buffer
		if i < len(p.stages)-1 {
			currentReader = buffers[i]
		}
	}
	
	return nil
}

// Stats returns current statistics
func (sp *StreamProcessor) Stats() StreamStats {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return sp.stats
}