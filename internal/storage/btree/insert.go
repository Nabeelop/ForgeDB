package btree

import (
	"bytes"
	"encoding/binary"
)

// Modes of updates.
const (
	MODE_UPSERT      = 0 // insert or replace
	MODE_UPDATE_ONLY = 1 // update existing keys
	MODE_INSERT_ONLY = 2 // only add new keys
)

type InsertReq struct {
	tree *BTree

	// Output
	Added   bool   // inserted new key
	Updated bool   // insert OR existing key changed
	Old     []byte // previous value before update

	// Input
	Key  []byte
	Val  []byte
	Mode int
}

func nodeLookupLE(node BNode, key []byte) uint16 {
	nkeys := node.nkeys()

	// key[0] is always <= target
	low := uint16(1)
	high := nkeys

	found := uint16(0)

	for low < high {
		mid := low + (high-low)/2

		cmp := bytes.Compare(node.getKey(mid), key)

		if cmp <= 0 {
			found = mid
			low = mid + 1
		} else {
			high = mid
		}
	}

	return found
}

// Add a new key-value pair to a leaf node.
func leafInsert(
	new BNode,
	old BNode,
	idx uint16,
	key []byte,
	val []byte,
) {
	// New node will have one extra key.
	new.setHeader(BNODE_LEAF, old.nkeys()+1)

	// Copy entries before the insertion position.
	nodeAppendRange(
		new,
		old,
		0,   // destination position
		0,   // source position
		idx, // number of entries
	)

	// Insert the new KV pair.
	nodeAppendKV(
		new,
		idx,
		0, // leaf nodes don't use child pointers
		key,
		val,
	)

	// Copy remaining entries after the insertion position.
	nodeAppendRange(
		new,
		old,
		idx+1,           // destination position
		idx,             // source position
		old.nkeys()-idx, // number of entries
	)
}

func leafUpdate(
	new BNode,
	old BNode,
	idx uint16,
	key []byte,
	val []byte,
) {
	// Same number of keys.
	new.setHeader(BNODE_LEAF, old.nkeys())

	// Copy entries before idx.
	nodeAppendRange(
		new,
		old,
		0,
		0,
		idx,
	)

	// Replace the KV at idx.
	nodeAppendKV(
		new,
		idx,
		0, // leaf nodes have no child pointers
		key,
		val,
	)

	// Copy entries after idx.
	nodeAppendRange(
		new,
		old,
		idx+1, // destination
		idx+1, // source
		old.nkeys()-idx-1,
	)
}

func nodeAppendRange(
	new BNode,
	old BNode,
	dstNew uint16,
	srcOld uint16,
	n uint16,
) {
	Assert(dstNew+n <= new.nkeys())
	Assert(srcOld+n <= old.nkeys())

	if n == 0 {
		return
	}

	// pointers
	for i := uint16(0); i < n; i++ {
		new.setPtr(dstNew+i, old.getPtr(srcOld+i))
	}

	dstBegin := new.getOffset(dstNew)
	srcBegin := old.getOffset(srcOld)

	// offsets
	for i := uint16(1); i <= n; i++ {
		offset := dstBegin +
			old.getOffset(srcOld+i) -
			srcBegin

		new.setOffset(dstNew+i, offset)
	}

	// KV bytes
	begin := old.kvPos(srcOld)
	end := old.kvPos(srcOld + n)

	copy(
		new.data[new.kvPos(dstNew):],
		old.data[begin:end],
	)
}

func nodeAppendKV(new BNode, idx uint16, ptr uint64, key []byte, val []byte) {
	//ptrs
	new.setPtr(idx, ptr)
	//KVs
	pos := new.kvPos(idx)
	binary.LittleEndian.PutUint16(new.data[pos+0:], uint16(len(key)))
	binary.LittleEndian.PutUint16(new.data[pos+2:], uint16(len(val)))
	copy(new.data[pos+4:], key)
	copy(new.data[pos+4+uint16(len(key)):], val)
	//the offsetofthe next key
	new.setOffset(idx+1, new.getOffset(idx)+4+uint16((len(key)+len(val))))
}

// Insert a KV into a node.
// The resulting node may exceed one page and will be split later.
// The caller is responsible for deallocating the old node and
// allocating/splitting result nodes as needed.
func treeInsert(
	tree *BTree,
	node BNode,
	req *InsertReq,
) BNode {

	// Result node.
	// It is allowed to be larger than one page temporarily.
	new := BNode{
		data: make([]byte, 2*BTREE_PAGE_SIZE),
	}

	// Find the largest key <= target key.
	idx := nodeLookupLE(node, req.Key)

	switch node.btype() {

	case BNODE_LEAF:
		// Leaf node.
		// node.getKey(idx) <= key

		exists := bytes.Equal(
			req.Key,
			node.getKey(idx),
		)

		switch req.Mode {
		case MODE_UPSERT:
			// Key already exists: update value.
			if exists {
				req.Added = false
				req.Updated = true
				oldVal := node.getVal(idx)
				req.Old = append([]byte(nil), oldVal...)
				leafUpdate(
					new,
					node,
					idx,
					req.Key,
					req.Val,
				)
			} else {
				// Insert after idx.
				req.Added = true
				req.Updated = true
				req.Old = nil
				leafInsert(
					new,
					node,
					idx+1,
					req.Key,
					req.Val,
				)
			}
		case MODE_INSERT_ONLY:
			// Only insert if key does not already exist.
			if !exists {
				req.Added = true
				req.Updated = true
				req.Old = nil
				leafInsert(
					new,
					node,
					idx+1,
					req.Key,
					req.Val,
				)
			} else {
				// Key already exists, return old node unchanged.
				copy(new.data, node.data)
				req.Added = false
				req.Updated = false
			}

		case MODE_UPDATE_ONLY:
			// Only update if key already exists.
			if exists {
				req.Added = false
				req.Updated = true
				oldVal := node.getVal(idx)
				req.Old = append([]byte(nil), oldVal...)
				leafUpdate(
					new,
					node,
					idx,
					req.Key,
					req.Val,
				)
				req.Added = false // not actually added, just updated
			} else {
				// Key not found, return old node unchanged.
				copy(new.data, node.data)
				req.Added = false
				req.Updated = false
			}
		}

	case BNODE_NODE:
		// Internal node: recurse into child.
		nodeInsert(
			tree,
			new,
			node,
			idx,
			*&req,
		)

	default:
		panic("bad node!")
	}

	return new
}

// part of the treeInsert():KV insertion to an internal node
func nodeInsert(
	tree *BTree, new BNode, node BNode, idx uint16,
	req *InsertReq,
) {
	//get and deallocate the kid node
	kptr := node.getPtr(idx)
	knode := tree.get(kptr)
	tree.del(kptr)
	//recursiveinsertiontothe kid node
	knode = treeInsert(tree, knode, *&req)
	//split the result
	nsplit, splited := nodeSplit3(knode)
	//updatethe kid links
	nodeReplaceKidN(tree, new, node, idx, splited[:nsplit]...)
}

// Split a node into two nodes.
// The right node always fits in one page.
// The left node may still be oversized.
func nodeSplit2(left BNode, right BNode, old BNode) {
	Assert(old.nkeys() >= 2)

	// find split point
	nleft := old.nkeys() / 2

	// move split point right until right fits
	for old.nbytes()-old.kvPos(nleft) > BTREE_PAGE_SIZE {
		nleft++
	}

	Assert(nleft < old.nkeys())

	// left node
	left.setHeader(old.btype(), nleft)

	nodeAppendRange(
		left,
		old,
		0,     // dst
		0,     // src
		nleft, // count
	)

	// right node
	nright := old.nkeys() - nleft

	right.setHeader(old.btype(), nright)

	nodeAppendRange(
		right,
		old,
		0,      // dst
		nleft,  // src
		nright, // count
	)
}

// split a node if it's too big. the results are 1~3 nodes.
func nodeSplit3(old BNode) (uint16, [3]BNode) {
	if old.nbytes() <= BTREE_PAGE_SIZE {
		old.data = old.data[:BTREE_PAGE_SIZE]
		return 1, [3]BNode{old}
	}
	left := BNode{make([]byte, 2*BTREE_PAGE_SIZE)}
	//might be split later
	right := BNode{make([]byte, BTREE_PAGE_SIZE)}

	nodeSplit2(left, right, old)

	if left.nbytes() <= BTREE_PAGE_SIZE {
		left.data = left.data[:BTREE_PAGE_SIZE]

		return 2, [3]BNode{left, right}
	}
	//the left node is still too large
	leftleft := BNode{make([]byte, BTREE_PAGE_SIZE)}

	middle := BNode{make([]byte, BTREE_PAGE_SIZE)}

	nodeSplit2(leftleft, middle, left)

	Assert(leftleft.nbytes() <= BTREE_PAGE_SIZE)

	return 3, [3]BNode{leftleft, middle, right}
}

// replace a link with multiple links
func nodeReplaceKidN(
	tree *BTree, new BNode, old BNode, idx uint16,
	kids ...BNode,
) {
	inc := uint16(len(kids))
	new.setHeader(BNODE_NODE, old.nkeys()+inc-1)
	nodeAppendRange(new, old, 0, 0, idx)
	for i, node := range kids {
		nodeAppendKV(new, idx+uint16(i), tree.new(node), node.getKey(0), nil)
	}
	nodeAppendRange(new, old, idx+inc, idx+1, old.nkeys()-(idx+1))
}
