package btree

import (
	"bytes"
	"encoding/binary"
	
)

// remove a key from a leaf node
func leafDelete(new BNode, old BNode, idx uint16) {
new.setHeader(BNODE_LEAF, old.nkeys()-1)
nodeAppendRange(new, old, 0, 0, idx)
nodeAppendRange(new, old, idx, idx+1, old.nkeys()-(idx+1))
}

// Delete a key from the tree.
func treeDelete(
	tree *BTree,
	node BNode,
	key []byte,
) BNode {

	// Find the largest key <= target.
	idx := nodeLookupLE(node, key)

	switch node.btype() {

	case BNODE_LEAF:
		// Key not found.
		if !bytes.Equal(key, node.getKey(idx)) {
			return BNode{}
		}

		// Delete the key from the leaf.
		new := BNode{
			data: make([]byte, BTREE_PAGE_SIZE),
		}

		leafDelete(
			new,
			node,
			idx,
		)

		return new

	case BNODE_NODE:
		// Recurse into child.
		return nodeDelete(
			tree,
			node,
			idx,
			key,
		)

	default:
		panic("bad node!")
	}
}