package storage

import (
	"fmt"
	"sync"
)

// DB coordinates the B-Tree indexing layer and the raw page file I/O.
type DB struct {
	pager *Pager
	mu    sync.RWMutex
	root  uint64 // Page number of the B-Tree root node
}

// Open opens a database file at the given path.
// If the file is empty, it initializes the first page (page 0) as an empty leaf node.
func Open(path string) (*DB, error) {
	pager, err := NewPager(path)
	if err != nil {
		return nil, err
	}

	db := &DB{
		pager: pager,
		root:  0,
	}

	// Initialize database with an empty root page if it is new
	if pager.TotalPages() == 0 {
		rootNode := NewBTreeNode(NodeTypeLeaf)
		data, err := rootNode.Serialize()
		if err != nil {
			pager.Close()
			return nil, fmt.Errorf("failed to serialize root node: %w", err)
		}
		if err := pager.WritePage(0, data); err != nil {
			pager.Close()
			return nil, fmt.Errorf("failed to write root node page: %w", err)
		}
		if err := pager.Sync(); err != nil {
			pager.Close()
			return nil, fmt.Errorf("failed to sync initial database: %w", err)
		}
	}

	return db, nil
}

// Get retrieves the value associated with a key from the database.
func (db *DB) Get(key []byte) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	// Implement database read traversal here:
	// 1. Fetch node at page number db.root (use db.pager.ReadPage)
	// 2. Deserialize B-Tree node
	// 3. Find key or traverse down children if internal node
	// 4. Return value if found in leaf node

	return nil, fmt.Errorf("Get operation not implemented yet")
}

// Put inserts or updates a key-value pair in the database.
func (db *DB) Put(key, value []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Implement database write/insert here:
	// 1. Traverse down to target leaf node
	// 2. Insert/update key-value pair
	// 3. If leaf node is full, implement split operations recursively upwards
	// 4. Serialize updated nodes and write back using db.pager.WritePage
	// 5. Sync updates to disk using db.pager.Sync

	return fmt.Errorf("Put operation not implemented yet")
}

// Close closes the database engine and releases the file resources.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.pager.Close()
}
