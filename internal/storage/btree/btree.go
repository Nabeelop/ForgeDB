package btree

import "bytes"

// BTree is a copy-on-write B-Tree. Page management is decoupled via callbacks.
type BTree struct {
	root uint64

	// Callbacks for page management.
	get func(uint64) BNode  // dereference a page pointer → BNode
	new func(BNode) uint64  // allocate a new page, return its page ID
	del func(uint64)        // deallocate a page
}

// Get returns the value for key, or nil if the key does not exist.
func (tree *BTree) Get(key []byte) []byte {
	if tree.root == 0 {
		return nil
	}

	node := tree.get(tree.root)

	for {
		idx := nodeLookupLE(node, key)

		switch node.btype() {
		case BNODE_LEAF:
			if idx < node.nkeys() && bytes.Equal(node.getKey(idx), key) {
				return node.getVal(idx)
			}
			return nil

		case BNODE_NODE:
			node = tree.get(node.getPtr(idx))

		default:
			panic("bad node type")
		}
	}
}

// Insert inserts or updates a key-value pair in the tree.
func (tree *BTree) Insert(key []byte, val []byte) {
	assert(len(key) != 0)
	assert(len(key) <= BTREE_MAX_KEY_SIZE)
	assert(len(val) <= BTREE_MAX_VAL_SIZE)

	// Bootstrap: create an empty leaf root with a sentinel key.
	if tree.root == 0 {
		root := BNode{data: make([]byte, BTREE_PAGE_SIZE)}
		root.setHeader(BNODE_LEAF, 2)
		nodeAppendKV(root, 0, 0, nil, nil) // dummy sentinel
		nodeAppendKV(root, 1, 0, key, val) // first real key
		tree.root = tree.new(root)
		return
	}

	node := tree.get(tree.root)
	tree.del(tree.root)

	node = treeInsert(tree, node, key, val)

	nsplit, splitted := nodeSplit3(node)

	if nsplit > 1 {
		// Root was split — create a new internal root.
		root := BNode{data: make([]byte, BTREE_PAGE_SIZE)}
		root.setHeader(BNODE_NODE, nsplit)
		for i, knode := range splitted[:nsplit] {
			nodeAppendKV(root, uint16(i), tree.new(knode), knode.getKey(0), nil)
		}
		tree.root = tree.new(root)
	} else {
		tree.root = tree.new(splitted[0])
	}
}

// Delete removes a key from the tree. Returns false if the key was not found.
func (tree *BTree) Delete(key []byte) bool {
	assert(len(key) != 0)
	assert(len(key) <= BTREE_MAX_KEY_SIZE)

	if tree.root == 0 {
		return false
	}

	updated := treeDelete(tree, tree.get(tree.root), key)

	if len(updated.data) == 0 {
		return false // key not found
	}

	tree.del(tree.root)

	// Shrink tree height: if root is an internal node with a single child,
	// promote that child as the new root.
	if updated.btype() == BNODE_NODE && updated.nkeys() == 1 {
		tree.root = updated.getPtr(0)
	} else {
		tree.root = tree.new(updated)
	}

	return true
}