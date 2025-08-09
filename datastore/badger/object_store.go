package badger

import (
	"fmt"

	"github.com/caiatech/govc/datastore"
	badgerdb "github.com/dgraph-io/badger/v4"
)

// ObjectStore implementation

// GetObject retrieves an object by hash
func (s *BadgerStore) GetObject(hash string) ([]byte, error) {
	s.reads.Add(1)
	
	var data []byte
	err := s.db.View(func(txn *badgerdb.Txn) error {
		item, err := txn.Get(makeObjectKey(hash))
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		return item.Value(func(val []byte) error {
			data = append([]byte{}, val...) // Copy the value
			return nil
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return data, nil
}

// PutObject stores an object
func (s *BadgerStore) PutObject(hash string, data []byte) error {
	s.writes.Add(1)
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		return txn.Set(makeObjectKey(hash), data)
	})
}

// DeleteObject deletes an object
func (s *BadgerStore) DeleteObject(hash string) error {
	s.deletes.Add(1)
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		key := makeObjectKey(hash)
		
		// Check if the key exists first
		_, err := txn.Get(key)
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		// Key exists, now delete it
		return txn.Delete(key)
	})
}

// HasObject checks if an object exists
func (s *BadgerStore) HasObject(hash string) (bool, error) {
	s.reads.Add(1)
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		_, err := txn.Get(makeObjectKey(hash))
		return err
	})
	
	if err == badgerdb.ErrKeyNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	
	return true, nil
}

// ListObjects lists objects with optional prefix filtering
func (s *BadgerStore) ListObjects(prefix string, limit int) ([]string, error) {
	s.reads.Add(1)
	
	var hashes []string
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		opts := badgerdb.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
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
		
		return nil
	})
	
	return hashes, err
}

// IterateObjects iterates over objects
func (s *BadgerStore) IterateObjects(prefix string, fn func(hash string, data []byte) error) error {
	s.reads.Add(1)
	
	return s.db.View(func(txn *badgerdb.Txn) error {
		opts := badgerdb.DefaultIteratorOptions
		it := txn.NewIterator(opts)
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
	})
}

// GetObjects retrieves multiple objects at once
func (s *BadgerStore) GetObjects(hashes []string) (map[string][]byte, error) {
	if len(hashes) == 0 {
		return map[string][]byte{}, nil
	}
	
	s.reads.Add(int64(len(hashes)))
	
	result := make(map[string][]byte)
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		for _, hash := range hashes {
			item, err := txn.Get(makeObjectKey(hash))
			if err == badgerdb.ErrKeyNotFound {
				continue // Skip missing objects
			}
			if err != nil {
				return err
			}
			
			err = item.Value(func(val []byte) error {
				result[hash] = append([]byte{}, val...) // Copy the value
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	
	return result, err
}

// PutObjects stores multiple objects at once
func (s *BadgerStore) PutObjects(objects map[string][]byte) error {
	if len(objects) == 0 {
		return nil
	}
	
	s.writes.Add(int64(len(objects)))
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		for hash, data := range objects {
			if err := txn.Set(makeObjectKey(hash), data); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteObjects deletes multiple objects at once
func (s *BadgerStore) DeleteObjects(hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	
	s.deletes.Add(int64(len(hashes)))
	
	return s.db.Update(func(txn *badgerdb.Txn) error {
		for _, hash := range hashes {
			if err := txn.Delete(makeObjectKey(hash)); err != nil && err != badgerdb.ErrKeyNotFound {
				return err
			}
		}
		return nil
	})
}

// GetObjectSize returns the size of an object
func (s *BadgerStore) GetObjectSize(hash string) (int64, error) {
	s.reads.Add(1)
	
	var size int64
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		item, err := txn.Get(makeObjectKey(hash))
		if err == badgerdb.ErrKeyNotFound {
			return datastore.ErrNotFound
		}
		if err != nil {
			return err
		}
		
		size = int64(item.ValueSize())
		return nil
	})
	
	return size, err
}

// CountObjects returns the total number of objects
func (s *BadgerStore) CountObjects() (int64, error) {
	var count int64
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		opts := badgerdb.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		
		prefix := []byte(prefixObject)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			count++
		}
		
		return nil
	})
	
	return count, err
}

// GetStorageSize returns the total storage size used
func (s *BadgerStore) GetStorageSize() (int64, error) {
	var totalSize int64
	
	err := s.db.View(func(txn *badgerdb.Txn) error {
		opts := badgerdb.DefaultIteratorOptions
		opts.PrefetchSize = 1000
		it := txn.NewIterator(opts)
		defer it.Close()
		
		prefix := []byte(prefixObject)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			totalSize += int64(it.Item().ValueSize())
		}
		
		return nil
	})
	
	return totalSize, err
}

// Vacuum optimizes the database
func (s *BadgerStore) Vacuum() error {
	// Run garbage collection
	err := s.db.RunValueLogGC(0.5)
	if err != nil && err != badgerdb.ErrNoRewrite {
		return fmt.Errorf("failed to run GC: %w", err)
	}
	
	// Flatten the LSM tree
	return s.db.Flatten(8)
}