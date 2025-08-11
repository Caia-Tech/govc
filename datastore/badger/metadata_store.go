package badger

import (
	"bytes"
	"fmt"
	"time"

	"github.com/Caia-Tech/govc/datastore"
	badgerdb "github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
)

// MetadataStore implementation

// SaveRepository saves a repository
func (s *BadgerStore) SaveRepository(repo *datastore.Repository) error {
	s.writes.Add(1)
	
	data, err := marshalJSON(repo)
	if err != nil {
		return fmt.Errorf("failed to marshal repository: %w", err)
	}
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		return txn.Set(makeRepoKey(repo.ID), data)
	})
}

// GetRepository retrieves a repository by ID
func (s *BadgerStore) GetRepository(id string) (*datastore.Repository, error) {
	s.reads.Add(1)
	
	var repo datastore.Repository
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		item, err := txn.Get(makeRepoKey(id))
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		return item.Value(func(val []byte) error {
			return unmarshalJSON(val, &repo)
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return &repo, nil
}

// ListRepositories lists repositories with filtering
func (s *BadgerStore) ListRepositories(filter datastore.RepositoryFilter) ([]*datastore.Repository, error) {
	s.reads.Add(1)
	
	var repos []*datastore.Repository
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		opts := badgerdb.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		
		prefix := []byte(prefixRepo)
		count := 0
		skipped := 0
		
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			// Skip based on offset
			if skipped < filter.Offset {
				skipped++
				continue
			}
			
			// Check limit
			if filter.Limit > 0 && count >= filter.Limit {
				break
			}
			
			var repo datastore.Repository
			err := it.Item().Value(func(val []byte) error {
				return unmarshalJSON(val, &repo)
			})
			if err != nil {
				return err
			}
			
			// Apply filters
			if filter.Name != "" && !bytes.Contains([]byte(repo.Name), []byte(filter.Name)) {
				continue
			}
			
			if filter.IsPrivate != nil && repo.IsPrivate != *filter.IsPrivate {
				continue
			}
			
			if filter.CreatedAfter != nil && repo.CreatedAt.Before(*filter.CreatedAfter) {
				continue
			}
			
			if filter.CreatedBefore != nil && repo.CreatedAt.After(*filter.CreatedBefore) {
				continue
			}
			
			repos = append(repos, &repo)
			count++
		}
		
		return nil
	})
	
	return repos, err
}

// DeleteRepository deletes a repository
func (s *BadgerStore) DeleteRepository(id string) error {
	s.deletes.Add(1)
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		// Check if exists
		_, err := txn.Get(makeRepoKey(id))
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		// Delete repository
		if err := txn.Delete(makeRepoKey(id)); err != nil {
			return err
		}
		
		// Delete all refs for this repository
		prefix := []byte(prefixRef + id + ":")
		opts := badgerdb.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if err := txn.Delete(it.Item().KeyCopy(nil)); err != nil {
				return err
			}
		}
		
		return nil
	})
}

// UpdateRepository updates a repository
func (s *BadgerStore) UpdateRepository(id string, updates map[string]interface{}) error {
	s.writes.Add(1)
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		// Get existing repository
		item, err := txn.Get(makeRepoKey(id))
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		var repo datastore.Repository
		err = item.Value(func(val []byte) error {
			return unmarshalJSON(val, &repo)
		})
		if err != nil {
			return err
		}
		
		// Apply updates
		for key, value := range updates {
			switch key {
			case "name":
				if v, ok := value.(string); ok {
					repo.Name = v
				}
			case "description":
				if v, ok := value.(string); ok {
					repo.Description = v
				}
			case "is_private":
				if v, ok := value.(bool); ok {
					repo.IsPrivate = v
				}
			case "metadata":
				if v, ok := value.(map[string]interface{}); ok {
					repo.Metadata = v
				}
			}
		}
		
		repo.UpdatedAt = time.Now()
		
		// Save updated repository
		data, err := marshalJSON(&repo)
		if err != nil {
			return err
		}
		
		return txn.Set(makeRepoKey(id), data)
	})
}

// SaveUser saves a user
func (s *BadgerStore) SaveUser(user *datastore.User) error {
	s.writes.Add(1)
	
	data, err := marshalJSON(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		// Save user by ID
		if err := txn.Set(makeUserKey(user.ID), data); err != nil {
			return err
		}
		
		// Save username -> ID mapping
		return txn.Set(makeUserByNameKey(user.Username), []byte(user.ID))
	})
}

// GetUser retrieves a user by ID
func (s *BadgerStore) GetUser(id string) (*datastore.User, error) {
	s.reads.Add(1)
	
	var user datastore.User
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		item, err := txn.Get(makeUserKey(id))
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		return item.Value(func(val []byte) error {
			return unmarshalJSON(val, &user)
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return &user, nil
}

// GetUserByUsername retrieves a user by username
func (s *BadgerStore) GetUserByUsername(username string) (*datastore.User, error) {
	s.reads.Add(1)
	
	var userID string
	
	// First get the user ID from username mapping
	err := s.db.View(func(txn *badgerdb.Txn) error {
		item, err := txn.Get(makeUserByNameKey(username))
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		return item.Value(func(val []byte) error {
			userID = string(val)
			return nil
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	// Now get the user by ID
	return s.GetUser(userID)
}

// ListUsers lists users with filtering
func (s *BadgerStore) ListUsers(filter datastore.UserFilter) ([]*datastore.User, error) {
	s.reads.Add(1)
	
	var users []*datastore.User
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		opts := badgerdb.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		
		prefix := []byte(prefixUser)
		count := 0
		skipped := 0
		
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			// Skip based on offset
			if skipped < filter.Offset {
				skipped++
				continue
			}
			
			// Check limit
			if filter.Limit > 0 && count >= filter.Limit {
				break
			}
			
			var user datastore.User
			err := it.Item().Value(func(val []byte) error {
				return unmarshalJSON(val, &user)
			})
			if err != nil {
				return err
			}
			
			// Apply filters
			if filter.Username != "" && user.Username != filter.Username {
				continue
			}
			
			if filter.Email != "" && user.Email != filter.Email {
				continue
			}
			
			if filter.IsActive != nil && user.IsActive != *filter.IsActive {
				continue
			}
			
			if filter.IsAdmin != nil && user.IsAdmin != *filter.IsAdmin {
				continue
			}
			
			users = append(users, &user)
			count++
		}
		
		return nil
	})
	
	return users, err
}

// DeleteUser deletes a user
func (s *BadgerStore) DeleteUser(id string) error {
	s.deletes.Add(1)
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		// Get user to find username
		item, err := txn.Get(makeUserKey(id))
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		var user datastore.User
		err = item.Value(func(val []byte) error {
			return unmarshalJSON(val, &user)
		})
		if err != nil {
			return err
		}
		
		// Delete user
		if err := txn.Delete(makeUserKey(id)); err != nil {
			return err
		}
		
		// Delete username mapping
		return txn.Delete(makeUserByNameKey(user.Username))
	})
}

// UpdateUser updates a user
func (s *BadgerStore) UpdateUser(id string, updates map[string]interface{}) error {
	s.writes.Add(1)
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		// Get existing user
		item, err := txn.Get(makeUserKey(id))
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		var user datastore.User
		err = item.Value(func(val []byte) error {
			return unmarshalJSON(val, &user)
		})
		if err != nil {
			return err
		}
		
		oldUsername := user.Username
		
		// Apply updates
		for key, value := range updates {
			switch key {
			case "username":
				if v, ok := value.(string); ok {
					user.Username = v
				}
			case "email":
				if v, ok := value.(string); ok {
					user.Email = v
				}
			case "full_name":
				if v, ok := value.(string); ok {
					user.FullName = v
				}
			case "is_active":
				if v, ok := value.(bool); ok {
					user.IsActive = v
				}
			case "is_admin":
				if v, ok := value.(bool); ok {
					user.IsAdmin = v
				}
			case "metadata":
				if v, ok := value.(map[string]interface{}); ok {
					user.Metadata = v
				}
			}
		}
		
		user.UpdatedAt = time.Now()
		
		// Save updated user
		data, err := marshalJSON(&user)
		if err != nil {
			return err
		}
		
		if err := txn.Set(makeUserKey(id), data); err != nil {
			return err
		}
		
		// Update username mapping if username changed
		if oldUsername != user.Username {
			// Delete old mapping
			if err := txn.Delete(makeUserByNameKey(oldUsername)); err != nil {
				return err
			}
			// Create new mapping
			if err := txn.Set(makeUserByNameKey(user.Username), []byte(user.ID)); err != nil {
				return err
			}
		}
		
		return nil
	})
}

// SaveRef saves a reference
func (s *BadgerStore) SaveRef(repoID string, ref *datastore.Reference) error {
	s.writes.Add(1)
	
	data, err := marshalJSON(ref)
	if err != nil {
		return fmt.Errorf("failed to marshal ref: %w", err)
	}
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		return txn.Set(makeRefKey(repoID, ref.Name), data)
	})
}

// GetRef retrieves a reference
func (s *BadgerStore) GetRef(repoID string, name string) (*datastore.Reference, error) {
	s.reads.Add(1)
	
	var ref datastore.Reference
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		item, err := txn.Get(makeRefKey(repoID, name))
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		return item.Value(func(val []byte) error {
			return unmarshalJSON(val, &ref)
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return &ref, nil
}

// ListRefs lists references for a repository
func (s *BadgerStore) ListRefs(repoID string, refType datastore.RefType) ([]*datastore.Reference, error) {
	s.reads.Add(1)
	
	var refs []*datastore.Reference
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		opts := badgerdb.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		
		prefix := []byte(prefixRef + repoID + ":")
		
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var ref datastore.Reference
			err := it.Item().Value(func(val []byte) error {
				return unmarshalJSON(val, &ref)
			})
			if err != nil {
				return err
			}
			
			// Filter by type if specified
			if refType != "" && ref.Type != refType {
				continue
			}
			
			refs = append(refs, &ref)
		}
		
		return nil
	})
	
	return refs, err
}

// DeleteRef deletes a reference
func (s *BadgerStore) DeleteRef(repoID string, name string) error {
	s.deletes.Add(1)
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		key := makeRefKey(repoID, name)
		
		// Check if exists
		_, err := txn.Get(key)
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		return txn.Delete(key)
	})
}

// UpdateRef updates a reference hash
func (s *BadgerStore) UpdateRef(repoID string, name string, newHash string) error {
	s.writes.Add(1)
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		key := makeRefKey(repoID, name)
		
		// Get existing ref
		item, err := txn.Get(key)
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		var ref datastore.Reference
		err = item.Value(func(val []byte) error {
			return unmarshalJSON(val, &ref)
		})
		if err != nil {
			return err
		}
		
		// Update hash
		ref.Hash = newHash
		ref.UpdatedAt = time.Now()
		
		// Save updated ref
		data, err := marshalJSON(&ref)
		if err != nil {
			return err
		}
		
		return txn.Set(key, data)
	})
}

// LogEvent logs an audit event
func (s *BadgerStore) LogEvent(event *datastore.AuditEvent) error {
	s.writes.Add(1)
	
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	data, err := marshalJSON(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		// Use timestamp in key for time-based ordering
		key := fmt.Sprintf("%s%d:%s", prefixEvent, event.Timestamp.UnixNano(), event.ID)
		return txn.Set([]byte(key), data)
	})
}

// QueryEvents queries audit events
func (s *BadgerStore) QueryEvents(filter datastore.EventFilter) ([]*datastore.AuditEvent, error) {
	s.reads.Add(1)
	
	var events []*datastore.AuditEvent
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		opts := badgerdb.DefaultIteratorOptions
		opts.Reverse = true // Most recent first
		it := txn.NewIterator(opts)
		defer it.Close()
		
		prefix := []byte(prefixEvent)
		count := 0
		skipped := 0
		
		for it.Seek(append(prefix, 0xFF)); it.ValidForPrefix(prefix); it.Next() {
			// Skip based on offset
			if skipped < filter.Offset {
				skipped++
				continue
			}
			
			// Check limit
			if filter.Limit > 0 && count >= filter.Limit {
				break
			}
			
			var event datastore.AuditEvent
			err := it.Item().Value(func(val []byte) error {
				return unmarshalJSON(val, &event)
			})
			if err != nil {
				return err
			}
			
			// Apply filters
			if filter.UserID != "" && event.UserID != filter.UserID {
				continue
			}
			
			if filter.Action != "" && event.Action != filter.Action {
				continue
			}
			
			if filter.Resource != "" && event.Resource != filter.Resource {
				continue
			}
			
			if filter.Success != nil && event.Success != *filter.Success {
				continue
			}
			
			if filter.After != nil && event.Timestamp.Before(*filter.After) {
				continue
			}
			
			if filter.Before != nil && event.Timestamp.After(*filter.Before) {
				continue
			}
			
			events = append(events, &event)
			count++
		}
		
		return nil
	})
	
	return events, err
}

// CountEvents counts audit events
func (s *BadgerStore) CountEvents(filter datastore.EventFilter) (int64, error) {
	s.reads.Add(1)
	
	var count int64
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		opts := badgerdb.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		
		prefix := []byte(prefixEvent)
		
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if filter.UserID != "" || filter.Action != "" || filter.Resource != "" {
				// Need to load the value to apply filters
				var event datastore.AuditEvent
				err := it.Item().Value(func(val []byte) error {
					return unmarshalJSON(val, &event)
				})
				if err != nil {
					return err
				}
				
				// Apply filters
				if filter.UserID != "" && event.UserID != filter.UserID {
					continue
				}
				
				if filter.Action != "" && event.Action != filter.Action {
					continue
				}
				
				if filter.Resource != "" && event.Resource != filter.Resource {
					continue
				}
				
				if filter.Success != nil && event.Success != *filter.Success {
					continue
				}
				
				if filter.After != nil && event.Timestamp.Before(*filter.After) {
					continue
				}
				
				if filter.Before != nil && event.Timestamp.After(*filter.Before) {
					continue
				}
			}
			
			count++
		}
		
		return nil
	})
	
	return count, err
}

// GetConfig retrieves a configuration value
func (s *BadgerStore) GetConfig(key string) (string, error) {
	s.reads.Add(1)
	
	var value string
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		item, err := txn.Get(makeConfigKey(key))
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		return item.Value(func(val []byte) error {
			value = string(val)
			return nil
		})
	})
	
	return value, err
}

// SetConfig sets a configuration value
func (s *BadgerStore) SetConfig(key string, value string) error {
	s.writes.Add(1)
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		return txn.Set(makeConfigKey(key), []byte(value))
	})
}

// GetAllConfig retrieves all configuration
func (s *BadgerStore) GetAllConfig() (map[string]string, error) {
	s.reads.Add(1)
	
	config := make(map[string]string)
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		opts := badgerdb.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		
		prefix := []byte(prefixConfig)
		
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key()[len(prefixConfig):])
			
			err := it.Item().Value(func(val []byte) error {
				config[key] = string(val)
				return nil
			})
			if err != nil {
				return err
			}
		}
		
		return nil
	})
	
	return config, err
}

// DeleteConfig deletes a configuration key
func (s *BadgerStore) DeleteConfig(key string) error {
	s.deletes.Add(1)
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		k := makeConfigKey(key)
		
		// Check if exists
		_, err := txn.Get(k)
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		return txn.Delete(k)
	})
}