package storage

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

const BTREE_PAGE_SIZE = 4096

// Create the initial mmap that covers the whole file.
func mmapInit(fp *os.File) (int, []byte, error) {

	fi, err := fp.Stat()
	if err != nil {
		return 0, nil, fmt.Errorf("stat: %w", err)
	}

	// Database file must contain whole pages.
	if fi.Size()%BTREE_PAGE_SIZE != 0 {
		return 0, nil,
			errors.New("file size is not a multiple of page size")
	}

	// Start with a 64 MB mapping.
	mmapSize := 64 << 20

	assert(mmapSize%BTREE_PAGE_SIZE == 0)

	// Grow mapping until it can cover the entire file.
	for mmapSize < int(fi.Size()) {
		mmapSize *= 2
	}

	// mmap size may be larger than the actual file size.
	chunk, err := syscall.Mmap(
		int(fp.Fd()), // file descriptor
		0,            // file offset
		mmapSize,     // mapping length
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED,
	)

	if err != nil {
		return 0, nil,
			fmt.Errorf("mmap: %w", err)
	}

	return int(fi.Size()), chunk, nil
}

type KV struct {
	Path string 
	//internals

	fp *os.File
	tree Btree
	mmap struct {
		file int          //file size,can be larger than db size
		total int         //mmap size can be larger than the file size
		chunks [][]byte  //multiple mmaps,can be non contiguous
	}

	page struct{
		flushed uint64   //number of pages flushed to the disk
		temp[][]byte  //stores new pages temporarily until flushed to the disk
	}
}



// Extend the mmap by adding new mappings.
func extendMmap(db *KV, npages int) error {

	if db.mmap.total >= npages*BTREE_PAGE_SIZE {
		return nil
	}

	// Double the mapped address space.
	chunk, err := syscall.Mmap(
		int(db.fp.Fd()),      // file descriptor
		int64(db.mmap.total), // offset in file
		db.mmap.total,        // length of new mapping
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED,
	)

	if err != nil {
		return fmt.Errorf("mmap: %w", err)
	}

	db.mmap.total += db.mmap.total

	db.mmap.chunks = append(
		db.mmap.chunks,
		chunk,
	)

	return nil
}


