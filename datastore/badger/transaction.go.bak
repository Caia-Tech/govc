package badger

import (
	"context"
	"fmt"
	"time"

	"github.com/caiatech/govc/datastore"
	badgerdb "github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
)

// badgerTransaction implements datastore.Transaction for BadgerDB
type badgerTransaction struct {
	store *BadgerStore
	txn   *badgerdb.Txn
	ctx   context.Context
}

// ObjectStore methods

func (t *badgerTransaction) GetObject(hash string) ([]byte, error) {
	t.store.reads.Add(1)
	
	item, err := t.txn.Get(makeObjectKey(hash))
	if err == badgerdb.ErrKeyNotFound {
		return nil, datastore.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	
	var data []byte
	err = item.Value(func(val []byte) error {
		data = append([]byte{}, val...) // Copy the value
		return nil
	})
	
	return data, err
}

func (t *badgerTransaction) PutObject(hash string, data []byte) error {
	t.store.writes.Add(1)
	return t.txn.Set(makeObjectKey(hash), data)
}

func (t *badgerTransaction) DeleteObject(hash string) error {
	t.store.deletes.Add(1)
	
	key := makeObjectKey(hash)
	
	// Check if the key exists first
	_, err := t.txn.Get(key)
	if err == badgerdb.ErrKeyNotFound {
		return datastore.ErrNotFound
	}
	if err != nil {
		return err
	}
	
	// Key exists, now delete it
	return t.txn.Delete(key)
}

func (t *badgerTransaction) HasObject(hash string) (bool, error) {
	t.store.reads.Add(1)
	
	_, err := t.txn.Get(makeObjectKey(hash))
	if err == badgerdb.ErrKeyNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	
	return true, nil
}

func (t *badgerTransaction) ListObjects(prefix string, limit int) ([]string, error) {
	t.store.reads.Add(1)
	
	var hashes []string
	
	opts := badgerdb.DefaultIteratorOptions
	opts.PrefetchValues = false
	it := t.txn.NewIterator(opts)
	defer it.Close()
	
	searchPrefix := []byte(prefixObject + prefix)
	count := 0
	
	for it.Seek(searchPrefix); it.ValidForPrefix(searchPrefix); it.Next() {
		if limit > 0 && count >= limit {
			break
		}
		
		key := it.Item().Key()
		hash := string(key[len(prefixObject):])
		hashes = append(hashes, hash)
		count++
	}
	
	return hashes, nil
}

func (t *badgerTransaction) IterateObjects(prefix string, fn func(hash string, data []byte) error) error {
	t.store.reads.Add(1)
	
	opts := badgerdb.DefaultIteratorOptions
	it := t.txn.NewIterator(opts)
	defer it.Close()
	
	searchPrefix := []byte(prefixObject + prefix)
	
	for it.Seek(searchPrefix); it.ValidForPrefix(searchPrefix); it.Next() {
		item := it.Item()
		key := item.Key()
		hash := string(key[len(prefixObject):])
		
		err := item.Value(func(val []byte) error {
			return fn(hash, val)
		})
		
		if err != nil {
			return err
		}
	}
	
	return nil
}

func (t *badgerTransaction) GetObjects(hashes []string) (map[string][]byte, error) {
	if len(hashes) == 0 {
		return map[string][]byte{}, nil
	}
	
	t.store.reads.Add(int64(len(hashes)))
	
	result := make(map[string][]byte)
	
	for _, hash := range hashes {
		item, err := t.txn.Get(makeObjectKey(hash))
		if err == badgerdb.ErrKeyNotFound {
			continue // Skip missing objects
		}
		if err != nil {
			return nil, err
		}
		
		err = item.Value(func(val []byte) error {
			result[hash] = append([]byte{}, val...) // Copy the value
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	
	return result, nil
}

func (t *badgerTransaction) PutObjects(objects map[string][]byte) error {
	if len(objects) == 0 {
		return nil
	}
	
	t.store.writes.Add(int64(len(objects)))
	
	for hash, data := range objects {
		if err := t.txn.Set(makeObjectKey(hash), data); err != nil {
			return err
		}
	}
	
	return nil
}

func (t *badgerTransaction) DeleteObjects(hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	
	t.store.deletes.Add(int64(len(hashes)))
	
	for _, hash := range hashes {
		if err := t.txn.Delete(makeObjectKey(hash)); err != nil && err != badgerdb.ErrKeyNotFound {
			return err
		}
	}
	
	return nil
}

func (t *badgerTransaction) GetObjectSize(hash string) (int64, error) {
	t.store.reads.Add(1)
	
	item, err := t.txn.Get(makeObjectKey(hash))
	if err == badgerdb.ErrKeyNotFound {
		return 0, datastore.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	
	return int64(item.ValueSize()), nil
}

func (t *badgerTransaction) CountObjects() (int64, error) {
	var count int64
	
	opts := badgerdb.DefaultIteratorOptions
	opts.PrefetchValues = false
	it := t.txn.NewIterator(opts)
	defer it.Close()
	
	prefix := []byte(prefixObject)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		count++
	}
	
	return count, nil
}

func (t *badgerTransaction) GetStorageSize() (int64, error) {
	var totalSize int64
	
	opts := badgerdb.DefaultIteratorOptions
	opts.PrefetchSize = 1000
	it := t.txn.NewIterator(opts)
	defer it.Close()
	
	prefix := []byte(prefixObject)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		totalSize += int64(it.Item().ValueSize())
	}
	
	return totalSize, nil
}

// MetadataStore methods

func (t *badgerTransaction) SaveRepository(repo *datastore.Repository) error {
	t.store.writes.Add(1)
	
	data, err := marshalJSON(repo)
	if err != nil {
		return fmt.Errorf("failed to marshal repository: %w", err)
	}
	
	return t.txn.Set(makeRepoKey(repo.ID), data)
}

func (t *badgerTransaction) GetRepository(id string) (*datastore.Repository, error) {
	t.store.reads.Add(1)
	
	item, err := t.txn.Get(makeRepoKey(id))
	if err == badgerdb.ErrKeyNotFound {
		return nil, datastore.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	
	var repo datastore.Repository
	err = item.Value(func(val []byte) error {
		return unmarshalJSON(val, &repo)
	})
	
	if err != nil {
		return nil, err
	}
	
	return &repo, nil
}

func (t *badgerTransaction) ListRepositories(filter datastore.RepositoryFilter) ([]*datastore.Repository, error) {
	// Delegate to store for simplicity
	return t.store.ListRepositories(filter)
}

func (t *badgerTransaction) DeleteRepository(id string) error {
	t.store.deletes.Add(1)
	
	// Check if exists
	_, err := t.txn.Get(makeRepoKey(id))
	if err == badgerdb.ErrKeyNotFound {
		return datastore.ErrNotFound
	}
	if err != nil {
		return err
	}
	
	// Delete repository
	if err := t.txn.Delete(makeRepoKey(id)); err != nil {
		return err
	}
	
	// Delete all refs for this repository
	prefix := []byte(prefixRef + id + ":")
	opts := badgerdb.DefaultIteratorOptions
	opts.PrefetchValues = false
	it := t.txn.NewIterator(opts)
	defer it.Close()
	
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		if err := t.txn.Delete(it.Item().KeyCopy(nil)); err != nil {
			return err
		}
	}
	
	return nil
}

func (t *badgerTransaction) UpdateRepository(id string, updates map[string]interface{}) error {
	repo, err := t.GetRepository(id)
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
	return t.SaveRepository(repo)
}

func (t *badgerTransaction) SaveUser(user *datastore.User) error {
	t.store.writes.Add(1)
	
	data, err := marshalJSON(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}
	
	// Save user by ID
	if err := t.txn.Set(makeUserKey(user.ID), data); err != nil {
		return err
	}
	
	// Save username -> ID mapping
	return t.txn.Set(makeUserByNameKey(user.Username), []byte(user.ID))
}

func (t *badgerTransaction) GetUser(id string) (*datastore.User, error) {
	t.store.reads.Add(1)
	
	item, err := t.txn.Get(makeUserKey(id))
	if err == badgerdb.ErrKeyNotFound {
		return nil, datastore.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	
	var user datastore.User
	err = item.Value(func(val []byte) error {
		return unmarshalJSON(val, &user)
	})
	
	if err != nil {
		return nil, err
	}
	
	return &user, nil
}

func (t *badgerTransaction) GetUserByUsername(username string) (*datastore.User, error) {
	t.store.reads.Add(1)
	
	// First get the user ID from username mapping
	item, err := t.txn.Get(makeUserByNameKey(username))
	if err == badgerdb.ErrKeyNotFound {
		return nil, datastore.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	
	var userID string
	err = item.Value(func(val []byte) error {
		userID = string(val)
		return nil
	})
	if err != nil {
		return nil, err
	}
	
	// Now get the user by ID
	return t.GetUser(userID)
}

func (t *badgerTransaction) ListUsers(filter datastore.UserFilter) ([]*datastore.User, error) {
	// Delegate to store for simplicity
	return t.store.ListUsers(filter)
}

func (t *badgerTransaction) DeleteUser(id string) error {
	t.store.deletes.Add(1)
	
	// Get user to find username
	user, err := t.GetUser(id)
	if err != nil {
		return err
	}
	
	// Delete user
	if err := t.txn.Delete(makeUserKey(id)); err != nil {
		return err
	}
	
	// Delete username mapping
	return t.txn.Delete(makeUserByNameKey(user.Username))
}

func (t *badgerTransaction) UpdateUser(id string, updates map[string]interface{}) error {
	user, err := t.GetUser(id)
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
	if err := t.SaveUser(user); err != nil {
		return err
	}
	
	// Update username mapping if username changed
	if oldUsername != user.Username {
		// Delete old mapping
		if err := t.txn.Delete(makeUserByNameKey(oldUsername)); err != nil {
			return err
		}
	}
	
	return nil
}

func (t *badgerTransaction) SaveRef(repoID string, ref *datastore.Reference) error {
	t.store.writes.Add(1)
	
	data, err := marshalJSON(ref)
	if err != nil {
		return fmt.Errorf("failed to marshal ref: %w", err)
	}
	
	return t.txn.Set(makeRefKey(repoID, ref.Name), data)
}

func (t *badgerTransaction) GetRef(repoID string, name string) (*datastore.Reference, error) {
	t.store.reads.Add(1)
	
	item, err := t.txn.Get(makeRefKey(repoID, name))
	if err == badgerdb.ErrKeyNotFound {
		return nil, datastore.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	
	var ref datastore.Reference
	err = item.Value(func(val []byte) error {
		return unmarshalJSON(val, &ref)
	})
	
	if err != nil {
		return nil, err
	}
	
	return &ref, nil
}

func (t *badgerTransaction) ListRefs(repoID string, refType datastore.RefType) ([]*datastore.Reference, error) {
	// Delegate to store for simplicity
	return t.store.ListRefs(repoID, refType)
}

func (t *badgerTransaction) DeleteRef(repoID string, name string) error {
	t.store.deletes.Add(1)
	
	key := makeRefKey(repoID, name)
	
	// Check if exists
	_, err := t.txn.Get(key)
	if err == badgerdb.ErrKeyNotFound {
		return datastore.ErrNotFound
	}
	if err != nil {
		return err
	}
	
	return t.txn.Delete(key)
}

func (t *badgerTransaction) UpdateRef(repoID string, name string, newHash string) error {
	ref, err := t.GetRef(repoID, name)
	if err != nil {
		return err
	}
	
	ref.Hash = newHash
	ref.UpdatedAt = time.Now()
	return t.SaveRef(repoID, ref)
}

func (t *badgerTransaction) LogEvent(event *datastore.AuditEvent) error {
	t.store.writes.Add(1)
	
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
	
	// Use timestamp in key for time-based ordering
	key := fmt.Sprintf("%s%d:%s", prefixEvent, event.Timestamp.UnixNano(), event.ID)
	return t.txn.Set([]byte(key), data)
}

func (t *badgerTransaction) QueryEvents(filter datastore.EventFilter) ([]*datastore.AuditEvent, error) {
	// Delegate to store for simplicity
	return t.store.QueryEvents(filter)
}

func (t *badgerTransaction) CountEvents(filter datastore.EventFilter) (int64, error) {
	// Delegate to store for simplicity
	return t.store.CountEvents(filter)
}

func (t *badgerTransaction) GetConfig(key string) (string, error) {
	t.store.reads.Add(1)
	
	item, err := t.txn.Get(makeConfigKey(key))
	if err == badgerdb.ErrKeyNotFound {
		return "", datastore.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	
	var value string
	err = item.Value(func(val []byte) error {
		value = string(val)
		return nil
	})
	
	return value, err
}

func (t *badgerTransaction) SetConfig(key string, value string) error {
	t.store.writes.Add(1)
	return t.txn.Set(makeConfigKey(key), []byte(value))
}

func (t *badgerTransaction) GetAllConfig() (map[string]string, error) {
	// Delegate to store for simplicity
	return t.store.GetAllConfig()
}

func (t *badgerTransaction) DeleteConfig(key string) error {
	t.store.deletes.Add(1)
	
	k := makeConfigKey(key)
	
	// Check if exists
	_, err := t.txn.Get(k)
	if err == badgerdb.ErrKeyNotFound {
		return datastore.ErrNotFound
	}
	if err != nil {
		return err
	}
	
	return t.txn.Delete(k)
}

// Transaction control

func (t *badgerTransaction) Commit() error {
	return t.txn.Commit()
}

func (t *badgerTransaction) Rollback() error {
	t.txn.Discard()
	return nil
}