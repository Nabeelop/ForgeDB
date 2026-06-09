//go:build linux || darwin

package storage

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"forgedb/internal/storage/btree"
)

// mmapInit creates the initial memory mapping covering the whole file.
func mmapInit(fp *os.File) (int, []byte, error) {
	fi, err := fp.Stat()
	if err != nil {
		return 0, nil, fmt.Errorf("stat: %w", err)
	}

	// Database file must contain whole pages.
	if fi.Size()%btree.BTREE_PAGE_SIZE != 0 {
		return 0, nil, errors.New("file size is not a multiple of page size")
	}

	// Start with a 64 MB mapping and double until it covers the entire file.
	mmapSize := 64 << 20
	assert(mmapSize%btree.BTREE_PAGE_SIZE == 0)
	for mmapSize < int(fi.Size()) {
		mmapSize *= 2
	}

	chunk, err := syscall.Mmap(
		int(fp.Fd()),
		0,
		mmapSize,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED,
	)
	if err != nil {
		return 0, nil, fmt.Errorf("mmap: %w", err)
	}

	return int(fi.Size()), chunk, nil
}

// extendMmap grows the mapped address space until it covers npages pages.
func extendMmap(db *KV, npages int) error {
	// Keep doubling the mapped space until it's large enough.
	for db.mmap.total < npages*btree.BTREE_PAGE_SIZE {
		chunkSize := db.mmap.total
		if chunkSize == 0 {
			chunkSize = 64 << 20
		}

		chunk, err := syscall.Mmap(
			int(db.fp.Fd()),
			int64(db.mmap.total), // start new mapping where the last one ended
			chunkSize,            // map the same size again (doubles total)
			syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED,
		)
		if err != nil {
			return fmt.Errorf("mmap: %w", err)
		}

		db.mmap.total += chunkSize
		db.mmap.chunks = append(db.mmap.chunks, chunk)
	}

	return nil
}


// Extend the file to at least npages pages.
func extendFile(db *KV, npages int) error {

	filePages := db.mmap.file / btree.BTREE_PAGE_SIZE

	if filePages >= npages {
		return nil
	}

	for filePages < npages {

		// Grow exponentially so we don't
		// extend the file every update.
		inc := filePages / 8

		if inc < 1 {
			inc = 1
		}

		filePages += inc
	}

	fileSize := filePages * btree.BTREE_PAGE_SIZE

	err := syscall.Fallocate(
		int(db.fp.Fd()),
		0,
		0,
		int64(fileSize),
	)

	if err != nil {
		return fmt.Errorf(
			"fallocate: %w",
			err,
		)
	}

	db.mmap.file = fileSize

	return nil
}


func closeMmap(db *KV) {
	for _, chunk := range db.mmap.chunks {
		err := syscall.Munmap(chunk)
		assert(err == nil)
	}
}
