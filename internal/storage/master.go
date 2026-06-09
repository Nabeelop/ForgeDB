package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"forgedb/internal/storage/btree"
)

const DB_SIG = "BuildYourOwnDB05"

// Read and verify the master page.
func masterLoad(db *KV) error {

	// Empty database file.
	if db.mmap.file == 0 {
		// Reserve page 0 for the master page.
		db.page.flushed = 1
		return nil
	}

	data := db.mmap.chunks[0]

	root := binary.LittleEndian.Uint64(data[16:24])
	used := binary.LittleEndian.Uint64(data[24:32])

	// Verify signature.
	if !bytes.Equal([]byte(DB_SIG), data[:16]) {
		return errors.New("bad signature")
	}

	bad := !(1 <= used && used <= uint64(db.mmap.file/btree.BTREE_PAGE_SIZE))
	bad = bad || !(root < used)

	if bad {
		return errors.New("bad master page")
	}

	db.tree.SetRoot(root)
	db.page.flushed = used

	return nil
}

// update the master page. it must be atomic.
func masterStore(db *KV) error {
	var data [32]byte
	copy(data[:16], []byte(DB_SIG))
	binary.LittleEndian.PutUint64(data[16:], db.tree.GetRoot())
	binary.LittleEndian.PutUint64(data[24:], db.page.flushed)

	// NOTE: Updating the page via mmap is not atomic.
	// Use the `pwrite()` syscall instead.
	_, err := db.fp.WriteAt(data[:], 0)
	if err != nil {
		return fmt.Errorf("write master page: %w", err)
	}
	return nil
}