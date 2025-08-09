package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/caiatech/govc/datastore"
)

// MetadataStore implements MongoDB-based metadata storage
type MetadataStore struct {
	database     *mongo.Database
	config       datastore.Config
	repositories *mongo.Collection
	users        *mongo.Collection
	references   *mongo.Collection
	auditEvents  *mongo.Collection
	configuration *mongo.Collection
}

// NewMetadataStore creates a new MongoDB metadata store
func NewMetadataStore(database *mongo.Database, config datastore.Config) *MetadataStore {
	store := &MetadataStore{
		database:      database,
		config:        config,
		repositories:  database.Collection("repositories"),
		users:         database.Collection("users"),
		references:    database.Collection("references"),
		auditEvents:   database.Collection("audit_events"),
		configuration: database.Collection("configuration"),
	}

	// Create indexes for better performance
	store.createIndexes()

	return store
}

// createIndexes creates necessary indexes for collections
func (s *MetadataStore) createIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Repository indexes
	s.repositories.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.M{"name": 1}, Options: options.Index().SetUnique(true)},
		{Keys: bson.M{"created_at": 1}},
		{Keys: bson.M{"is_private": 1}},
	})

	// User indexes
	s.users.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.M{"username": 1}, Options: options.Index().SetUnique(true)},
		{Keys: bson.M{"email": 1}, Options: options.Index().SetUnique(true)},
		{Keys: bson.M{"is_active": 1}},
	})

	// Reference indexes
	s.references.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.M{"repo_id": 1, "name": 1}, Options: options.Index().SetUnique(true)},
		{Keys: bson.M{"repo_id": 1, "type": 1}},
		{Keys: bson.M{"updated_at": 1}},
	})

	// Audit event indexes
	s.auditEvents.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.M{"timestamp": 1}},
		{Keys: bson.M{"user_id": 1}},
		{Keys: bson.M{"action": 1}},
		{Keys: bson.M{"resource": 1}},
	})

	// Configuration indexes
	s.configuration.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.M{"key": 1}, 
		Options: options.Index().SetUnique(true),
	})
}

// Repository management

// SaveRepository saves or updates a repository
func (s *MetadataStore) SaveRepository(repo *datastore.Repository) error {
	if repo == nil {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if repo.CreatedAt.IsZero() {
		repo.CreatedAt = time.Now()
	}
	repo.UpdatedAt = time.Now()

	_, err := s.repositories.ReplaceOne(
		ctx,
		bson.M{"_id": repo.ID},
		repo,
		options.Replace().SetUpsert(true),
	)

	if err != nil {
		return fmt.Errorf("failed to save repository: %w", err)
	}

	return nil
}

// GetRepository retrieves a repository by ID
func (s *MetadataStore) GetRepository(id string) (*datastore.Repository, error) {
	if id == "" {
		return nil, datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var repo datastore.Repository
	err := s.repositories.FindOne(ctx, bson.M{"_id": id}).Decode(&repo)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, datastore.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get repository %s: %w", id, err)
	}

	return &repo, nil
}

// ListRepositories lists repositories matching the filter
func (s *MetadataStore) ListRepositories(filter datastore.RepositoryFilter) ([]*datastore.Repository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build MongoDB filter
	mongoFilter := bson.M{}
	
	if len(filter.IDs) > 0 {
		mongoFilter["_id"] = bson.M{"$in": filter.IDs}
	}
	
	if filter.Name != "" {
		mongoFilter["name"] = bson.M{"$regex": filter.Name, "$options": "i"}
	}
	
	if filter.IsPrivate != nil {
		mongoFilter["is_private"] = *filter.IsPrivate
	}
	
	if filter.CreatedAfter != nil {
		mongoFilter["created_at"] = bson.M{"$gte": *filter.CreatedAfter}
	}
	
	if filter.CreatedBefore != nil {
		if createdAt, exists := mongoFilter["created_at"]; exists {
			createdAt.(bson.M)["$lte"] = *filter.CreatedBefore
		} else {
			mongoFilter["created_at"] = bson.M{"$lte": *filter.CreatedBefore}
		}
	}

	// Build options
	findOptions := options.Find()
	
	if filter.Limit > 0 {
		findOptions.SetLimit(int64(filter.Limit))
	}
	
	if filter.Offset > 0 {
		findOptions.SetSkip(int64(filter.Offset))
	}
	
	if filter.OrderBy != "" {
		order := 1
		if filter.OrderBy[0] == '-' {
			order = -1
			filter.OrderBy = filter.OrderBy[1:]
		}
		findOptions.SetSort(bson.M{filter.OrderBy: order})
	}

	cursor, err := s.repositories.Find(ctx, mongoFilter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}
	defer cursor.Close(ctx)

	var repositories []*datastore.Repository
	for cursor.Next(ctx) {
		var repo datastore.Repository
		if err := cursor.Decode(&repo); err != nil {
			continue
		}
		repositories = append(repositories, &repo)
	}

	return repositories, nil
}

// DeleteRepository removes a repository
func (s *MetadataStore) DeleteRepository(id string) error {
	if id == "" {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.repositories.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("failed to delete repository %s: %w", id, err)
	}

	if result.DeletedCount == 0 {
		return datastore.ErrNotFound
	}

	return nil
}

// UpdateRepository updates repository fields
func (s *MetadataStore) UpdateRepository(id string, updates map[string]interface{}) error {
	if id == "" || len(updates) == 0 {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	updates["updated_at"] = time.Now()

	result, err := s.repositories.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": updates},
	)
	
	if err != nil {
		return fmt.Errorf("failed to update repository %s: %w", id, err)
	}

	if result.MatchedCount == 0 {
		return datastore.ErrNotFound
	}

	return nil
}

// User management

// SaveUser saves or updates a user
func (s *MetadataStore) SaveUser(user *datastore.User) error {
	if user == nil {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}
	user.UpdatedAt = time.Now()

	_, err := s.users.ReplaceOne(
		ctx,
		bson.M{"_id": user.ID},
		user,
		options.Replace().SetUpsert(true),
	)

	if err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}

	return nil
}

// GetUser retrieves a user by ID
func (s *MetadataStore) GetUser(id string) (*datastore.User, error) {
	if id == "" {
		return nil, datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var user datastore.User
	err := s.users.FindOne(ctx, bson.M{"_id": id}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, datastore.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user %s: %w", id, err)
	}

	return &user, nil
}

// GetUserByUsername retrieves a user by username
func (s *MetadataStore) GetUserByUsername(username string) (*datastore.User, error) {
	if username == "" {
		return nil, datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var user datastore.User
	err := s.users.FindOne(ctx, bson.M{"username": username}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, datastore.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user by username %s: %w", username, err)
	}

	return &user, nil
}

// ListUsers lists users matching the filter
func (s *MetadataStore) ListUsers(filter datastore.UserFilter) ([]*datastore.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build MongoDB filter
	mongoFilter := bson.M{}
	
	if len(filter.IDs) > 0 {
		mongoFilter["_id"] = bson.M{"$in": filter.IDs}
	}
	
	if filter.Username != "" {
		mongoFilter["username"] = bson.M{"$regex": filter.Username, "$options": "i"}
	}
	
	if filter.Email != "" {
		mongoFilter["email"] = bson.M{"$regex": filter.Email, "$options": "i"}
	}
	
	if filter.IsActive != nil {
		mongoFilter["is_active"] = *filter.IsActive
	}
	
	if filter.IsAdmin != nil {
		mongoFilter["is_admin"] = *filter.IsAdmin
	}

	// Build options
	findOptions := options.Find()
	
	if filter.Limit > 0 {
		findOptions.SetLimit(int64(filter.Limit))
	}
	
	if filter.Offset > 0 {
		findOptions.SetSkip(int64(filter.Offset))
	}
	
	if filter.OrderBy != "" {
		order := 1
		if filter.OrderBy[0] == '-' {
			order = -1
			filter.OrderBy = filter.OrderBy[1:]
		}
		findOptions.SetSort(bson.M{filter.OrderBy: order})
	}

	cursor, err := s.users.Find(ctx, mongoFilter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer cursor.Close(ctx)

	var users []*datastore.User
	for cursor.Next(ctx) {
		var user datastore.User
		if err := cursor.Decode(&user); err != nil {
			continue
		}
		users = append(users, &user)
	}

	return users, nil
}

// DeleteUser removes a user
func (s *MetadataStore) DeleteUser(id string) error {
	if id == "" {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.users.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("failed to delete user %s: %w", id, err)
	}

	if result.DeletedCount == 0 {
		return datastore.ErrNotFound
	}

	return nil
}

// UpdateUser updates user fields
func (s *MetadataStore) UpdateUser(id string, updates map[string]interface{}) error {
	if id == "" || len(updates) == 0 {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	updates["updated_at"] = time.Now()

	result, err := s.users.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": updates},
	)
	
	if err != nil {
		return fmt.Errorf("failed to update user %s: %w", id, err)
	}

	if result.MatchedCount == 0 {
		return datastore.ErrNotFound
	}

	return nil
}

// Reference management

// ReferenceDocument represents a reference document in MongoDB
type ReferenceDocument struct {
	RepoID    string               `bson:"repo_id"`
	Name      string               `bson:"name"`
	Hash      string               `bson:"hash"`
	Type      datastore.RefType    `bson:"type"`
	UpdatedAt time.Time            `bson:"updated_at"`
	UpdatedBy string               `bson:"updated_by"`
}

// SaveRef saves or updates a reference
func (s *MetadataStore) SaveRef(repoID string, ref *datastore.Reference) error {
	if repoID == "" || ref == nil {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ref.UpdatedAt = time.Now()

	doc := ReferenceDocument{
		RepoID:    repoID,
		Name:      ref.Name,
		Hash:      ref.Hash,
		Type:      ref.Type,
		UpdatedAt: ref.UpdatedAt,
		UpdatedBy: ref.UpdatedBy,
	}

	_, err := s.references.ReplaceOne(
		ctx,
		bson.M{"repo_id": repoID, "name": ref.Name},
		doc,
		options.Replace().SetUpsert(true),
	)

	if err != nil {
		return fmt.Errorf("failed to save reference: %w", err)
	}

	return nil
}

// GetRef retrieves a reference
func (s *MetadataStore) GetRef(repoID string, name string) (*datastore.Reference, error) {
	if repoID == "" || name == "" {
		return nil, datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var doc ReferenceDocument
	err := s.references.FindOne(ctx, bson.M{"repo_id": repoID, "name": name}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, datastore.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get reference %s/%s: %w", repoID, name, err)
	}

	return &datastore.Reference{
		Name:      doc.Name,
		Hash:      doc.Hash,
		Type:      doc.Type,
		UpdatedAt: doc.UpdatedAt,
		UpdatedBy: doc.UpdatedBy,
	}, nil
}

// ListRefs lists references for a repository
func (s *MetadataStore) ListRefs(repoID string, refType datastore.RefType) ([]*datastore.Reference, error) {
	if repoID == "" {
		return nil, datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	filter := bson.M{"repo_id": repoID}
	if refType != "" {
		filter["type"] = refType
	}

	cursor, err := s.references.Find(ctx, filter, options.Find().SetSort(bson.M{"name": 1}))
	if err != nil {
		return nil, fmt.Errorf("failed to list references: %w", err)
	}
	defer cursor.Close(ctx)

	var references []*datastore.Reference
	for cursor.Next(ctx) {
		var doc ReferenceDocument
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		
		references = append(references, &datastore.Reference{
			Name:      doc.Name,
			Hash:      doc.Hash,
			Type:      doc.Type,
			UpdatedAt: doc.UpdatedAt,
			UpdatedBy: doc.UpdatedBy,
		})
	}

	return references, nil
}

// DeleteRef removes a reference
func (s *MetadataStore) DeleteRef(repoID string, name string) error {
	if repoID == "" || name == "" {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.references.DeleteOne(ctx, bson.M{"repo_id": repoID, "name": name})
	if err != nil {
		return fmt.Errorf("failed to delete reference %s/%s: %w", repoID, name, err)
	}

	if result.DeletedCount == 0 {
		return datastore.ErrNotFound
	}

	return nil
}

// UpdateRef updates a reference hash
func (s *MetadataStore) UpdateRef(repoID string, name string, newHash string) error {
	if repoID == "" || name == "" || newHash == "" {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.references.UpdateOne(
		ctx,
		bson.M{"repo_id": repoID, "name": name},
		bson.M{"$set": bson.M{"hash": newHash, "updated_at": time.Now()}},
	)
	
	if err != nil {
		return fmt.Errorf("failed to update reference %s/%s: %w", repoID, name, err)
	}

	if result.MatchedCount == 0 {
		return datastore.ErrNotFound
	}

	return nil
}

// Audit logging

// LogEvent logs an audit event
func (s *MetadataStore) LogEvent(event *datastore.AuditEvent) error {
	if event == nil {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	_, err := s.auditEvents.InsertOne(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to log audit event: %w", err)
	}

	return nil
}

// QueryEvents queries audit events
func (s *MetadataStore) QueryEvents(filter datastore.EventFilter) ([]*datastore.AuditEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build MongoDB filter
	mongoFilter := bson.M{}
	
	if filter.UserID != "" {
		mongoFilter["user_id"] = filter.UserID
	}
	
	if filter.Action != "" {
		mongoFilter["action"] = filter.Action
	}
	
	if filter.Resource != "" {
		mongoFilter["resource"] = filter.Resource
	}
	
	if filter.ResourceID != "" {
		mongoFilter["resource_id"] = filter.ResourceID
	}
	
	if filter.After != nil {
		mongoFilter["timestamp"] = bson.M{"$gte": *filter.After}
	}
	
	if filter.Before != nil {
		if timestamp, exists := mongoFilter["timestamp"]; exists {
			timestamp.(bson.M)["$lte"] = *filter.Before
		} else {
			mongoFilter["timestamp"] = bson.M{"$lte": *filter.Before}
		}
	}
	
	if filter.Success != nil {
		mongoFilter["success"] = *filter.Success
	}

	// Build options
	findOptions := options.Find()
	
	if filter.Limit > 0 {
		findOptions.SetLimit(int64(filter.Limit))
	}
	
	if filter.Offset > 0 {
		findOptions.SetSkip(int64(filter.Offset))
	}
	
	if filter.OrderBy != "" {
		order := 1
		if filter.OrderBy[0] == '-' {
			order = -1
			filter.OrderBy = filter.OrderBy[1:]
		}
		findOptions.SetSort(bson.M{filter.OrderBy: order})
	} else {
		findOptions.SetSort(bson.M{"timestamp": -1}) // Default: newest first
	}

	cursor, err := s.auditEvents.Find(ctx, mongoFilter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit events: %w", err)
	}
	defer cursor.Close(ctx)

	var events []*datastore.AuditEvent
	for cursor.Next(ctx) {
		var event datastore.AuditEvent
		if err := cursor.Decode(&event); err != nil {
			continue
		}
		events = append(events, &event)
	}

	return events, nil
}

// CountEvents counts audit events matching the filter
func (s *MetadataStore) CountEvents(filter datastore.EventFilter) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build MongoDB filter (same as QueryEvents)
	mongoFilter := bson.M{}
	
	if filter.UserID != "" {
		mongoFilter["user_id"] = filter.UserID
	}
	
	if filter.Action != "" {
		mongoFilter["action"] = filter.Action
	}
	
	if filter.Resource != "" {
		mongoFilter["resource"] = filter.Resource
	}
	
	if filter.ResourceID != "" {
		mongoFilter["resource_id"] = filter.ResourceID
	}
	
	if filter.After != nil {
		mongoFilter["timestamp"] = bson.M{"$gte": *filter.After}
	}
	
	if filter.Before != nil {
		if timestamp, exists := mongoFilter["timestamp"]; exists {
			timestamp.(bson.M)["$lte"] = *filter.Before
		} else {
			mongoFilter["timestamp"] = bson.M{"$lte": *filter.Before}
		}
	}
	
	if filter.Success != nil {
		mongoFilter["success"] = *filter.Success
	}

	count, err := s.auditEvents.CountDocuments(ctx, mongoFilter)
	if err != nil {
		return 0, fmt.Errorf("failed to count audit events: %w", err)
	}

	return count, nil
}

// Configuration management

// ConfigDocument represents a configuration document
type ConfigDocument struct {
	Key   string `bson:"_id"`
	Value string `bson:"value"`
}

// GetConfig retrieves a configuration value
func (s *MetadataStore) GetConfig(key string) (string, error) {
	if key == "" {
		return "", datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var doc ConfigDocument
	err := s.configuration.FindOne(ctx, bson.M{"_id": key}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return "", datastore.ErrNotFound
		}
		return "", fmt.Errorf("failed to get config %s: %w", key, err)
	}

	return doc.Value, nil
}

// SetConfig sets a configuration value
func (s *MetadataStore) SetConfig(key string, value string) error {
	if key == "" {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	doc := ConfigDocument{
		Key:   key,
		Value: value,
	}

	_, err := s.configuration.ReplaceOne(
		ctx,
		bson.M{"_id": key},
		doc,
		options.Replace().SetUpsert(true),
	)

	if err != nil {
		return fmt.Errorf("failed to set config %s: %w", key, err)
	}

	return nil
}

// GetAllConfig retrieves all configuration values
func (s *MetadataStore) GetAllConfig() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cursor, err := s.configuration.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to get all config: %w", err)
	}
	defer cursor.Close(ctx)

	config := make(map[string]string)
	for cursor.Next(ctx) {
		var doc ConfigDocument
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		config[doc.Key] = doc.Value
	}

	return config, nil
}

// DeleteConfig removes a configuration value
func (s *MetadataStore) DeleteConfig(key string) error {
	if key == "" {
		return datastore.ErrInvalidData
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.configuration.DeleteOne(ctx, bson.M{"_id": key})
	if err != nil {
		return fmt.Errorf("failed to delete config %s: %w", key, err)
	}

	if result.DeletedCount == 0 {
		return datastore.ErrNotFound
	}

	return nil
}