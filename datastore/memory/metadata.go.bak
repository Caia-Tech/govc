package memory

import (
	"fmt"
	"strings"
	"time"

	"github.com/caiatech/govc/datastore"
	"github.com/google/uuid"
)

// ListRepositories lists repositories with filtering
func (m *MemoryStore) ListRepositories(filter datastore.RepositoryFilter) ([]*datastore.Repository, error) {
	m.repositoriesMu.RLock()
	defer m.repositoriesMu.RUnlock()

	var results []*datastore.Repository
	
	for _, repo := range m.repositories {
		// Apply filters
		if len(filter.IDs) > 0 && !contains(filter.IDs, repo.ID) {
			continue
		}
		if filter.Name != "" && !strings.Contains(repo.Name, filter.Name) {
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
		
		results = append(results, repo)
	}
	
	// Apply offset and limit
	if filter.Offset > 0 && filter.Offset < len(results) {
		results = results[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(results) {
		results = results[:filter.Limit]
	}
	
	m.reads.Add(1)
	return results, nil
}

// DeleteRepository deletes a repository
func (m *MemoryStore) DeleteRepository(id string) error {
	m.repositoriesMu.Lock()
	defer m.repositoriesMu.Unlock()
	
	if _, exists := m.repositories[id]; !exists {
		return datastore.ErrNotFound
	}
	
	delete(m.repositories, id)
	m.deletes.Add(1)
	
	// Also delete associated refs
	m.refsMu.Lock()
	for key := range m.refs {
		if strings.HasPrefix(key, id+":") {
			delete(m.refs, key)
		}
	}
	m.refsMu.Unlock()
	
	return nil
}

// UpdateRepository updates a repository
func (m *MemoryStore) UpdateRepository(id string, updates map[string]interface{}) error {
	m.repositoriesMu.Lock()
	defer m.repositoriesMu.Unlock()
	
	repo, exists := m.repositories[id]
	if !exists {
		return datastore.ErrNotFound
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
	m.writes.Add(1)
	return nil
}

// SaveUser saves a user
func (m *MemoryStore) SaveUser(user *datastore.User) error {
	if user.ID == "" {
		user.ID = uuid.New().String()
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}
	user.UpdatedAt = time.Now()
	
	m.usersMu.Lock()
	defer m.usersMu.Unlock()
	
	// Check for duplicate username/email
	if existing, exists := m.usersByName[user.Username]; exists && existing.ID != user.ID {
		return fmt.Errorf("username already exists: %s", user.Username)
	}
	if existing, exists := m.usersByEmail[user.Email]; exists && existing.ID != user.ID {
		return fmt.Errorf("email already exists: %s", user.Email)
	}
	
	// Remove old username/email mappings if updating
	if oldUser, exists := m.users[user.ID]; exists {
		delete(m.usersByName, oldUser.Username)
		delete(m.usersByEmail, oldUser.Email)
	}
	
	m.users[user.ID] = user
	m.usersByName[user.Username] = user
	m.usersByEmail[user.Email] = user
	m.writes.Add(1)
	return nil
}

// GetUser retrieves a user by ID
func (m *MemoryStore) GetUser(id string) (*datastore.User, error) {
	m.usersMu.RLock()
	defer m.usersMu.RUnlock()
	
	m.reads.Add(1)
	if user, exists := m.users[id]; exists {
		return user, nil
	}
	return nil, datastore.ErrNotFound
}

// GetUserByUsername retrieves a user by username
func (m *MemoryStore) GetUserByUsername(username string) (*datastore.User, error) {
	m.usersMu.RLock()
	defer m.usersMu.RUnlock()
	
	m.reads.Add(1)
	if user, exists := m.usersByName[username]; exists {
		return user, nil
	}
	return nil, datastore.ErrNotFound
}

// ListUsers lists users with filtering
func (m *MemoryStore) ListUsers(filter datastore.UserFilter) ([]*datastore.User, error) {
	m.usersMu.RLock()
	defer m.usersMu.RUnlock()
	
	var results []*datastore.User
	
	for _, user := range m.users {
		// Apply filters
		if len(filter.IDs) > 0 && !contains(filter.IDs, user.ID) {
			continue
		}
		if filter.Username != "" && !strings.Contains(user.Username, filter.Username) {
			continue
		}
		if filter.Email != "" && !strings.Contains(user.Email, filter.Email) {
			continue
		}
		if filter.IsActive != nil && user.IsActive != *filter.IsActive {
			continue
		}
		if filter.IsAdmin != nil && user.IsAdmin != *filter.IsAdmin {
			continue
		}
		
		results = append(results, user)
	}
	
	// Apply offset and limit
	if filter.Offset > 0 && filter.Offset < len(results) {
		results = results[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(results) {
		results = results[:filter.Limit]
	}
	
	m.reads.Add(1)
	return results, nil
}

// DeleteUser deletes a user
func (m *MemoryStore) DeleteUser(id string) error {
	m.usersMu.Lock()
	defer m.usersMu.Unlock()
	
	user, exists := m.users[id]
	if !exists {
		return datastore.ErrNotFound
	}
	
	delete(m.users, id)
	delete(m.usersByName, user.Username)
	delete(m.usersByEmail, user.Email)
	m.deletes.Add(1)
	return nil
}

// UpdateUser updates a user
func (m *MemoryStore) UpdateUser(id string, updates map[string]interface{}) error {
	m.usersMu.Lock()
	defer m.usersMu.Unlock()
	
	user, exists := m.users[id]
	if !exists {
		return datastore.ErrNotFound
	}
	
	// Apply updates
	for key, value := range updates {
		switch key {
		case "username":
			if v, ok := value.(string); ok {
				delete(m.usersByName, user.Username)
				user.Username = v
				m.usersByName[v] = user
			}
		case "email":
			if v, ok := value.(string); ok {
				delete(m.usersByEmail, user.Email)
				user.Email = v
				m.usersByEmail[v] = user
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
		case "last_login_at":
			if v, ok := value.(time.Time); ok {
				user.LastLoginAt = &v
			}
		}
	}
	
	user.UpdatedAt = time.Now()
	m.writes.Add(1)
	return nil
}

// SaveRef saves a reference
func (m *MemoryStore) SaveRef(repoID string, ref *datastore.Reference) error {
	if ref.UpdatedAt.IsZero() {
		ref.UpdatedAt = time.Now()
	}
	
	m.refsMu.Lock()
	defer m.refsMu.Unlock()
	
	key := fmt.Sprintf("%s:%s", repoID, ref.Name)
	m.refs[key] = ref
	m.writes.Add(1)
	return nil
}

// GetRef retrieves a reference
func (m *MemoryStore) GetRef(repoID string, name string) (*datastore.Reference, error) {
	m.refsMu.RLock()
	defer m.refsMu.RUnlock()
	
	key := fmt.Sprintf("%s:%s", repoID, name)
	m.reads.Add(1)
	
	if ref, exists := m.refs[key]; exists {
		return ref, nil
	}
	return nil, datastore.ErrNotFound
}

// ListRefs lists references for a repository
func (m *MemoryStore) ListRefs(repoID string, refType datastore.RefType) ([]*datastore.Reference, error) {
	m.refsMu.RLock()
	defer m.refsMu.RUnlock()
	
	var results []*datastore.Reference
	prefix := repoID + ":"
	
	for key, ref := range m.refs {
		if strings.HasPrefix(key, prefix) {
			if refType == "" || ref.Type == refType {
				results = append(results, ref)
			}
		}
	}
	
	m.reads.Add(1)
	return results, nil
}

// DeleteRef deletes a reference
func (m *MemoryStore) DeleteRef(repoID string, name string) error {
	m.refsMu.Lock()
	defer m.refsMu.Unlock()
	
	key := fmt.Sprintf("%s:%s", repoID, name)
	if _, exists := m.refs[key]; !exists {
		return datastore.ErrNotFound
	}
	
	delete(m.refs, key)
	m.deletes.Add(1)
	return nil
}

// UpdateRef updates a reference hash
func (m *MemoryStore) UpdateRef(repoID string, name string, newHash string) error {
	m.refsMu.Lock()
	defer m.refsMu.Unlock()
	
	key := fmt.Sprintf("%s:%s", repoID, name)
	ref, exists := m.refs[key]
	if !exists {
		return datastore.ErrNotFound
	}
	
	ref.Hash = newHash
	ref.UpdatedAt = time.Now()
	m.writes.Add(1)
	return nil
}

// LogEvent logs an audit event
func (m *MemoryStore) LogEvent(event *datastore.AuditEvent) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	m.eventsMu.Lock()
	defer m.eventsMu.Unlock()
	
	m.events = append(m.events, event)
	m.writes.Add(1)
	
	// Keep only last 10000 events to prevent unbounded growth
	if len(m.events) > 10000 {
		m.events = m.events[len(m.events)-10000:]
	}
	
	return nil
}

// QueryEvents queries audit events
func (m *MemoryStore) QueryEvents(filter datastore.EventFilter) ([]*datastore.AuditEvent, error) {
	m.eventsMu.RLock()
	defer m.eventsMu.RUnlock()
	
	var results []*datastore.AuditEvent
	
	for _, event := range m.events {
		// Apply filters
		if filter.UserID != "" && event.UserID != filter.UserID {
			continue
		}
		if filter.Action != "" && !strings.Contains(event.Action, filter.Action) {
			continue
		}
		if filter.Resource != "" && event.Resource != filter.Resource {
			continue
		}
		if filter.ResourceID != "" && event.ResourceID != filter.ResourceID {
			continue
		}
		if filter.After != nil && event.Timestamp.Before(*filter.After) {
			continue
		}
		if filter.Before != nil && event.Timestamp.After(*filter.Before) {
			continue
		}
		if filter.Success != nil && event.Success != *filter.Success {
			continue
		}
		
		results = append(results, event)
	}
	
	// Apply offset and limit
	if filter.Offset > 0 && filter.Offset < len(results) {
		results = results[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(results) {
		results = results[:filter.Limit]
	}
	
	m.reads.Add(1)
	return results, nil
}

// CountEvents counts audit events
func (m *MemoryStore) CountEvents(filter datastore.EventFilter) (int64, error) {
	events, err := m.QueryEvents(filter)
	if err != nil {
		return 0, err
	}
	return int64(len(events)), nil
}

// GetConfig retrieves a configuration value
func (m *MemoryStore) GetConfig(key string) (string, error) {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	
	m.reads.Add(1)
	if value, exists := m.config[key]; exists {
		return value, nil
	}
	return "", datastore.ErrNotFound
}

// SetConfig sets a configuration value
func (m *MemoryStore) SetConfig(key string, value string) error {
	m.configMu.Lock()
	defer m.configMu.Unlock()
	
	m.config[key] = value
	m.writes.Add(1)
	return nil
}

// GetAllConfig retrieves all configuration
func (m *MemoryStore) GetAllConfig() (map[string]string, error) {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	
	// Return a copy to prevent external modifications
	result := make(map[string]string, len(m.config))
	for k, v := range m.config {
		result[k] = v
	}
	
	m.reads.Add(1)
	return result, nil
}

// DeleteConfig deletes a configuration value
func (m *MemoryStore) DeleteConfig(key string) error {
	m.configMu.Lock()
	defer m.configMu.Unlock()
	
	if _, exists := m.config[key]; !exists {
		return datastore.ErrNotFound
	}
	
	delete(m.config, key)
	m.deletes.Add(1)
	return nil
}