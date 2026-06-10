package storage

import (
	"encoding/binary"
	"forgedb/internal/storage/btree"
)

const BNODE_FREE_LIST = 3
const FREE_LIST_HEADER = 4 + 8 + 8
const FREE_LIST_CAP = (btree.BTREE_PAGE_SIZE - FREE_LIST_HEADER) / 8

//Freelist accessor functions

func flnSize(node btree.BNode) int {
	return int(binary.LittleEndian.Uint16(node.Data()[2:4]))
}

func flnNext(node btree.BNode) uint64 {
	return binary.LittleEndian.Uint64(node.Data()[12:20])
}

func flnPtr(node btree.BNode, idx int) uint64 {
	assert(idx >= 0)
	assert(idx < flnSize(node))
	return binary.LittleEndian.Uint64(node.Data()[20+uint64(idx)*8:])
}

func flnSetPtr(node btree.BNode, idx int, ptr uint64) {
	assert(idx >= 0)
	assert(idx < flnSize(node))
	binary.LittleEndian.PutUint64(node.Data()[20+uint64(idx)*8:], ptr)
}

func flnSetHeader(node btree.BNode, size uint16, next uint64) {
	binary.LittleEndian.PutUint16(node.Data()[0:2], BNODE_FREE_LIST)
	binary.LittleEndian.PutUint16(node.Data()[2:4], size)
	binary.LittleEndian.PutUint64(node.Data()[12:20], next)
}

func flnSetTotal(node btree.BNode, total uint64) {
	binary.LittleEndian.PutUint64(node.Data()[4:12], total)
}

func flnTotal(node btree.BNode) uint64 {
	return binary.LittleEndian.Uint64(node.Data()[4:12])
}

type FreeList struct {
	head uint64
	// callbacks for managing on-disk pages
	get func(uint64) btree.BNode  // dereference a pointer
	new func(btree.BNode) uint64  // append a new page
	use func(uint64, btree.BNode) // reuse a page
}

// Returns the total number of free page pointers
func (fl *FreeList) Total() uint64 {
	ptr := fl.head
	total := 0

	for ptr != 0 {
		node := fl.get(ptr)
		total += flnSize(node)
		ptr = flnNext(node)
	}
	return uint64(total)
}

// Get returns the (topn+1)-th smallest page pointer from the free list.
func (fl *FreeList) Get(topn int) uint64 {
	assert(0 <= topn && topn < int(fl.Total()))
	node := fl.get(fl.head)
	for flnSize(node) <= topn {
		topn -= flnSize(node)
		next := flnNext(node)
		assert(next != 0)
		node = fl.get(next)
	}
	return flnPtr(node, flnSize(node)-topn-1)
}

// remove `popn` pointers and add some new pointers
func (fl *FreeList) Update(popn int, freed []uint64) {
	assert(popn <= int(fl.Total()))

	if popn == 0 && len(freed) == 0 {
		return // nothing to do
	}

	// prepare to construct the new list
	total := fl.Total()
	reuse := []uint64{}

	for fl.head != 0 && len(reuse)*FREE_LIST_CAP < len(freed) {
		node := fl.get(fl.head)

		// recycle the node page itself
		freed = append(freed, fl.head)

		if popn >= flnSize(node) {
			// Phase 1:
			// remove all pointers in this node
			popn -= flnSize(node)

		} else {
			// Phase 2:
			// remove some pointers

			remain := flnSize(node) - popn
			popn = 0

			// reuse pointers from the free list itself
			for remain > 0 &&
				len(reuse)*FREE_LIST_CAP < len(freed)+remain {

				remain--
				reuse = append(reuse, flnPtr(node, remain))
			}

			// move remaining pointers into freed
			for i := 0; i < remain; i++ {
				freed = append(freed, flnPtr(node, i))
			}
		}

		// discard current node
		total -= uint64(flnSize(node))
		fl.head = flnNext(node)
	}

	assert(
		len(reuse)*FREE_LIST_CAP >= len(freed) ||
			fl.head == 0,
	)

	// Phase 3:
	// prepend new nodes
	flPush(fl, freed, reuse)

	// update total count
	flnSetTotal(
		fl.get(fl.head),
		total+uint64(len(freed)),
	)
}

func flPush(fl *FreeList, freed []uint64, reuse []uint64) {
	for len(freed) > 0 {

		new := btree.NewBNode(make([]byte, btree.BTREE_PAGE_SIZE))

		//construct a new node
		size := len(freed)
		if size > FREE_LIST_CAP {
			size = FREE_LIST_CAP
		}
		flnSetHeader(new, uint16(size), fl.head)

		//fill the new node with free pointers
		for i, ptr := range freed[:size] {
			flnSetPtr(new, i, ptr)
		}
		freed = freed[size:]
		if len(reuse) > 0 {
			//reuse a pointer from the list
			fl.head, reuse = reuse[0], reuse[1:]
			//overwrite an existing node page
			fl.use(fl.head, new)
		} else {
			//or append a page to house the new node
			fl.head = fl.new(new)
		}
	}
	assert(len(reuse) == 0)
}

// Callback for FreeList.
// Allocate a brand-new page by appending to the database.
func (db *KV) pageAppend(node btree.BNode) uint64 {
	assert(len(node.Data()) <= btree.BTREE_PAGE_SIZE)

	ptr := db.page.flushed + uint64(db.page.nappend)
	db.page.nappend++

	db.page.updates[ptr] = node.Data()

	return ptr
}

// Callback for FreeList.
// Reuse an existing page.
func (db *KV) pageUse(ptr uint64, node btree.BNode) {
	db.page.updates[ptr] = node.Data()
}
