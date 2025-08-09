package lsm

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
)

// BloomFilter provides fast probabilistic membership testing
// Optimized for Git object hashes with tunable false positive rate
type BloomFilter struct {
	bits      []uint64  // Bit array
	size      uint32    // Number of bits
	hashCount uint32    // Number of hash functions
	numItems  uint32    // Number of items added
}

// NewBloomFilter creates a new Bloom filter
// expectedItems: expected number of items to be added
// bitsPerItem: bits per item (higher = lower false positive rate)
func NewBloomFilter(expectedItems int, bitsPerItem int) *BloomFilter {
	// Calculate optimal bit array size and hash function count
	size := uint32(expectedItems * bitsPerItem)
	hashCount := uint32(float64(bitsPerItem) * 0.693) // ln(2)
	
	if hashCount < 1 {
		hashCount = 1
	}
	if hashCount > 16 {
		hashCount = 16 // Reasonable upper limit
	}
	
	// Round up to next multiple of 64 for efficient storage
	numWords := (size + 63) / 64
	size = numWords * 64
	
	return &BloomFilter{
		bits:      make([]uint64, numWords),
		size:      size,
		hashCount: hashCount,
		numItems:  0,
	}
}

// Add adds an item to the bloom filter
func (bf *BloomFilter) Add(item string) {
	h1, h2 := bf.hash(item)
	
	for i := uint32(0); i < bf.hashCount; i++ {
		// Double hashing: hash_i(x) = hash1(x) + i * hash2(x)
		bitIndex := (h1 + i*h2) % bf.size
		wordIndex := bitIndex / 64
		bitOffset := bitIndex % 64
		
		bf.bits[wordIndex] |= 1 << bitOffset
	}
	
	bf.numItems++
}

// MayContain checks if an item might be in the set
// Returns false if definitely not in set, true if might be in set
func (bf *BloomFilter) MayContain(item string) bool {
	h1, h2 := bf.hash(item)
	
	for i := uint32(0); i < bf.hashCount; i++ {
		bitIndex := (h1 + i*h2) % bf.size
		wordIndex := bitIndex / 64
		bitOffset := bitIndex % 64
		
		if (bf.bits[wordIndex] & (1 << bitOffset)) == 0 {
			return false // Definitely not in set
		}
	}
	
	return true // Might be in set
}

// hash computes two hash values for double hashing
func (bf *BloomFilter) hash(item string) (uint32, uint32) {
	// Use FNV hash family for good distribution
	h1 := fnv.New32a()
	h1.Write([]byte(item))
	hash1 := h1.Sum32()
	
	h2 := fnv.New32()
	h2.Write([]byte(item))
	hash2 := h2.Sum32()
	
	// Ensure hash2 is odd for better distribution in double hashing
	if hash2%2 == 0 {
		hash2++
	}
	
	return hash1, hash2
}

// FalsePositiveRate estimates the current false positive rate
func (bf *BloomFilter) FalsePositiveRate() float64 {
	if bf.numItems == 0 {
		return 0.0
	}
	
	// Calculate the probability that a bit is still 0
	// P(bit is 0) = (1 - 1/m)^(k*n) where m=bits, k=hashes, n=items
	bitsPerItem := float64(bf.size) / float64(bf.numItems)
	prob := 1.0
	for i := 0; i < int(bf.hashCount); i++ {
		prob *= (1.0 - 1.0/bitsPerItem)
	}
	
	// False positive rate = (1 - P(bit is 0))^k
	fpr := 1.0
	for i := 0; i < int(bf.hashCount); i++ {
		fpr *= (1.0 - prob)
	}
	
	return fpr
}

// Stats returns statistics about the bloom filter
type BloomFilterStats struct {
	Size                uint32  // Size in bits
	HashCount          uint32  // Number of hash functions
	NumItems           uint32  // Items added
	FalsePositiveRate  float64 // Estimated false positive rate
	BitsPerItem        float64 // Average bits per item
	MemoryUsage        int64   // Memory usage in bytes
}

// Stats returns statistics about the bloom filter
func (bf *BloomFilter) Stats() BloomFilterStats {
	memoryUsage := int64(len(bf.bits) * 8) // 8 bytes per uint64
	bitsPerItem := 0.0
	if bf.numItems > 0 {
		bitsPerItem = float64(bf.size) / float64(bf.numItems)
	}
	
	return BloomFilterStats{
		Size:              bf.size,
		HashCount:         bf.hashCount,
		NumItems:          bf.numItems,
		FalsePositiveRate: bf.FalsePositiveRate(),
		BitsPerItem:       bitsPerItem,
		MemoryUsage:       memoryUsage,
	}
}

// Clear resets the bloom filter
func (bf *BloomFilter) Clear() {
	for i := range bf.bits {
		bf.bits[i] = 0
	}
	bf.numItems = 0
}

// Union merges another bloom filter into this one
// Both filters must have the same size and hash count
func (bf *BloomFilter) Union(other *BloomFilter) error {
	if bf.size != other.size || bf.hashCount != other.hashCount {
		return fmt.Errorf("bloom filters must have same size and hash count")
	}
	
	for i := range bf.bits {
		bf.bits[i] |= other.bits[i]
	}
	
	// Note: numItems is approximate after union
	bf.numItems += other.numItems
	
	return nil
}

// Serialize converts the bloom filter to bytes for storage
func (bf *BloomFilter) Serialize() []byte {
	// Format: size(4) + hashCount(4) + numItems(4) + bits(8*len(bits))
	bufSize := 12 + len(bf.bits)*8
	buf := make([]byte, bufSize)
	
	binary.LittleEndian.PutUint32(buf[0:4], bf.size)
	binary.LittleEndian.PutUint32(buf[4:8], bf.hashCount)
	binary.LittleEndian.PutUint32(buf[8:12], bf.numItems)
	
	offset := 12
	for _, word := range bf.bits {
		binary.LittleEndian.PutUint64(buf[offset:offset+8], word)
		offset += 8
	}
	
	return buf
}

// DeserializeBloomFilter creates a bloom filter from serialized bytes
func DeserializeBloomFilter(data []byte) *BloomFilter {
	if len(data) < 12 {
		return nil
	}
	
	size := binary.LittleEndian.Uint32(data[0:4])
	hashCount := binary.LittleEndian.Uint32(data[4:8])
	numItems := binary.LittleEndian.Uint32(data[8:12])
	
	numWords := (size + 63) / 64
	expectedSize := 12 + int(numWords)*8
	
	if len(data) != expectedSize {
		return nil
	}
	
	bits := make([]uint64, numWords)
	offset := 12
	for i := range bits {
		bits[i] = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
	}
	
	return &BloomFilter{
		bits:      bits,
		size:      size,
		hashCount: hashCount,
		numItems:  numItems,
	}
}

// OptimalParameters calculates optimal bloom filter parameters
func OptimalParameters(expectedItems int, targetFPR float64) (int, int) {
	// Calculate optimal bit array size
	// m = -n * ln(p) / (ln(2))^2
	if targetFPR <= 0 || targetFPR >= 1 {
		targetFPR = 0.01 // Default to 1% false positive rate
	}
	
	ln2 := 0.693147
	size := int(-float64(expectedItems) * (-ln2) / (ln2 * ln2))
	
	// Calculate optimal number of hash functions
	// k = (m/n) * ln(2)
	hashCount := int(float64(size) / float64(expectedItems) * ln2)
	
	if hashCount < 1 {
		hashCount = 1
	}
	if hashCount > 16 {
		hashCount = 16
	}
	
	bitsPerItem := size / expectedItems
	if bitsPerItem < 1 {
		bitsPerItem = 1
	}
	
	return bitsPerItem, hashCount
}

// BloomFilterBuilder helps construct bloom filters with optimal parameters
type BloomFilterBuilder struct {
	expectedItems int
	targetFPR     float64
	items         []string
}

// NewBloomFilterBuilder creates a new bloom filter builder
func NewBloomFilterBuilder(expectedItems int) *BloomFilterBuilder {
	return &BloomFilterBuilder{
		expectedItems: expectedItems,
		targetFPR:     0.01, // 1% default
		items:         make([]string, 0, expectedItems),
	}
}

// SetTargetFalsePositiveRate sets the target false positive rate
func (bfb *BloomFilterBuilder) SetTargetFalsePositiveRate(fpr float64) *BloomFilterBuilder {
	bfb.targetFPR = fpr
	return bfb
}

// Add adds an item to be included in the bloom filter
func (bfb *BloomFilterBuilder) Add(item string) *BloomFilterBuilder {
	bfb.items = append(bfb.items, item)
	return bfb
}

// Build creates the bloom filter with all added items
func (bfb *BloomFilterBuilder) Build() *BloomFilter {
	bitsPerItem, _ := OptimalParameters(bfb.expectedItems, bfb.targetFPR)
	bf := NewBloomFilter(bfb.expectedItems, bitsPerItem)
	
	for _, item := range bfb.items {
		bf.Add(item)
	}
	
	return bf
}