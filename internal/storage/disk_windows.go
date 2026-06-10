//go:build windows

package storage

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"forgedb/internal/storage/btree"
)

// Windows file-mapping access flags (from winbase.h).
const (
	fileMapWrite = uint32(0x0002) // FILE_MAP_WRITE (implies read)
)

// mmapInit creates the initial memory-mapped region for the database file.
// Windows equivalent of Unix mmap with MAP_SHARED | PROT_READ | PROT_WRITE.
func mmapInit(fp *os.File) (int, []byte, error) {
	fi, err := fp.Stat()
	if err != nil {
		return 0, nil, fmt.Errorf("stat: %w", err)
	}

	if fi.Size()%int64(btree.BTREE_PAGE_SIZE) != 0 {
		return 0, nil, errors.New("file size is not a multiple of page size")
	}

	// Start with 64 MB and grow until it covers the whole file.
	mmapSize := 64 << 20
	assert(mmapSize%btree.BTREE_PAGE_SIZE == 0)
	for mmapSize < int(fi.Size()) {
		mmapSize *= 2
	}

	// Windows mmap cannot exceed the physical file size, so extend it first.
	if fi.Size() < int64(mmapSize) {
		if err := fp.Truncate(int64(mmapSize)); err != nil {
			return 0, nil, fmt.Errorf("truncate: %w", err)
		}
	}

	// Create a named file-mapping object backed by the file.
	handle, err := syscall.CreateFileMapping(
		syscall.Handle(fp.Fd()),
		nil,
		syscall.PAGE_READWRITE,
		uint32(uint64(mmapSize)>>32), // high 32 bits of max size
		uint32(mmapSize),             // low  32 bits of max size
		nil,
	)
	if err != nil {
		return 0, nil, fmt.Errorf("CreateFileMapping: %w", err)
	}

	// Map the view. The handle can be closed immediately after — the view
	// stays valid until UnmapViewOfFile is called.
	addr, err := syscall.MapViewOfFile(
		handle,
		fileMapWrite,
		0, 0, // offset = 0
		uintptr(mmapSize),
	)
	_ = syscall.CloseHandle(handle)
	if err != nil {
		return 0, nil, fmt.Errorf("MapViewOfFile: %w", err)
	}

	chunk := unsafe.Slice((*byte)(unsafe.Pointer(addr)), mmapSize)
	return int(fi.Size()), chunk, nil
}

// extendMmap grows the memory-mapped address space by appending a new chunk.
// Each call doubles the total mapped space, matching the Unix implementation.
func extendMmap(db *KV, npages int) error {
	for db.mmap.total < npages*btree.BTREE_PAGE_SIZE {
		// New chunk is the same size as all existing chunks combined → doubles total.
		chunkSize := db.mmap.total
		if chunkSize == 0 {
			chunkSize = 64 << 20
		}
		newTotal := db.mmap.total + chunkSize

		// Ensure the physical file is large enough to back the new region.
		fi, err := db.fp.Stat()
		if err != nil {
			return fmt.Errorf("stat: %w", err)
		}
		if fi.Size() < int64(newTotal) {
			if err := db.fp.Truncate(int64(newTotal)); err != nil {
				return fmt.Errorf("truncate: %w", err)
			}
		}

		// Create a new file-mapping object covering the full (extended) file.
		handle, err := syscall.CreateFileMapping(
			syscall.Handle(db.fp.Fd()),
			nil,
			syscall.PAGE_READWRITE,
			uint32(uint64(newTotal)>>32),
			uint32(newTotal),
			nil,
		)
		if err != nil {
			return fmt.Errorf("CreateFileMapping: %w", err)
		}

		// Map only the new chunk, starting at the current end of the mapping.
		// Note: offset must be a multiple of the system allocation granularity
		// (64 KB on Windows). Our chunks are always multiples of 64 MB so ✅.
		offsetHigh := uint32(uint64(db.mmap.total) >> 32)
		offsetLow := uint32(db.mmap.total)

		addr, err := syscall.MapViewOfFile(
			handle,
			fileMapWrite,
			offsetHigh,
			offsetLow,
			uintptr(chunkSize),
		)
		_ = syscall.CloseHandle(handle)
		if err != nil {
			return fmt.Errorf("MapViewOfFile: %w", err)
		}

		chunk := unsafe.Slice((*byte)(unsafe.Pointer(addr)), chunkSize)
		db.mmap.chunks = append(db.mmap.chunks, chunk)
		db.mmap.total = newTotal
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

	fi, err := db.fp.Stat()
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if fi.Size() < int64(fileSize) {
		err := db.fp.Truncate(int64(fileSize))
		if err != nil {
			return fmt.Errorf("truncate: %w", err)
		}
	}

	db.mmap.file = fileSize

	return nil
}

func closeMmap(db *KV) {
	for _, chunk := range db.mmap.chunks {
		if len(chunk) == 0 {
			continue
		}
		addr := uintptr(unsafe.Pointer(&chunk[0]))
		err := syscall.UnmapViewOfFile(addr)
		assert(err == nil)
	}
}
