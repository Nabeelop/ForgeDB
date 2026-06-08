package storage

import (
	"errors"
	"fmt"
	"os"
)

// PageSize is the standard unit of disk I/O in our database.
const PageSize = 4096

// Pager handles reading and writing fixed-size pages from a file.
type Pager struct {
	file *os.File
	size int64
}

// NewPager opens or creates a file for database storage and returns a Pager.
func NewPager(path string) (*Pager, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open storage file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat storage file: %w", err)
	}

	if info.Size()%PageSize != 0 {
		file.Close()
		return nil, errors.New("database file size is not a multiple of PageSize")
	}

	return &Pager{
		file: file,
		size: info.Size(),
	}, nil
}

// ReadPage reads a page from the storage file at the given page number (0-indexed).
func (p *Pager) ReadPage(pageNumber uint64) ([]byte, error) {
	offset := int64(pageNumber * PageSize)
	if offset >= p.size {
		return nil, fmt.Errorf("page number %d is out of bounds", pageNumber)
	}

	page := make([]byte, PageSize)
	_, err := p.file.ReadAt(page, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	return page, nil
}

// WritePage writes a page to the storage file at the given page number.
func (p *Pager) WritePage(pageNumber uint64, data []byte) error {
	if len(data) != PageSize {
		return fmt.Errorf("invalid data size: expected %d, got %d", PageSize, len(data))
	}

	offset := int64(pageNumber * PageSize)
	_, err := p.file.WriteAt(data, offset)
	if err != nil {
		return fmt.Errorf("failed to write page: %w", err)
	}

	// Expand current pager size tracked if writing beyond old file boundaries
	if offset >= p.size {
		p.size = offset + PageSize
	}

	return nil
}

// TotalPages returns the number of pages currently in the storage file.
func (p *Pager) TotalPages() uint64 {
	return uint64(p.size / PageSize)
}

// Sync flushes all writes to disk.
func (p *Pager) Sync() error {
	return p.file.Sync()
}

// Close closes the underlying storage file.
func (p *Pager) Close() error {
	return p.file.Close()
}
