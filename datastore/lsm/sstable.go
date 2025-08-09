package lsm

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SSTable represents a Sorted String Table - immutable on-disk data structure
// Optimized for Git objects with efficient compression and indexing
type SSTable struct {
	path           string
	level          int
	fileNumber     int64
	size           int64
	numEntries     int
	minKey         string
	maxKey         string
	bloomFilter    *BloomFilter
	index          *SSTableIndex
	compression    CompressionType
	createdAt      time.Time
	mu             sync.RWMutex
	file           *os.File
}

// SSTableIndex provides fast key lookup within an SSTable
type SSTableIndex struct {
	entries    []IndexEntry
	blockSize  int
}

// IndexEntry maps a key prefix to a block offset
type IndexEntry struct {
	Key         string
	Offset      int64
	BlockSize   int
}

// SSTableWriter handles creation of new SSTables
type SSTableWriter struct {
	path        string
	file        *os.File
	buffer      *bytes.Buffer
	compression CompressionType
	blockSize   int
	index       *SSTableIndex
	bloom       *BloomFilter
	numEntries  int
	minKey      string
	maxKey      string
	currentBlock *Block
	blockOffset int64
	mu          sync.Mutex
}

// Block represents a compressed block of data within an SSTable
type Block struct {
	entries     []*Entry
	uncompressed []byte
	compressed  []byte
	checksum   uint32
}

// NewSSTableWriter creates a new SSTable writer
func NewSSTableWriter(path string, level int, fileNum int64, config LSMConfig) (*SSTableWriter, error) {
	filename := fmt.Sprintf("L%d_%06d.sst", level, fileNum)
	fullPath := filepath.Join(path, filename)
	
	file, err := os.Create(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSTable file: %w", err)
	}
	
	writer := &SSTableWriter{
		path:        fullPath,
		file:        file,
		buffer:      bytes.NewBuffer(make([]byte, 0, config.BlockSize*2)),
		compression: config.Compression,
		blockSize:   config.BlockSize,
		index:       &SSTableIndex{blockSize: config.BlockSize},
		bloom:       NewBloomFilter(100000, config.BloomFilterBits),
		currentBlock: &Block{entries: make([]*Entry, 0, 100)},
	}
	
	return writer, nil
}

// Add adds a key-value pair to the SSTable being written
func (w *SSTableWriter) Add(key string, value []byte, deleted bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	entry := &Entry{
		Key:       key,
		Value:     make([]byte, len(value)),
		Deleted:   deleted,
		Timestamp: time.Now(),
		Size:      int64(len(key) + len(value) + 32), // Approximate
	}
	copy(entry.Value, value)
	
	// Add to bloom filter
	w.bloom.Add(key)
	
	// Update min/max keys
	if w.numEntries == 0 {
		w.minKey = key
	}
	w.maxKey = key
	
	// Add to current block
	w.currentBlock.entries = append(w.currentBlock.entries, entry)
	w.numEntries++
	
	// Check if block is full
	currentSize := w.currentBlock.estimateSize()
	if currentSize >= w.blockSize {
		if err := w.flushBlock(); err != nil {
			return err
		}
	}
	
	return nil
}

// flushBlock compresses and writes the current block to disk
func (w *SSTableWriter) flushBlock() error {
	if len(w.currentBlock.entries) == 0 {
		return nil
	}
	
	// Serialize block entries
	w.buffer.Reset()
	for _, entry := range w.currentBlock.entries {
		w.serializeEntry(entry)
	}
	w.currentBlock.uncompressed = w.buffer.Bytes()
	
	// Compress block
	compressed, err := w.compressBlock(w.currentBlock.uncompressed)
	if err != nil {
		return fmt.Errorf("failed to compress block: %w", err)
	}
	w.currentBlock.compressed = compressed
	
	// Calculate checksum
	w.currentBlock.checksum = crc32Checksum(w.currentBlock.compressed)
	
	// Add index entry for this block
	if len(w.currentBlock.entries) > 0 {
		indexEntry := IndexEntry{
			Key:       w.currentBlock.entries[0].Key, // First key in block
			Offset:    w.blockOffset,
			BlockSize: len(w.currentBlock.compressed),
		}
		w.index.entries = append(w.index.entries, indexEntry)
	}
	
	// Write block header
	header := make([]byte, 16)
	binary.LittleEndian.PutUint32(header[0:4], uint32(len(w.currentBlock.compressed))) // Compressed size
	binary.LittleEndian.PutUint32(header[4:8], uint32(len(w.currentBlock.uncompressed))) // Uncompressed size
	binary.LittleEndian.PutUint32(header[8:12], w.currentBlock.checksum) // Checksum
	binary.LittleEndian.PutUint32(header[12:16], uint32(w.compression)) // Compression type
	
	if _, err := w.file.Write(header); err != nil {
		return fmt.Errorf("failed to write block header: %w", err)
	}
	
	// Write compressed block
	if _, err := w.file.Write(w.currentBlock.compressed); err != nil {
		return fmt.Errorf("failed to write block data: %w", err)
	}
	
	w.blockOffset += int64(16 + len(w.currentBlock.compressed))
	
	// Reset for next block
	w.currentBlock = &Block{entries: make([]*Entry, 0, 100)}
	
	return nil
}

// serializeEntry serializes an entry to the buffer
func (w *SSTableWriter) serializeEntry(entry *Entry) {
	// Key length (4 bytes) + Key + Value length (4 bytes) + Value + Flags (1 byte) + Timestamp (8 bytes)
	keyLen := uint32(len(entry.Key))
	valueLen := uint32(len(entry.Value))
	
	binary.Write(w.buffer, binary.LittleEndian, keyLen)
	w.buffer.WriteString(entry.Key)
	binary.Write(w.buffer, binary.LittleEndian, valueLen)
	w.buffer.Write(entry.Value)
	
	// Flags: bit 0 = deleted
	flags := uint8(0)
	if entry.Deleted {
		flags |= 1
	}
	w.buffer.WriteByte(flags)
	
	// Timestamp
	binary.Write(w.buffer, binary.LittleEndian, entry.Timestamp.UnixNano())
}

// compressBlock compresses block data based on compression type
func (w *SSTableWriter) compressBlock(data []byte) ([]byte, error) {
	switch w.compression {
	case NoCompression:
		return data, nil
	case Snappy:
		// Would use snappy compression here
		return data, nil // Simplified
	case ZSTD:
		// Would use ZSTD compression here
		return data, nil // Simplified
	case LZ4:
		// Would use LZ4 compression here
		return data, nil // Simplified
	default:
		// Fallback to gzip
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(data); err != nil {
			return nil, err
		}
		if err := gz.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
}

// estimateSize estimates the current block size
func (b *Block) estimateSize() int {
	size := 0
	for _, entry := range b.entries {
		size += len(entry.Key) + len(entry.Value) + 20 // Overhead
	}
	return size
}

// Finish completes the SSTable creation and writes metadata
func (w *SSTableWriter) Finish() (*SSTable, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	// Flush final block
	if err := w.flushBlock(); err != nil {
		return nil, err
	}
	
	// Write index
	indexOffset := w.blockOffset
	if err := w.writeIndex(); err != nil {
		return nil, err
	}
	
	// Write bloom filter
	bloomOffset := w.blockOffset
	if err := w.writeBloomFilter(); err != nil {
		return nil, err
	}
	
	// Write footer
	if err := w.writeFooter(indexOffset, bloomOffset); err != nil {
		return nil, err
	}
	
	// Sync and close file
	if err := w.file.Sync(); err != nil {
		return nil, err
	}
	
	fileSize := w.blockOffset
	if err := w.file.Close(); err != nil {
		return nil, err
	}
	
	// Create SSTable object
	sstable := &SSTable{
		path:        w.path,
		size:        fileSize,
		numEntries:  w.numEntries,
		minKey:      w.minKey,
		maxKey:      w.maxKey,
		bloomFilter: w.bloom,
		index:       w.index,
		compression: w.compression,
		createdAt:   time.Now(),
	}
	
	return sstable, nil
}

// writeIndex writes the block index to the file
func (w *SSTableWriter) writeIndex() error {
	// Serialize index entries
	w.buffer.Reset()
	
	// Number of index entries
	binary.Write(w.buffer, binary.LittleEndian, uint32(len(w.index.entries)))
	
	for _, entry := range w.index.entries {
		keyLen := uint32(len(entry.Key))
		binary.Write(w.buffer, binary.LittleEndian, keyLen)
		w.buffer.WriteString(entry.Key)
		binary.Write(w.buffer, binary.LittleEndian, entry.Offset)
		binary.Write(w.buffer, binary.LittleEndian, uint32(entry.BlockSize))
	}
	
	// Write to file
	if _, err := w.file.Write(w.buffer.Bytes()); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}
	
	w.blockOffset += int64(w.buffer.Len())
	return nil
}

// writeBloomFilter writes the bloom filter to the file
func (w *SSTableWriter) writeBloomFilter() error {
	bloomData := w.bloom.Serialize()
	
	// Write bloom filter size and data
	w.buffer.Reset()
	binary.Write(w.buffer, binary.LittleEndian, uint32(len(bloomData)))
	w.buffer.Write(bloomData)
	
	if _, err := w.file.Write(w.buffer.Bytes()); err != nil {
		return fmt.Errorf("failed to write bloom filter: %w", err)
	}
	
	w.blockOffset += int64(w.buffer.Len())
	return nil
}

// writeFooter writes the file footer with metadata offsets
func (w *SSTableWriter) writeFooter(indexOffset, bloomOffset int64) error {
	w.buffer.Reset()
	
	// Footer format:
	// - Index offset (8 bytes)
	// - Bloom filter offset (8 bytes)  
	// - Number of entries (4 bytes)
	// - Min key length (4 bytes) + Min key
	// - Max key length (4 bytes) + Max key
	// - Compression type (4 bytes)
	// - Creation timestamp (8 bytes)
	// - Magic number (8 bytes)
	
	binary.Write(w.buffer, binary.LittleEndian, indexOffset)
	binary.Write(w.buffer, binary.LittleEndian, bloomOffset)
	binary.Write(w.buffer, binary.LittleEndian, uint32(w.numEntries))
	
	// Min/Max keys
	binary.Write(w.buffer, binary.LittleEndian, uint32(len(w.minKey)))
	w.buffer.WriteString(w.minKey)
	binary.Write(w.buffer, binary.LittleEndian, uint32(len(w.maxKey)))
	w.buffer.WriteString(w.maxKey)
	
	binary.Write(w.buffer, binary.LittleEndian, uint32(w.compression))
	binary.Write(w.buffer, binary.LittleEndian, time.Now().UnixNano())
	binary.Write(w.buffer, binary.LittleEndian, uint64(0xDEADBEEFCAFEBABE)) // Magic number
	
	if _, err := w.file.Write(w.buffer.Bytes()); err != nil {
		return fmt.Errorf("failed to write footer: %w", err)
	}
	
	w.blockOffset += int64(w.buffer.Len())
	return nil
}

// OpenSSTable opens an existing SSTable from disk
func OpenSSTable(path string) (*SSTable, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open SSTable: %w", err)
	}
	
	// Get file size
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat SSTable: %w", err)
	}
	
	// Read footer to get metadata
	footerSize := int64(256) // Conservative estimate
	if stat.Size() < footerSize {
		file.Close()
		return nil, fmt.Errorf("SSTable file too small")
	}
	
	// Seek to potential footer location
	_, err = file.Seek(-footerSize, io.SeekEnd)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to seek to footer: %w", err)
	}
	
	// Read potential footer
	footerData := make([]byte, footerSize)
	_, err = io.ReadFull(file, footerData)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to read footer: %w", err)
	}
	
	// Find magic number to locate actual footer
	magicNumber := uint64(0xDEADBEEFCAFEBABE)
	footerStart := -1
	
	for i := len(footerData) - 8; i >= 0; i-- {
		if binary.LittleEndian.Uint64(footerData[i:i+8]) == magicNumber {
			footerStart = i - 32 // Back up to start of footer
			break
		}
	}
	
	if footerStart < 0 {
		file.Close()
		return nil, fmt.Errorf("invalid SSTable: magic number not found")
	}
	
	// Parse footer
	footer := footerData[footerStart:]
	indexOffset := int64(binary.LittleEndian.Uint64(footer[0:8]))
	bloomOffset := int64(binary.LittleEndian.Uint64(footer[8:16]))
	numEntries := int(binary.LittleEndian.Uint32(footer[16:20]))
	
	// Read min/max keys with bounds checking
	minKeyLen := binary.LittleEndian.Uint32(footer[20:24])
	if 24+minKeyLen > uint32(len(footer)) {
		file.Close()
		return nil, fmt.Errorf("invalid SSTable footer: min key length exceeds footer size")
	}
	minKey := string(footer[24:24+minKeyLen])
	
	if 24+minKeyLen+4 > uint32(len(footer)) {
		file.Close()
		return nil, fmt.Errorf("invalid SSTable footer: max key length position exceeds footer size")
	}
	maxKeyLen := binary.LittleEndian.Uint32(footer[24+minKeyLen:28+minKeyLen])
	
	if 28+minKeyLen+maxKeyLen > uint32(len(footer)) {
		file.Close()
		return nil, fmt.Errorf("invalid SSTable footer: max key exceeds footer size")
	}
	maxKey := string(footer[28+minKeyLen:28+minKeyLen+maxKeyLen])
	
	compressionPos := 28+minKeyLen+maxKeyLen
	if compressionPos+4 > uint32(len(footer)) {
		file.Close()
		return nil, fmt.Errorf("invalid SSTable footer: compression field exceeds footer size")
	}
	compression := CompressionType(binary.LittleEndian.Uint32(footer[compressionPos:compressionPos+4]))
	
	timestampPos := compressionPos+4
	if timestampPos+8 > uint32(len(footer)) {
		file.Close()
		return nil, fmt.Errorf("invalid SSTable footer: timestamp field exceeds footer size")
	}
	createdAt := time.Unix(0, int64(binary.LittleEndian.Uint64(footer[timestampPos:timestampPos+8])))
	
	sstable := &SSTable{
		path:        path,
		size:        stat.Size(),
		numEntries:  numEntries,
		minKey:      minKey,
		maxKey:      maxKey,
		compression: compression,
		createdAt:   createdAt,
		file:        file,
	}
	
	// Load index
	if err := sstable.loadIndex(indexOffset); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to load index: %w", err)
	}
	
	// Load bloom filter
	if err := sstable.loadBloomFilter(bloomOffset); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to load bloom filter: %w", err)
	}
	
	return sstable, nil
}

// loadIndex loads the block index from the file
func (sst *SSTable) loadIndex(offset int64) error {
	sst.mu.Lock()
	defer sst.mu.Unlock()
	
	_, err := sst.file.Seek(offset, io.SeekStart)
	if err != nil {
		return err
	}
	
	// Read number of entries
	var numEntries uint32
	if err := binary.Read(sst.file, binary.LittleEndian, &numEntries); err != nil {
		return err
	}
	
	sst.index = &SSTableIndex{
		entries: make([]IndexEntry, numEntries),
	}
	
	// Read index entries
	for i := uint32(0); i < numEntries; i++ {
		var keyLen uint32
		if err := binary.Read(sst.file, binary.LittleEndian, &keyLen); err != nil {
			return err
		}
		
		keyData := make([]byte, keyLen)
		if _, err := io.ReadFull(sst.file, keyData); err != nil {
			return err
		}
		
		var blockOffset int64
		var blockSize uint32
		if err := binary.Read(sst.file, binary.LittleEndian, &blockOffset); err != nil {
			return err
		}
		if err := binary.Read(sst.file, binary.LittleEndian, &blockSize); err != nil {
			return err
		}
		
		sst.index.entries[i] = IndexEntry{
			Key:       string(keyData),
			Offset:    blockOffset,
			BlockSize: int(blockSize),
		}
	}
	
	return nil
}

// loadBloomFilter loads the bloom filter from the file
func (sst *SSTable) loadBloomFilter(offset int64) error {
	sst.mu.Lock()
	defer sst.mu.Unlock()
	
	_, err := sst.file.Seek(offset, io.SeekStart)
	if err != nil {
		return err
	}
	
	// Read bloom filter size
	var bloomSize uint32
	if err := binary.Read(sst.file, binary.LittleEndian, &bloomSize); err != nil {
		return err
	}
	
	// Read bloom filter data
	bloomData := make([]byte, bloomSize)
	if _, err := io.ReadFull(sst.file, bloomData); err != nil {
		return err
	}
	
	// Deserialize bloom filter
	sst.bloomFilter = DeserializeBloomFilter(bloomData)
	
	return nil
}

// Get retrieves a value by key from the SSTable
func (sst *SSTable) Get(key string) ([]byte, bool, error) {
	// Check bloom filter first (fast negative lookup)
	if !sst.bloomFilter.MayContain(key) {
		return nil, false, nil
	}
	
	// Find the block that might contain the key
	blockIndex := sst.findBlock(key)
	if blockIndex < 0 {
		return nil, false, nil
	}
	
	// Load and search the block
	block, err := sst.loadBlock(blockIndex)
	if err != nil {
		return nil, false, fmt.Errorf("failed to load block: %w", err)
	}
	
	return block.Get(key)
}

// findBlock uses binary search to find the block that might contain the key
func (sst *SSTable) findBlock(key string) int {
	sst.mu.RLock()
	defer sst.mu.RUnlock()
	
	if sst.index == nil || len(sst.index.entries) == 0 {
		return -1
	}
	
	// Binary search for the appropriate block
	left, right := 0, len(sst.index.entries)
	for left < right {
		mid := (left + right) / 2
		if sst.index.entries[mid].Key > key {
			right = mid
		} else {
			left = mid + 1
		}
	}
	
	// Return the previous block (which should contain keys <= target)
	if left > 0 {
		return left - 1
	}
	
	return 0
}

// loadBlock loads a specific block from disk
func (sst *SSTable) loadBlock(blockIndex int) (*Block, error) {
	sst.mu.Lock()
	defer sst.mu.Unlock()
	
	if blockIndex >= len(sst.index.entries) {
		return nil, fmt.Errorf("block index out of range")
	}
	
	entry := sst.index.entries[blockIndex]
	
	// Seek to block position
	_, err := sst.file.Seek(entry.Offset, io.SeekStart)
	if err != nil {
		return nil, err
	}
	
	// Read block header
	header := make([]byte, 16)
	if _, err := io.ReadFull(sst.file, header); err != nil {
		return nil, err
	}
	
	compressedSize := binary.LittleEndian.Uint32(header[0:4])
	uncompressedSize := binary.LittleEndian.Uint32(header[4:8])
	checksum := binary.LittleEndian.Uint32(header[8:12])
	compressionType := CompressionType(binary.LittleEndian.Uint32(header[12:16]))
	
	// Read compressed block data
	compressedData := make([]byte, compressedSize)
	if _, err := io.ReadFull(sst.file, compressedData); err != nil {
		return nil, err
	}
	
	// Verify checksum
	if crc32Checksum(compressedData) != checksum {
		return nil, fmt.Errorf("block checksum mismatch")
	}
	
	// Decompress block
	uncompressedData, err := sst.decompressBlock(compressedData, compressionType)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress block: %w", err)
	}
	
	if len(uncompressedData) != int(uncompressedSize) {
		return nil, fmt.Errorf("decompressed size mismatch")
	}
	
	// Parse block entries
	block := &Block{
		uncompressed: uncompressedData,
		compressed:   compressedData,
		checksum:     checksum,
	}
	
	if err := sst.parseBlockEntries(block); err != nil {
		return nil, fmt.Errorf("failed to parse block entries: %w", err)
	}
	
	return block, nil
}

// decompressBlock decompresses block data
func (sst *SSTable) decompressBlock(data []byte, compression CompressionType) ([]byte, error) {
	switch compression {
	case NoCompression:
		return data, nil
	default:
		// Fallback to gzip
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, gr); err != nil {
			return nil, err
		}
		
		return buf.Bytes(), nil
	}
}

// parseBlockEntries parses entries from uncompressed block data
func (sst *SSTable) parseBlockEntries(block *Block) error {
	data := block.uncompressed
	offset := 0
	
	block.entries = make([]*Entry, 0)
	
	for offset < len(data) {
		// Read key length and key
		if offset+4 > len(data) {
			break
		}
		keyLen := binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4
		
		if offset+int(keyLen) > len(data) {
			break
		}
		key := string(data[offset : offset+int(keyLen)])
		offset += int(keyLen)
		
		// Read value length and value
		if offset+4 > len(data) {
			break
		}
		valueLen := binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4
		
		if offset+int(valueLen) > len(data) {
			break
		}
		value := make([]byte, valueLen)
		copy(value, data[offset:offset+int(valueLen)])
		offset += int(valueLen)
		
		// Read flags
		if offset+1 > len(data) {
			break
		}
		flags := data[offset]
		offset++
		deleted := (flags & 1) != 0
		
		// Read timestamp
		if offset+8 > len(data) {
			break
		}
		timestamp := time.Unix(0, int64(binary.LittleEndian.Uint64(data[offset:offset+8])))
		offset += 8
		
		entry := &Entry{
			Key:       key,
			Value:     value,
			Deleted:   deleted,
			Timestamp: timestamp,
		}
		
		block.entries = append(block.entries, entry)
	}
	
	return nil
}

// Get searches for a key within the block
func (block *Block) Get(key string) ([]byte, bool, error) {
	// Binary search within the block
	left, right := 0, len(block.entries)
	for left < right {
		mid := (left + right) / 2
		if block.entries[mid].Key == key {
			entry := block.entries[mid]
			if entry.Deleted {
				return nil, false, nil
			}
			result := make([]byte, len(entry.Value))
			copy(result, entry.Value)
			return result, true, nil
		} else if block.entries[mid].Key < key {
			left = mid + 1
		} else {
			right = mid
		}
	}
	
	return nil, false, nil
}

// Close closes the SSTable and releases resources
func (sst *SSTable) Close() error {
	sst.mu.Lock()
	defer sst.mu.Unlock()
	
	if sst.file != nil {
		return sst.file.Close()
	}
	return nil
}

// crc32Checksum calculates CRC32 checksum
func crc32Checksum(data []byte) uint32 {
	// Simplified checksum - in production would use proper CRC32
	sum := uint32(0)
	for _, b := range data {
		sum = (sum << 1) ^ uint32(b)
	}
	return sum
}