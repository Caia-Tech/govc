package memory

import (
	"fmt"
	
	"github.com/caiatech/govc/datastore"
)

// MetadataStore methods for transaction

func (tx *memoryTransaction) SaveRepository(repo *datastore.Repository) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledback {
		return ErrTransactionClosed
	}

	tx.mutations[repo.ID] = repo
	return nil
}

func (tx *memoryTransaction) GetRepository(id string) (*datastore.Repository, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if value, exists := tx.mutations[id]; exists {
		if repo, ok := value.(*datastore.Repository); ok {
			return repo, nil
		}
	}

	return tx.store.GetRepository(id)
}

func (tx *memoryTransaction) ListRepositories(filter datastore.RepositoryFilter) ([]*datastore.Repository, error) {
	// For simplicity, delegate to store
	return tx.store.ListRepositories(filter)
}

func (tx *memoryTransaction) DeleteRepository(id string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledback {
		return ErrTransactionClosed
	}

	tx.deleted[id] = true
	delete(tx.mutations, id)
	return nil
}

func (tx *memoryTransaction) UpdateRepository(id string, updates map[string]interface{}) error {
	repo, err := tx.GetRepository(id)
	if err != nil {
		return err
	}

	// Apply updates to a copy
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
		}
	}

	return tx.SaveRepository(repo)
}

func (tx *memoryTransaction) SaveUser(user *datastore.User) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledback {
		return ErrTransactionClosed
	}

	tx.mutations[user.ID] = user
	return nil
}

func (tx *memoryTransaction) GetUser(id string) (*datastore.User, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if value, exists := tx.mutations[id]; exists {
		if user, ok := value.(*datastore.User); ok {
			return user, nil
		}
	}

	return tx.store.GetUser(id)
}

func (tx *memoryTransaction) GetUserByUsername(username string) (*datastore.User, error) {
	// For simplicity, delegate to store
	return tx.store.GetUserByUsername(username)
}

func (tx *memoryTransaction) ListUsers(filter datastore.UserFilter) ([]*datastore.User, error) {
	return tx.store.ListUsers(filter)
}

func (tx *memoryTransaction) DeleteUser(id string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledback {
		return ErrTransactionClosed
	}

	tx.deleted[id] = true
	delete(tx.mutations, id)
	return nil
}

func (tx *memoryTransaction) UpdateUser(id string, updates map[string]interface{}) error {
	user, err := tx.GetUser(id)
	if err != nil {
		return err
	}

	// Apply updates to a copy
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
		}
	}

	return tx.SaveUser(user)
}

func (tx *memoryTransaction) SaveRef(repoID string, ref *datastore.Reference) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledback {
		return ErrTransactionClosed
	}

	key := repoID + ":" + ref.Name
	tx.mutations[key] = ref
	return nil
}

func (tx *memoryTransaction) GetRef(repoID string, name string) (*datastore.Reference, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	key := repoID + ":" + name
	if value, exists := tx.mutations[key]; exists {
		if ref, ok := value.(*datastore.Reference); ok {
			return ref, nil
		}
	}

	return tx.store.GetRef(repoID, name)
}

func (tx *memoryTransaction) ListRefs(repoID string, refType datastore.RefType) ([]*datastore.Reference, error) {
	return tx.store.ListRefs(repoID, refType)
}

func (tx *memoryTransaction) DeleteRef(repoID string, name string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledback {
		return ErrTransactionClosed
	}

	key := repoID + ":" + name
	tx.deleted[key] = true
	delete(tx.mutations, key)
	return nil
}

func (tx *memoryTransaction) UpdateRef(repoID string, name string, newHash string) error {
	ref, err := tx.GetRef(repoID, name)
	if err != nil {
		return err
	}

	ref.Hash = newHash
	return tx.SaveRef(repoID, ref)
}

func (tx *memoryTransaction) LogEvent(event *datastore.AuditEvent) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledback {
		return ErrTransactionClosed
	}

	tx.mutations[event.ID] = event
	return nil
}

func (tx *memoryTransaction) QueryEvents(filter datastore.EventFilter) ([]*datastore.AuditEvent, error) {
	return tx.store.QueryEvents(filter)
}

func (tx *memoryTransaction) CountEvents(filter datastore.EventFilter) (int64, error) {
	return tx.store.CountEvents(filter)
}

func (tx *memoryTransaction) GetConfig(key string) (string, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if value, exists := tx.mutations[key]; exists {
		if cfg, ok := value.(configValue); ok {
			return cfg.value, nil
		}
	}

	return tx.store.GetConfig(key)
}

func (tx *memoryTransaction) SetConfig(key string, value string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledback {
		return ErrTransactionClosed
	}

	tx.mutations[key] = configValue{key: key, value: value}
	return nil
}

func (tx *memoryTransaction) GetAllConfig() (map[string]string, error) {
	return tx.store.GetAllConfig()
}

func (tx *memoryTransaction) DeleteConfig(key string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledback {
		return ErrTransactionClosed
	}

	tx.deleted[key] = true
	delete(tx.mutations, key)
	return nil
}

var ErrTransactionClosed = fmt.Errorf("transaction is closed")