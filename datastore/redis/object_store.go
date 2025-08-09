package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/caiatech/govc/datastore"
)

// ObjectStore implements Redis-based object storage
type ObjectStore struct {
	client redis.UniversalClient
	config datastore.Config
	prefix string
}

// NewObjectStore creates a new Redis object store
func NewObjectStore(client redis.UniversalClient, config datastore.Config) *ObjectStore {
	prefix := "govc:objects:"

	return &ObjectStore{
		client: client,
		config: config,
		prefix: prefix,
	}
}

// GetObject retrieves an object by hash
func (s *ObjectStore) GetObject(hash string) ([]byte, error) {
	if hash == "" {
		return nil, datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := s.prefix + hash
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, datastore.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get object %s: %w", hash, err)
	}

	return data, nil
}

// PutObject stores an object
func (s *ObjectStore) PutObject(hash string, data []byte) error {
	if hash == "" {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := s.prefix + hash
	
	// Set with optional TTL if configured
	var expiration time.Duration
	if ttl := s.config.GetDurationOption("ttl", 0); ttl > 0 {
		expiration = ttl
	}

	err := s.client.Set(ctx, key, data, expiration).Err()
	if err != nil {
		return fmt.Errorf("failed to store object %s: %w", hash, err)
	}

	return nil
}

// DeleteObject removes an object
func (s *ObjectStore) DeleteObject(hash string) error {
	if hash == "" {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := s.prefix + hash
	deleted, err := s.client.Del(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to delete object %s: %w", hash, err)
	}

	if deleted == 0 {
		return datastore.ErrNotFound
	}

	return nil
}

// HasObject checks if object exists
func (s *ObjectStore) HasObject(hash string) (bool, error) {
	if hash == "" {
		return false, datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := s.prefix + hash
	exists, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check object existence %s: %w", hash, err)
	}

	return exists > 0, nil
}

// ListObjects lists objects matching a prefix
func (s *ObjectStore) ListObjects(prefix string, limit int) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pattern := s.prefix + prefix + "*"
	var hashes []string

	iter := s.client.Scan(ctx, 0, pattern, 0).Iterator()
	count := 0
	
	for iter.Next(ctx) {
		if limit > 0 && count >= limit {
			break
		}

		key := iter.Val()
		// Remove prefix to get hash
		hash := strings.TrimPrefix(key, s.prefix)
		hashes = append(hashes, hash)
		count++
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	return hashes, nil
}

// IterateObjects iterates over objects matching a prefix
func (s *ObjectStore) IterateObjects(prefix string, fn func(hash string, data []byte) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pattern := s.prefix + prefix + "*"
	
	iter := s.client.Scan(ctx, 0, pattern, 0).Iterator()
	
	for iter.Next(ctx) {
		key := iter.Val()
		hash := strings.TrimPrefix(key, s.prefix)

		// Get the data
		data, err := s.client.Get(ctx, key).Bytes()
		if err != nil {
			if err == redis.Nil {
				continue // Key was deleted between scan and get
			}
			return fmt.Errorf("failed to get object data for %s: %w", hash, err)
		}

		if err := fn(hash, data); err != nil {
			return err
		}
	}

	return iter.Err()
}

// GetObjects retrieves multiple objects
func (s *ObjectStore) GetObjects(hashes []string) (map[string][]byte, error) {
	if len(hashes) == 0 {
		return make(map[string][]byte), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build keys
	keys := make([]string, len(hashes))
	for i, hash := range hashes {
		if hash == "" {
			continue
		}
		keys[i] = s.prefix + hash
	}

	// Use MGET for batch retrieval
	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get multiple objects: %w", err)
	}

	result := make(map[string][]byte)
	for i, value := range values {
		if value != nil && hashes[i] != "" {
			if data, ok := value.(string); ok {
				result[hashes[i]] = []byte(data)
			}
		}
	}

	return result, nil
}

// PutObjects stores multiple objects
func (s *ObjectStore) PutObjects(objects map[string][]byte) error {
	if len(objects) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pipe := s.client.Pipeline()

	var expiration time.Duration
	if ttl := s.config.GetDurationOption("ttl", 0); ttl > 0 {
		expiration = ttl
	}

	for hash, data := range objects {
		if hash == "" {
			continue
		}
		key := s.prefix + hash
		pipe.Set(ctx, key, data, expiration)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to store multiple objects: %w", err)
	}

	return nil
}

// DeleteObjects removes multiple objects
func (s *ObjectStore) DeleteObjects(hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	keys := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		if hash != "" {
			keys = append(keys, s.prefix+hash)
		}
	}

	if len(keys) > 0 {
		_, err := s.client.Del(ctx, keys...).Result()
		if err != nil {
			return fmt.Errorf("failed to delete multiple objects: %w", err)
		}
	}

	return nil
}

// GetObjectSize returns the size of an object
func (s *ObjectStore) GetObjectSize(hash string) (int64, error) {
	if hash == "" {
		return 0, datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := s.prefix + hash
	size, err := s.client.StrLen(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, datastore.ErrNotFound
		}
		return 0, fmt.Errorf("failed to get object size %s: %w", hash, err)
	}

	if size == 0 {
		// Check if key exists
		exists, err := s.client.Exists(ctx, key).Result()
		if err != nil {
			return 0, fmt.Errorf("failed to check object existence %s: %w", hash, err)
		}
		if exists == 0 {
			return 0, datastore.ErrNotFound
		}
	}

	return size, nil
}

// CountObjects returns the total number of objects
func (s *ObjectStore) CountObjects() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pattern := s.prefix + "*"
	var count int64

	iter := s.client.Scan(ctx, 0, pattern, 0).Iterator()
	
	for iter.Next(ctx) {
		count++
	}

	if err := iter.Err(); err != nil {
		return 0, fmt.Errorf("failed to count objects: %w", err)
	}

	return count, nil
}

// GetStorageSize returns the total storage used (approximate)
func (s *ObjectStore) GetStorageSize() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pattern := s.prefix + "*"
	var totalSize int64

	iter := s.client.Scan(ctx, 0, pattern, 0).Iterator()
	
	for iter.Next(ctx) {
		key := iter.Val()
		size, err := s.client.StrLen(ctx, key).Result()
		if err != nil {
			// Skip errors, continue counting
			continue
		}
		totalSize += size
	}

	if err := iter.Err(); err != nil {
		return 0, fmt.Errorf("failed to calculate storage size: %w", err)
	}

	return totalSize, nil
}