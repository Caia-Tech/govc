package redis

import (
	"github.com/caiatech/govc/datastore"
)

// StubMetadataStore provides minimal metadata store implementation for Redis
// Redis is primarily used for caching and sessions, not metadata storage
type StubMetadataStore struct{}

func (s *StubMetadataStore) SaveRepository(repo *datastore.Repository) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) GetRepository(id string) (*datastore.Repository, error) {
	return nil, datastore.ErrNotImplemented
}

func (s *StubMetadataStore) ListRepositories(filter datastore.RepositoryFilter) ([]*datastore.Repository, error) {
	return nil, datastore.ErrNotImplemented
}

func (s *StubMetadataStore) DeleteRepository(id string) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) UpdateRepository(id string, updates map[string]interface{}) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) SaveUser(user *datastore.User) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) GetUser(id string) (*datastore.User, error) {
	return nil, datastore.ErrNotImplemented
}

func (s *StubMetadataStore) GetUserByUsername(username string) (*datastore.User, error) {
	return nil, datastore.ErrNotImplemented
}

func (s *StubMetadataStore) ListUsers(filter datastore.UserFilter) ([]*datastore.User, error) {
	return nil, datastore.ErrNotImplemented
}

func (s *StubMetadataStore) DeleteUser(id string) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) UpdateUser(id string, updates map[string]interface{}) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) SaveRef(repoID string, ref *datastore.Reference) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) GetRef(repoID string, name string) (*datastore.Reference, error) {
	return nil, datastore.ErrNotImplemented
}

func (s *StubMetadataStore) ListRefs(repoID string, refType datastore.RefType) ([]*datastore.Reference, error) {
	return nil, datastore.ErrNotImplemented
}

func (s *StubMetadataStore) DeleteRef(repoID string, name string) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) SaveAuditEvent(event *datastore.AuditEvent) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) CountEvents(filter datastore.EventFilter) (int64, error) {
	return 0, datastore.ErrNotImplemented
}

func (s *StubMetadataStore) UpdateRef(repoID string, name string, newHash string) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) LogEvent(event *datastore.AuditEvent) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) QueryEvents(filter datastore.EventFilter) ([]*datastore.AuditEvent, error) {
	return nil, datastore.ErrNotImplemented
}

func (s *StubMetadataStore) GetConfig(key string) (string, error) {
	return "", datastore.ErrNotImplemented
}

func (s *StubMetadataStore) SetConfig(key string, value string) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) GetAllConfig() (map[string]string, error) {
	return nil, datastore.ErrNotImplemented
}

func (s *StubMetadataStore) DeleteConfig(key string) error {
	return datastore.ErrNotImplemented
}