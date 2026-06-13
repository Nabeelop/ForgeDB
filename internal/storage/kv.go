package storage

import (
	"fmt"
	"forgedb/internal/storage/btree"
	"os"
)

func assert(cond bool) {
	if !cond {
		panic("assertion failed")
	}
}

// KV is the top-level database handle.
// It wires together the B-Tree index and the memory-mapped file backend.
type KV struct {
	Path string

	// internals
	fp   *os.File
	Tree btree.BTree
	free FreeList

	mmap struct {
		file   int      // file size in bytes (may be larger than db size)
		total  int      // total mapped address space (may be larger than file)
		chunks [][]byte // non-contiguous mmap regions
	}

	page struct {
		flushed uint64 // database size in number of pages

		nfree   int // number of pages taken from the free list
		nappend int // number of pages to be appended

		// Newly allocated or deallocated pages keyed by page number.
		// A nil value denotes a deallocated page.
		updates map[uint64][]byte
	}
}

// Get returns the value for key, or nil if not found.
func (db *KV) Get(key []byte) ([]byte, bool) {
	val := db.Tree.Get(key)

	if val == nil {
		return nil, false
	}

	return val, true
}

// Updates or inserts a key-value pair
func (db *KV) Update(
	req *btree.InsertReq,
) (bool, error) {

	db.Tree.InsertEx(req)

	return req.Added, nil
}

// Put inserts or updates a key-value pair.
func (db *KV) Set(
	req *btree.InsertReq,
) error {

	_, err := db.Update(
		req,
	)

	if err != nil {
		return err
	}

	return flushPages(db)
}

// Delete removes a key. Returns false if the key was not found.
func (db *KV) Del(
	req *btree.DeleteReq,
) (bool, error) {

	deleted := db.Tree.DeleteEx(req)

	return deleted, flushPages(db)
}

// persist the newly allocated pages after updates
func flushPages(db *KV) error {
	if err := writePages(db); err != nil {
		return err
	}
	return syncPages(db)
}

func writePages(db *KV) error {
	// update the free list
	freed := []uint64{}

	for ptr, page := range db.page.updates {
		if page == nil {
			freed = append(freed, ptr)
		}
	}

	db.free.Update(db.page.nfree, freed)

	// extend the file & mmap if needed
	npages := int(db.page.flushed) + db.page.nappend
	if err := extendFile(db, npages); err != nil {
		return err
	}
	if err := extendMmap(db, npages); err != nil {
		return err
	}

	// copy pages to the file
	for ptr, page := range db.page.updates {
		if page != nil {
			copy(pageGetMapped(db, ptr).Data(), page)
		}
	}

	return nil
}

// Flush pages to disk and commit the update.
func syncPages(db *KV) error {

	// Flush newly written pages first.
	// This must happen before updating the master page.
	if err := db.fp.Sync(); err != nil {
		return fmt.Errorf("fsync: %w", err)
	}

	// These pages are now durable.
	db.page.flushed += uint64(db.page.nappend)

	// Clear temporary page buffer.
	db.page.nappend = 0
	db.page.nfree = 0
	db.page.updates = make(map[uint64][]byte)

	// Update the master page
	// (root pointer + number of used pages).
	if err := masterStore(db); err != nil {
		return err
	}

	// Flush the master page.
	if err := db.fp.Sync(); err != nil {
		return fmt.Errorf("fsync: %w", err)
	}

	return nil
}

func (db *KV) Open() error {

	// Open or create the database file.
	fp, err := os.OpenFile(
		db.Path,
		os.O_RDWR|os.O_CREATE,
		0644,
	)
	if err != nil {
		return fmt.Errorf("OpenFile: %w", err)
	}

	db.fp = fp

	// Create the initial mmap.
	sz, chunk, err := mmapInit(db.fp)
	if err != nil {
		goto fail
	}

	db.mmap.file = sz
	db.mmap.total = len(chunk)
	db.mmap.chunks = [][]byte{chunk}

	// BTree callbacks.
	db.Tree.SetCallbacks(db.pageGet, db.pageNew, db.pageDel)

	//Freelist Callbacks
	db.free = FreeList{
		get: db.pageGet,
		new: db.pageAppend,
		use: db.pageUse,
	}

	db.page.updates = make(map[uint64][]byte)

	// Load the master page.
	err = masterLoad(db)
	if err != nil {
		goto fail
	}

	return nil

fail:
	db.Close()
	return err
}

// Open opens (or creates) the database file at path and returns a KV handle.
func Open(path string) (*KV, error) {
	db := &KV{Path: path}
	if err := db.Open(); err != nil {
		return nil, err
	}
	return db, nil
}

// Close closes the underlying file handle.
func (db *KV) Close() error {
	closeMmap(db)

	if db.fp != nil {
		return db.fp.Close()
	}
	return nil
}
