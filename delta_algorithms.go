package govc

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Implementation of VCDIFF-based delta compression algorithm
// This is optimized for high-frequency application data with many similar objects

// VCDIFFDelta implements VCDIFF-inspired delta compression
func NewVCDIFFDelta(windowSize, minMatchSize int) *VCDIFFDelta {
	return &VCDIFFDelta{
		windowSize:   windowSize,
		minMatchSize: minMatchSize,
	}
}

// CreateDelta creates a delta from base to target using VCDIFF-inspired algorithm
func (vd *VCDIFFDelta) CreateDelta(baseData, targetData []byte) ([]byte, error) {
	if len(baseData) == 0 {
		// No base, return full data with indicator
		delta := &DeltaPacket{
			Type:    DeltaTypeFull,
			Length:  len(targetData),
			Data:    targetData,
		}
		return delta.Serialize(), nil
	}
	
	if bytes.Equal(baseData, targetData) {
		// Identical data, return empty delta
		delta := &DeltaPacket{
			Type:   DeltaTypeIdentical,
			Length: 0,
			Data:   nil,
		}
		return delta.Serialize(), nil
	}
	
	// Create rolling hash index of base data
	hashIndex := vd.createHashIndex(baseData)
	
	// Generate delta instructions
	instructions := make([]*DeltaInstruction, 0)
	targetPos := 0
	
	for targetPos < len(targetData) {
		// Try to find a match in base data
		match := vd.findBestMatch(baseData, targetData, targetPos, hashIndex)
		
		if match != nil && match.Length >= vd.minMatchSize {
			// Found a good match, create copy instruction
			if match.TargetStart > targetPos {
				// Add literal data before the match
				literalData := targetData[targetPos:match.TargetStart]
				instructions = append(instructions, &DeltaInstruction{
					Type:   InstructionTypeAdd,
					Length: len(literalData),
					Data:   literalData,
				})
			}
			
			// Add copy instruction
			instructions = append(instructions, &DeltaInstruction{
				Type:   InstructionTypeCopy,
				Offset: match.BaseStart,
				Length: match.Length,
			})
			
			targetPos = match.TargetStart + match.Length
		} else {
			// No good match, collect literal data
			literalStart := targetPos
			targetPos++
			
			// Collect consecutive unmatched bytes
			for targetPos < len(targetData) {
				nextMatch := vd.findBestMatch(baseData, targetData, targetPos, hashIndex)
				if nextMatch != nil && nextMatch.Length >= vd.minMatchSize {
					break
				}
				targetPos++
			}
			
			// Add literal instruction
			literalData := targetData[literalStart:targetPos]
			instructions = append(instructions, &DeltaInstruction{
				Type:   InstructionTypeAdd,
				Length: len(literalData),
				Data:   literalData,
			})
		}
	}
	
	// Serialize delta packet
	delta := &DeltaPacket{
		Type:         DeltaTypeDelta,
		BaseSize:     len(baseData),
		TargetSize:   len(targetData),
		Instructions: instructions,
	}
	
	return delta.Serialize(), nil
}

// ApplyDelta applies delta to base data to reconstruct target
func (vd *VCDIFFDelta) ApplyDelta(baseData, deltaData []byte) ([]byte, error) {
	// Parse delta packet
	delta, err := ParseDeltaPacket(deltaData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse delta: %w", err)
	}
	
	switch delta.Type {
	case DeltaTypeFull:
		return delta.Data, nil
		
	case DeltaTypeIdentical:
		return baseData, nil
		
	case DeltaTypeDelta:
		return vd.applyDeltaInstructions(baseData, delta)
		
	default:
		return nil, fmt.Errorf("unknown delta type: %d", delta.Type)
	}
}

// CompressionRatio calculates the compression ratio
func (vd *VCDIFFDelta) CompressionRatio(baseData, targetData, deltaData []byte) float64 {
	if len(targetData) == 0 {
		return 0.0
	}
	return float64(len(deltaData)) / float64(len(targetData))
}

// createHashIndex creates a hash index for fast matching
func (vd *VCDIFFDelta) createHashIndex(data []byte) map[uint32][]int {
	index := make(map[uint32][]int)
	
	for i := 0; i <= len(data)-vd.minMatchSize; i++ {
		hash := vd.rollingHash(data[i:i+vd.minMatchSize])
		index[hash] = append(index[hash], i)
	}
	
	return index
}

// findBestMatch finds the best match for target data at given position
func (vd *VCDIFFDelta) findBestMatch(baseData, targetData []byte, targetPos int, hashIndex map[uint32][]int) *DeltaMatch {
	if targetPos+vd.minMatchSize > len(targetData) {
		return nil
	}
	
	// Calculate hash for target position
	targetHash := vd.rollingHash(targetData[targetPos:targetPos+vd.minMatchSize])
	
	// Find candidate matches
	candidates, exists := hashIndex[targetHash]
	if !exists {
		return nil
	}
	
	bestMatch := &DeltaMatch{
		BaseStart:   -1,
		TargetStart: targetPos,
		Length:      0,
	}
	
	// Check each candidate
	for _, basePos := range candidates {
		// Extend match as far as possible
		matchLen := 0
		maxLen := len(targetData) - targetPos
		if len(baseData) - basePos < maxLen {
			maxLen = len(baseData) - basePos
		}
		
		for matchLen < maxLen && baseData[basePos+matchLen] == targetData[targetPos+matchLen] {
			matchLen++
		}
		
		// Keep best match
		if matchLen > bestMatch.Length {
			bestMatch.BaseStart = basePos
			bestMatch.Length = matchLen
		}
	}
	
	if bestMatch.Length < vd.minMatchSize {
		return nil
	}
	
	return bestMatch
}

// applyDeltaInstructions applies delta instructions to reconstruct target
func (vd *VCDIFFDelta) applyDeltaInstructions(baseData []byte, delta *DeltaPacket) ([]byte, error) {
	result := make([]byte, 0, delta.TargetSize)
	
	for _, instruction := range delta.Instructions {
		switch instruction.Type {
		case InstructionTypeAdd:
			result = append(result, instruction.Data...)
			
		case InstructionTypeCopy:
			if instruction.Offset+instruction.Length > len(baseData) {
				return nil, fmt.Errorf("copy instruction out of bounds: offset=%d, length=%d, base_size=%d", 
					instruction.Offset, instruction.Length, len(baseData))
			}
			result = append(result, baseData[instruction.Offset:instruction.Offset+instruction.Length]...)
			
		default:
			return nil, fmt.Errorf("unknown instruction type: %d", instruction.Type)
		}
	}
	
	if len(result) != delta.TargetSize {
		return nil, fmt.Errorf("reconstructed size mismatch: expected %d, got %d", 
			delta.TargetSize, len(result))
	}
	
	return result, nil
}

// rollingHash computes a simple rolling hash
func (vd *VCDIFFDelta) rollingHash(data []byte) uint32 {
	var hash uint32
	for i, b := range data {
		hash = hash*31 + uint32(b) + uint32(i)
	}
	return hash
}

// BinaryDelta implements simple binary delta compression
func NewBinaryDelta(blockSize int) *BinaryDelta {
	return &BinaryDelta{
		blockSize: blockSize,
	}
}

// CreateDelta creates a simple binary delta
func (bd *BinaryDelta) CreateDelta(baseData, targetData []byte) ([]byte, error) {
	if bytes.Equal(baseData, targetData) {
		// Identical
		return (&DeltaPacket{
			Type:   DeltaTypeIdentical,
			Length: 0,
		}).Serialize(), nil
	}
	
	// Simple block-based comparison
	instructions := make([]*DeltaInstruction, 0)
	targetPos := 0
	
	for targetPos < len(targetData) {
		blockEnd := targetPos + bd.blockSize
		if blockEnd > len(targetData) {
			blockEnd = len(targetData)
		}
		
		targetBlock := targetData[targetPos:blockEnd]
		
		// Look for matching block in base
		basePos := bd.findBlockMatch(baseData, targetBlock)
		
		if basePos >= 0 {
			// Found match, create copy instruction
			instructions = append(instructions, &DeltaInstruction{
				Type:   InstructionTypeCopy,
				Offset: basePos,
				Length: len(targetBlock),
			})
		} else {
			// No match, create add instruction
			instructions = append(instructions, &DeltaInstruction{
				Type:   InstructionTypeAdd,
				Length: len(targetBlock),
				Data:   targetBlock,
			})
		}
		
		targetPos = blockEnd
	}
	
	delta := &DeltaPacket{
		Type:         DeltaTypeDelta,
		BaseSize:     len(baseData),
		TargetSize:   len(targetData),
		Instructions: instructions,
	}
	
	return delta.Serialize(), nil
}

// ApplyDelta applies binary delta
func (bd *BinaryDelta) ApplyDelta(baseData, deltaData []byte) ([]byte, error) {
	// Use same logic as VCDIFF
	vcdiff := &VCDIFFDelta{}
	return vcdiff.ApplyDelta(baseData, deltaData)
}

// CompressionRatio calculates compression ratio
func (bd *BinaryDelta) CompressionRatio(baseData, targetData, deltaData []byte) float64 {
	if len(targetData) == 0 {
		return 0.0
	}
	return float64(len(deltaData)) / float64(len(targetData))
}

// findBlockMatch finds a matching block in base data
func (bd *BinaryDelta) findBlockMatch(baseData, targetBlock []byte) int {
	for i := 0; i <= len(baseData)-len(targetBlock); i++ {
		if bytes.Equal(baseData[i:i+len(targetBlock)], targetBlock) {
			return i
		}
	}
	return -1
}

// Delta packet structure and serialization

// DeltaType defines types of delta packets
type DeltaType byte

const (
	DeltaTypeFull      DeltaType = 0x00 // Full data (no compression)
	DeltaTypeIdentical DeltaType = 0x01 // Identical to base
	DeltaTypeDelta     DeltaType = 0x02 // Delta compression
)

// InstructionType defines delta instruction types
type InstructionType byte

const (
	InstructionTypeAdd  InstructionType = 0x01 // Add literal data
	InstructionTypeCopy InstructionType = 0x02 // Copy from base
)

// DeltaPacket represents a delta packet
type DeltaPacket struct {
	Type         DeltaType            `json:"type"`
	BaseSize     int                  `json:"base_size,omitempty"`
	TargetSize   int                  `json:"target_size,omitempty"`
	Length       int                  `json:"length,omitempty"`        // For full type
	Data         []byte               `json:"data,omitempty"`          // For full type
	Instructions []*DeltaInstruction  `json:"instructions,omitempty"`  // For delta type
}

// DeltaInstruction represents a delta instruction
type DeltaInstruction struct {
	Type   InstructionType `json:"type"`
	Offset int             `json:"offset,omitempty"` // For copy instructions
	Length int             `json:"length"`
	Data   []byte          `json:"data,omitempty"`   // For add instructions
}

// DeltaMatch represents a match found during delta creation
type DeltaMatch struct {
	BaseStart   int `json:"base_start"`
	TargetStart int `json:"target_start"`
	Length      int `json:"length"`
}

// Serialize converts delta packet to binary format
func (dp *DeltaPacket) Serialize() []byte {
	buf := bytes.NewBuffer(nil)
	
	// Write type
	buf.WriteByte(byte(dp.Type))
	
	switch dp.Type {
	case DeltaTypeFull:
		// Write length and data
		binary.Write(buf, binary.LittleEndian, uint32(dp.Length))
		buf.Write(dp.Data)
		
	case DeltaTypeIdentical:
		// No additional data needed
		
	case DeltaTypeDelta:
		// Write sizes
		binary.Write(buf, binary.LittleEndian, uint32(dp.BaseSize))
		binary.Write(buf, binary.LittleEndian, uint32(dp.TargetSize))
		
		// Write number of instructions
		binary.Write(buf, binary.LittleEndian, uint32(len(dp.Instructions)))
		
		// Write instructions
		for _, inst := range dp.Instructions {
			buf.WriteByte(byte(inst.Type))
			
			switch inst.Type {
			case InstructionTypeAdd:
				binary.Write(buf, binary.LittleEndian, uint32(inst.Length))
				buf.Write(inst.Data)
				
			case InstructionTypeCopy:
				binary.Write(buf, binary.LittleEndian, uint32(inst.Offset))
				binary.Write(buf, binary.LittleEndian, uint32(inst.Length))
			}
		}
	}
	
	return buf.Bytes()
}

// ParseDeltaPacket parses binary delta packet
func ParseDeltaPacket(data []byte) (*DeltaPacket, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty delta data")
	}
	
	buf := bytes.NewBuffer(data)
	
	// Read type
	typeByte, err := buf.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("failed to read type: %w", err)
	}
	
	delta := &DeltaPacket{
		Type: DeltaType(typeByte),
	}
	
	switch delta.Type {
	case DeltaTypeFull:
		// Read length
		var length uint32
		if err := binary.Read(buf, binary.LittleEndian, &length); err != nil {
			return nil, fmt.Errorf("failed to read length: %w", err)
		}
		delta.Length = int(length)
		
		// Read data
		delta.Data = make([]byte, length)
		if _, err := buf.Read(delta.Data); err != nil {
			return nil, fmt.Errorf("failed to read data: %w", err)
		}
		
	case DeltaTypeIdentical:
		// No additional data
		
	case DeltaTypeDelta:
		// Read sizes
		var baseSize, targetSize uint32
		if err := binary.Read(buf, binary.LittleEndian, &baseSize); err != nil {
			return nil, fmt.Errorf("failed to read base size: %w", err)
		}
		if err := binary.Read(buf, binary.LittleEndian, &targetSize); err != nil {
			return nil, fmt.Errorf("failed to read target size: %w", err)
		}
		
		delta.BaseSize = int(baseSize)
		delta.TargetSize = int(targetSize)
		
		// Read number of instructions
		var numInstructions uint32
		if err := binary.Read(buf, binary.LittleEndian, &numInstructions); err != nil {
			return nil, fmt.Errorf("failed to read instruction count: %w", err)
		}
		
		// Read instructions
		delta.Instructions = make([]*DeltaInstruction, numInstructions)
		for i := 0; i < int(numInstructions); i++ {
			instTypeByte, err := buf.ReadByte()
			if err != nil {
				return nil, fmt.Errorf("failed to read instruction type: %w", err)
			}
			
			inst := &DeltaInstruction{
				Type: InstructionType(instTypeByte),
			}
			
			switch inst.Type {
			case InstructionTypeAdd:
				var length uint32
				if err := binary.Read(buf, binary.LittleEndian, &length); err != nil {
					return nil, fmt.Errorf("failed to read add length: %w", err)
				}
				inst.Length = int(length)
				
				inst.Data = make([]byte, length)
				if _, err := buf.Read(inst.Data); err != nil {
					return nil, fmt.Errorf("failed to read add data: %w", err)
				}
				
			case InstructionTypeCopy:
				var offset, length uint32
				if err := binary.Read(buf, binary.LittleEndian, &offset); err != nil {
					return nil, fmt.Errorf("failed to read copy offset: %w", err)
				}
				if err := binary.Read(buf, binary.LittleEndian, &length); err != nil {
					return nil, fmt.Errorf("failed to read copy length: %w", err)
				}
				
				inst.Offset = int(offset)
				inst.Length = int(length)
				
			default:
				return nil, fmt.Errorf("unknown instruction type: %d", inst.Type)
			}
			
			delta.Instructions[i] = inst
		}
		
	default:
		return nil, fmt.Errorf("unknown delta type: %d", delta.Type)
	}
	
	return delta, nil
}