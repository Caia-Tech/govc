package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/gridfs"

	"github.com/caiatech/govc/datastore"
)

// ObjectStore implements MongoDB-based object storage using GridFS for large objects
// and regular collections for smaller ones
type ObjectStore struct {
	database   *mongo.Database
	config     datastore.Config
	objects    *mongo.Collection
	gridfsBucket *gridfs.Bucket
	useGridFS  bool
}

// ObjectDocument represents a document in the objects collection
type ObjectDocument struct {
	Hash      string    `bson:"_id"`
	Data      []byte    `bson:"data"`
	Size      int64     `bson:"size"`
	CreatedAt time.Time `bson:"created_at"`
}

// NewObjectStore creates a new MongoDB object store
func NewObjectStore(database *mongo.Database, config datastore.Config) *ObjectStore {
	useGridFS := config.GetBoolOption("use_gridfs", true)
	
	store := &ObjectStore{
		database:  database,
		config:    config,
		objects:   database.Collection("objects"),
		useGridFS: useGridFS,
	}

	if useGridFS {
		store.gridfsBucket, _ = gridfs.NewBucket(database, options.GridFSBucket().SetName("objects_fs"))
	}

	return store
}

// GetObject retrieves an object by hash
func (s *ObjectStore) GetObject(hash string) ([]byte, error) {
	if hash == "" {
		return nil, datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try regular collection first for smaller objects
	var doc ObjectDocument
	err := s.objects.FindOne(ctx, bson.M{"_id": hash}).Decode(&doc)
	if err == nil {
		return doc.Data, nil
	}

	// If not found in objects collection and GridFS is enabled, try GridFS
	if s.useGridFS && s.gridfsBucket != nil && err == mongo.ErrNoDocuments {
		downloadStream, err := s.gridfsBucket.OpenDownloadStreamByName(hash)
		if err != nil {
			if err == gridfs.ErrFileNotFound {
				return nil, datastore.ErrNotFound
			}
			return nil, fmt.Errorf("failed to open GridFS download stream for %s: %w", hash, err)
		}
		defer downloadStream.Close()

		// Read all data from GridFS
		var data []byte
		buf := make([]byte, 4096)
		for {
			n, readErr := downloadStream.Read(buf)
			if n > 0 {
				data = append(data, buf[:n]...)
			}
			if readErr != nil {
				break
			}
		}

		return data, nil
	}

	if err == mongo.ErrNoDocuments {
		return nil, datastore.ErrNotFound
	}

	return nil, fmt.Errorf("failed to get object %s: %w", hash, err)
}

// PutObject stores an object
func (s *ObjectStore) PutObject(hash string, data []byte) error {
	if hash == "" {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	size := int64(len(data))
	maxSizeForCollection := int64(16 * 1024 * 1024) // 16MB MongoDB document limit

	// Use GridFS for large objects if enabled
	if s.useGridFS && s.gridfsBucket != nil && size > maxSizeForCollection {
		uploadStream, err := s.gridfsBucket.OpenUploadStreamWithID(hash, hash)
		if err != nil {
			return fmt.Errorf("failed to open GridFS upload stream for %s: %w", hash, err)
		}
		defer uploadStream.Close()

		_, err = uploadStream.Write(data)
		if err != nil {
			return fmt.Errorf("failed to write to GridFS for %s: %w", hash, err)
		}

		return nil
	}

	// Store in regular collection
	doc := ObjectDocument{
		Hash:      hash,
		Data:      data,
		Size:      size,
		CreatedAt: time.Now(),
	}

	_, err := s.objects.ReplaceOne(ctx, bson.M{"_id": hash}, doc, options.Replace().SetUpsert(true))
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to delete from regular collection
	result, err := s.objects.DeleteOne(ctx, bson.M{"_id": hash})
	if err != nil {
		return fmt.Errorf("failed to delete object %s: %w", hash, err)
	}

	// If found in regular collection, we're done
	if result.DeletedCount > 0 {
		return nil
	}

	// Try GridFS if enabled
	if s.useGridFS && s.gridfsBucket != nil {
		err = s.gridfsBucket.Delete(hash)
		if err != nil {
			if err == gridfs.ErrFileNotFound {
				return datastore.ErrNotFound
			}
			return fmt.Errorf("failed to delete object from GridFS %s: %w", hash, err)
		}
		return nil
	}

	return datastore.ErrNotFound
}

// HasObject checks if object exists
func (s *ObjectStore) HasObject(hash string) (bool, error) {
	if hash == "" {
		return false, datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check regular collection
	count, err := s.objects.CountDocuments(ctx, bson.M{"_id": hash})
	if err != nil {
		return false, fmt.Errorf("failed to check object existence %s: %w", hash, err)
	}

	if count > 0 {
		return true, nil
	}

	// Check GridFS if enabled
	if s.useGridFS && s.gridfsBucket != nil {
		cursor, err := s.gridfsBucket.Find(bson.M{"filename": hash})
		if err != nil {
			return false, fmt.Errorf("failed to find GridFS object %s: %w", hash, err)
		}
		defer cursor.Close(ctx)
		hasNext := cursor.Next(ctx)
		if hasNext {
			return true, nil
		}
	}

	return false, nil
}

// ListObjects lists objects matching a prefix
func (s *ObjectStore) ListObjects(prefix string, limit int) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var hashes []string

	// Query regular collection
	filter := bson.M{}
	if prefix != "" {
		filter["_id"] = bson.M{"$regex": "^" + prefix}
	}

	findOptions := options.Find().SetProjection(bson.M{"_id": 1})
	if limit > 0 {
		findOptions.SetLimit(int64(limit))
	}

	cursor, err := s.objects.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var doc struct {
			ID string `bson:"_id"`
		}
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		hashes = append(hashes, doc.ID)
		
		if limit > 0 && len(hashes) >= limit {
			break
		}
	}

	// Query GridFS if enabled and we haven't reached the limit
	if s.useGridFS && s.gridfsBucket != nil && (limit == 0 || len(hashes) < limit) {
		gridfsFilter := bson.M{}
		if prefix != "" {
			gridfsFilter["filename"] = bson.M{"$regex": "^" + prefix}
		}

		// gridfsLimit := int64(0)
		// if limit > 0 {
		//	gridfsLimit = int64(limit - len(hashes))
		// }

		gfsCursor, err := s.gridfsBucket.Find(gridfsFilter)
		if err == nil {
			defer gfsCursor.Close(ctx)

			for gfsCursor.Next(ctx) {
				var file gridfs.File
				if err := gfsCursor.Decode(&file); err != nil {
					continue
				}
				hashes = append(hashes, file.Name)
				
				if limit > 0 && len(hashes) >= limit {
					break
				}
			}
		}
	}

	return hashes, nil
}

// IterateObjects iterates over objects matching a prefix
func (s *ObjectStore) IterateObjects(prefix string, fn func(hash string, data []byte) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour) // Long timeout for iteration
	defer cancel()

	// Iterate regular collection
	filter := bson.M{}
	if prefix != "" {
		filter["_id"] = bson.M{"$regex": "^" + prefix}
	}

	cursor, err := s.objects.Find(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to iterate objects: %w", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var doc ObjectDocument
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		if err := fn(doc.Hash, doc.Data); err != nil {
			return err
		}
	}

	// Iterate GridFS if enabled
	if s.useGridFS && s.gridfsBucket != nil {
		gridfsFilter := bson.M{}
		if prefix != "" {
			gridfsFilter["filename"] = bson.M{"$regex": "^" + prefix}
		}

		gfsCursor, err := s.gridfsBucket.Find(gridfsFilter)
		if err == nil {
			defer gfsCursor.Close(ctx)

			for gfsCursor.Next(ctx) {
				var file gridfs.File
				if err := gfsCursor.Decode(&file); err != nil {
					continue
				}

				// Download the file data
				data, err := s.GetObject(file.Name)
				if err != nil {
					continue
				}

				if err := fn(file.Name, data); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// GetObjects retrieves multiple objects
func (s *ObjectStore) GetObjects(hashes []string) (map[string][]byte, error) {
	if len(hashes) == 0 {
		return make(map[string][]byte), nil
	}

	result := make(map[string][]byte)

	for _, hash := range hashes {
		if hash == "" {
			continue
		}

		data, err := s.GetObject(hash)
		if err == nil {
			result[hash] = data
		}
		// Continue with other hashes even if one fails
	}

	return result, nil
}

// PutObjects stores multiple objects
func (s *ObjectStore) PutObjects(objects map[string][]byte) error {
	if len(objects) == 0 {
		return nil
	}

	// Process in batches for better performance
	for hash, data := range objects {
		if hash == "" {
			continue
		}

		if err := s.PutObject(hash, data); err != nil {
			return fmt.Errorf("failed to store object %s: %w", hash, err)
		}
	}

	return nil
}

// DeleteObjects removes multiple objects
func (s *ObjectStore) DeleteObjects(hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Delete from regular collection
	filter := bson.M{"_id": bson.M{"$in": hashes}}
	_, err := s.objects.DeleteMany(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete objects from collection: %w", err)
	}

	// Delete from GridFS if enabled
	if s.useGridFS && s.gridfsBucket != nil {
		for _, hash := range hashes {
			if hash == "" {
				continue
			}
			// Individual GridFS deletions (GridFS doesn't support bulk delete)
			s.gridfsBucket.Delete(hash) // Ignore errors for individual deletes
		}
	}

	return nil
}

// GetObjectSize returns the size of an object
func (s *ObjectStore) GetObjectSize(hash string) (int64, error) {
	if hash == "" {
		return 0, datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check regular collection first
	var doc ObjectDocument
	err := s.objects.FindOne(ctx, bson.M{"_id": hash}, options.FindOne().SetProjection(bson.M{"size": 1})).Decode(&doc)
	if err == nil {
		return doc.Size, nil
	}

	// Check GridFS if enabled
	if s.useGridFS && s.gridfsBucket != nil && err == mongo.ErrNoDocuments {
		cursor, err := s.gridfsBucket.Find(bson.M{"filename": hash})
		if err == nil {
			defer cursor.Close(ctx)
			if cursor.Next(ctx) {
				var file gridfs.File
				if err := cursor.Decode(&file); err == nil {
					return file.Length, nil
				}
			}
		}
	}

	if err == mongo.ErrNoDocuments {
		return 0, datastore.ErrNotFound
	}

	return 0, fmt.Errorf("failed to get object size %s: %w", hash, err)
}

// CountObjects returns the total number of objects
func (s *ObjectStore) CountObjects() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Count objects in regular collection
	count, err := s.objects.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("failed to count objects: %w", err)
	}

	// Count GridFS objects if enabled
	if s.useGridFS && s.gridfsBucket != nil {
		gfsCursor, err := s.gridfsBucket.Find(bson.M{})
		if err == nil {
			defer gfsCursor.Close(ctx)
			for gfsCursor.Next(ctx) {
				count++
			}
		}
		// Ignore GridFS count errors, just use regular collection count
	}

	return count, nil
}

// GetStorageSize returns the total storage used
func (s *ObjectStore) GetStorageSize() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Aggregate size from regular collection
	pipeline := []bson.M{
		{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": "$size"}}},
	}

	cursor, err := s.objects.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate storage size: %w", err)
	}
	defer cursor.Close(ctx)

	var totalSize int64
	if cursor.Next(ctx) {
		var result struct {
			Total int64 `bson:"total"`
		}
		if err := cursor.Decode(&result); err == nil {
			totalSize = result.Total
		}
	}

	// Add GridFS size if enabled (simplified estimation)
	if s.useGridFS && s.gridfsBucket != nil {
		gfsCursor, err := s.gridfsBucket.Find(bson.M{})
		if err == nil {
			defer gfsCursor.Close(ctx)
			for gfsCursor.Next(ctx) {
				var file gridfs.File
				if err := gfsCursor.Decode(&file); err == nil {
					totalSize += file.Length
				}
			}
		}
	}

	return totalSize, nil
}