package bolt

import "github.com/Caia-Tech/govc/datastore"

// ComprehensiveStubTransaction implements all Transaction methods
type ComprehensiveStubTransaction struct {
	*StubObjectStore
	*StubMetadataStore
}

func NewStubTransaction() *ComprehensiveStubTransaction {
	return &ComprehensiveStubTransaction{
		StubObjectStore:   &StubObjectStore{},
		StubMetadataStore: &StubMetadataStore{},
	}
}

func (t *ComprehensiveStubTransaction) Commit() error {
	return datastore.ErrNotImplemented
}

func (t *ComprehensiveStubTransaction) Rollback() error {
	return nil
}