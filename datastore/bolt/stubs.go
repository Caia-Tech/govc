package bolt

import (
	"github.com/Caia-Tech/govc/datastore"
)

// StubMetadataStore provides minimal metadata store implementation
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

func (s *StubMetadataStore) UpdateRef(repoID string, name string, newHash string) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) SaveAuditEvent(event *datastore.AuditEvent) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) CountEvents(filter datastore.EventFilter) (int64, error) {
	return 0, datastore.ErrNotImplemented
}

func (s *StubMetadataStore) DeleteConfig(key string) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) GetAllConfig() (map[string]string, error) {
	return nil, datastore.ErrNotImplemented
}

func (s *StubMetadataStore) GetConfig(key string) (string, error) {
	return "", datastore.ErrNotImplemented
}

func (s *StubMetadataStore) SetConfig(key string, value string) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) LogEvent(event *datastore.AuditEvent) error {
	return datastore.ErrNotImplemented
}

func (s *StubMetadataStore) QueryEvents(filter datastore.EventFilter) ([]*datastore.AuditEvent, error) {
	return nil, datastore.ErrNotImplemented
}

// StubTransaction provides minimal transaction implementation
type StubTransaction struct{}

func (t *StubTransaction) Commit() error {
	return datastore.ErrNotImplemented
}

func (t *StubTransaction) Rollback() error {
	return nil
}

func (t *StubTransaction) ObjectStore() datastore.ObjectStore {
	return &StubObjectStore{}
}

func (t *StubTransaction) MetadataStore() datastore.MetadataStore {
	return &StubMetadataStore{}
}

func (t *StubTransaction) CountEvents(filter datastore.EventFilter) (int64, error) {
	return 0, datastore.ErrNotImplemented
}

func (t *StubTransaction) CountObjects() (int64, error) {
	return 0, datastore.ErrNotImplemented
}

// StubObjectStore for transaction
type StubObjectStore struct{}

func (s *StubObjectStore) GetObject(hash string) ([]byte, error) {
	return nil, datastore.ErrNotImplemented
}

func (s *StubObjectStore) PutObject(hash string, data []byte) error {
	return datastore.ErrNotImplemented
}

func (s *StubObjectStore) DeleteObject(hash string) error {
	return datastore.ErrNotImplemented
}

func (s *StubObjectStore) HasObject(hash string) (bool, error) {
	return false, datastore.ErrNotImplemented
}

func (s *StubObjectStore) ListObjects(prefix string, limit int) ([]string, error) {
	return nil, datastore.ErrNotImplemented
}

func (s *StubObjectStore) IterateObjects(prefix string, fn func(hash string, data []byte) error) error {
	return datastore.ErrNotImplemented
}

func (s *StubObjectStore) GetObjects(hashes []string) (map[string][]byte, error) {
	return nil, datastore.ErrNotImplemented
}

func (s *StubObjectStore) PutObjects(objects map[string][]byte) error {
	return datastore.ErrNotImplemented
}

func (s *StubObjectStore) DeleteObjects(hashes []string) error {
	return datastore.ErrNotImplemented
}

func (s *StubObjectStore) GetObjectSize(hash string) (int64, error) {
	return 0, datastore.ErrNotImplemented
}

func (s *StubObjectStore) CountObjects() (int64, error) {
	return 0, datastore.ErrNotImplemented
}

func (s *StubObjectStore) GetStorageSize() (int64, error) {
	return 0, datastore.ErrNotImplemented
}