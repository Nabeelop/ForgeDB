package btree

import "bytes"

// BTree is a copy-on-write B-Tree. Page management is decoupled via callbacks.
type BTree struct {
	root uint64

	// Callbacks for page management.
	get func(uint64) BNode // dereference a page pointer → BNode
	new func(BNode) uint64 // allocate a new page, return its page ID
	del func(uint64)       // deallocate a page
}

// GetRoot returns the root page ID of the B-Tree.
func (tree *BTree) GetRoot() uint64 {
	return tree.root
}

// SetRoot sets the root page ID of the B-Tree.
func (tree *BTree) SetRoot(root uint64) {
	tree.root = root
}

// SetCallbacks configures the page management callbacks for the B-Tree.
func (tree *BTree) SetCallbacks(
	get func(uint64) BNode,
	new func(BNode) uint64,
	del func(uint64),
) {
	tree.get = get
	tree.new = new
	tree.del = del
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
func (tree *BTree) InsertEx(req *InsertReq) {
	Assert(len(req.Key) != 0)
	Assert(len(req.Key) <= BTREE_MAX_KEY_SIZE)
	Assert(len(req.Val) <= BTREE_MAX_VAL_SIZE)

	// Bootstrap an empty tree.
	if tree.root == 0 {

		if req.Mode == MODE_UPDATE_ONLY {
			req.Added = false
			return
		}

		root := BNode{
			data: make([]byte, BTREE_PAGE_SIZE),
		}

		root.setHeader(BNODE_LEAF, 2)

		// Sentinel key.
		nodeAppendKV(
			root,
			0,
			0,
			nil,
			nil,
		)

		// First real key.
		nodeAppendKV(
			root,
			1,
			0,
			req.Key,
			req.Val,
		)

		req.Added = true

		tree.root = tree.new(root)
		return
	}

	node := tree.get(tree.root)
	tree.del(tree.root)

	// Modified insertion path.
	node = treeInsert(
		tree,
		node,
		req,
	)

	nsplit, splitted := nodeSplit3(node)

	if nsplit > 1 {

		root := BNode{
			data: make([]byte, BTREE_PAGE_SIZE),
		}

		root.setHeader(
			BNODE_NODE,
			nsplit,
		)

		for i, knode := range splitted[:nsplit] {
			nodeAppendKV(
				root,
				uint16(i),
				tree.new(knode),
				knode.getKey(0),
				nil,
			)
		}

		tree.root = tree.new(root)

	} else {

		tree.root = tree.new(
			splitted[0],
		)
	}
}

// Delete removes a key from the tree. Returns false if the key was not found.
func (tree *BTree) DeleteEx(req *DeleteReq) bool {
	Assert(len(req.Key) != 0)
	Assert(len(req.Key) <= BTREE_MAX_KEY_SIZE)

	if tree.root == 0 {
		return false
	}

	updated := treeDelete(tree, tree.get(tree.root), req.Key)

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
