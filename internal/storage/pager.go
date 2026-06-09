package storage

import "forgedb/internal/storage/btree"

// pageGet dereferences a page pointer into a BNode by scanning the mmap chunks.
func (db *KV) pageGet(ptr uint64) btree.BNode {
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

// pageNew is the callback for BTree to allocate a new page.
func (db *KV) pageNew(node btree.BNode) uint64 {
	// TODO: reuse deallocated pages
	assert(len(node.Data()) <= btree.BTREE_PAGE_SIZE)
	ptr := db.page.flushed + uint64(len(db.page.temp))
	db.page.temp = append(db.page.temp, node.Data())
	return ptr
}

// pageDel is the callback for BTree to deallocate a page.
func (db *KV) pageDel(uint64) {
	// TODO: implement this
}