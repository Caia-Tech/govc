package utils

import (
	"crypto/sha256"
	"fmt"
)

// CalculateHash calculates SHA256 hash of the given data
func CalculateHash(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

// CalculateHashString calculates SHA256 hash of the given data and returns as hex string
func CalculateHashString(data []byte) string {
	hash := CalculateHash(data)
	return fmt.Sprintf("%x", hash)
}