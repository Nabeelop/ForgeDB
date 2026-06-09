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
	tree btree.BTree

	mmap struct {
		file   int      // file size in bytes (may be larger than db size)
		total  int      // total mapped address space (may be larger than file)
		chunks [][]byte // non-contiguous mmap regions
	}

	page struct {
		flushed uint64   // number of pages flushed to disk
		temp    [][]byte // new pages buffered until the next flush
	}
}

// Get returns the value for key, or nil if not found.
func (db *KV) Get(key []byte) ([]byte, bool) {
	val := db.tree.Get(key)

	if val == nil {
		return nil, false
	}

	return val, true
}

// Put inserts or updates a key-value pair.
func (db *KV)Set(key []byte, val[]byte)error{
db.tree.Insert(key,val)
return flushPages(db)
}

// Delete removes a key. Returns false if the key was not found.
func (db *KV)Del(key []byte)(bool,error){
deleted :=db.tree.Delete(key)
return deleted,flushPages(db)
}

//persistthe newly allocatedpages after updates
func flushPages(db *KV)error {
if err :=writePages(db); err!=nil {
return err
}
return syncPages(db)
}

func writePages(db *KV)error {
//extendthe file &mmap ifneeded
npages :=int(db.page.flushed)+ len(db.page.temp)
if err :=extendFile(db, npages);err !=nil {
return err
}
	if err := extendMmap(db, npages); err != nil {
		return err
	}
	//copy data tothe file
	for i, page := range db.page.temp {
		ptr := db.page.flushed + uint64(i)
		copy(db.pageGet(ptr).Data(), page)
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
	db.page.flushed += uint64(len(db.page.temp))

	// Clear temporary page buffer.
	db.page.temp = db.page.temp[:0]

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
	db.tree.SetCallbacks(db.pageGet, db.pageNew, db.pageDel)

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
