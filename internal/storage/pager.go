package storage

import "forgedb/internal/storage/btree"

// Callback for BTree & FreeList.
// Dereference a page pointer and return the page.
func (db *KV) pageGet(ptr uint64) btree.BNode {
	if page, ok := db.page.updates[ptr]; ok {
		assert(page != nil)
		return btree.NewBNode(page) // newly allocated/modified page
	}

	return pageGetMapped(db, ptr) // page already written to disk
}

func pageGetMapped(db *KV, ptr uint64) btree.BNode {
	start := uint64(0)

	for _, chunk := range db.mmap.chunks {
		end := start + uint64(len(chunk))/btree.BTREE_PAGE_SIZE

		if ptr < end {
			offset := btree.BTREE_PAGE_SIZE * (ptr - start)

			return btree.NewBNode(chunk[offset : offset+btree.BTREE_PAGE_SIZE])
		}

		start = end
	}

	panic("bad ptr")
}

// Callback for BTree.
// Allocate a new page.
func (db *KV) pageNew(node btree.BNode) uint64 {
	assert(len(node.Data()) <= btree.BTREE_PAGE_SIZE)

	var ptr uint64

	if db.page.nfree < int(db.free.Total()) {
		// Reuse a deallocated page.
		ptr = db.free.Get(db.page.nfree)
		db.page.nfree++
	} else {
		// Append a new page.
		ptr = db.page.flushed + uint64(db.page.nappend)
		db.page.nappend++
	}

	db.page.updates[ptr] = node.Data()

	return ptr
}

// pageDel is the callback for BTree to deallocate a page.

func (db *KV) pageDel(ptr uint64) {
	db.page.updates[ptr] = nil
}
