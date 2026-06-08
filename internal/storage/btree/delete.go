package btree

import (
	"bytes"
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


// Part of treeDelete().
func nodeDelete(
	tree *BTree,
	node BNode,
	idx uint16,
	key []byte,
) BNode {

	// Recurse into the child.
	kptr := node.getPtr(idx)

	updated := treeDelete(
		tree,
		tree.get(kptr),
		key,
	)

	// Key not found.
	if len(updated.data) == 0 {
		return BNode{}
	}

	// Old child is no longer needed.
	tree.del(kptr)

	new := BNode{
		data: make([]byte, BTREE_PAGE_SIZE),
	}

	// Check whether the updated child should be merged.
	mergeDir, sibling := shouldMerge(
		tree,
		node,
		idx,
		updated,
	)

	switch {

	case mergeDir < 0:
		// Merge with left sibling.

		merged := BNode{
			data: make([]byte, BTREE_PAGE_SIZE),
		}

		nodeMerge(
			merged,
			sibling,
			updated,
		)

		// Delete left sibling.
		tree.del(node.getPtr(idx - 1))

		// Replace:
		//    [left sibling][updated]
		// with:
		//    [merged]
		nodeReplace2Kid(
			new,
			node,
			idx-1,
			tree.new(merged),
			merged.getKey(0),
		)

	case mergeDir > 0:
		// Merge with right sibling.

		merged := BNode{
			data: make([]byte, BTREE_PAGE_SIZE),
		}

		nodeMerge(
			merged,
			updated,
			sibling,
		)

		// Delete right sibling.
		tree.del(node.getPtr(idx + 1))

		// Replace:
		//    [updated][right sibling]
		// with:
		//    [merged]
		nodeReplace2Kid(
			new,
			node,
			idx,
			tree.new(merged),
			merged.getKey(0),
		)

	default:
		// No merge required.
		assert(updated.nkeys() > 0)

		nodeReplaceKidN(
			tree,
			new,
			node,
			idx,
			updated,
		)
	}

	return new
}

//merge 2nodes into 1
func nodeMerge(new BNode, left BNode,right BNode){
new.setHeader(left.btype(), left.nkeys()+right.nkeys())
nodeAppendRange(new,left,0,0, left.nkeys())
nodeAppendRange(new,right,left.nkeys(), 0,right.nkeys())
}


// Replace 2 child links with 1 child link.
func nodeReplace2Kid(
	new BNode,
	old BNode,
	idx uint16,
	ptr uint64,
	key []byte,
) {
	assert(idx+1 < old.nkeys())

	// old loses one child entry
	new.setHeader(
		BNODE_NODE,
		old.nkeys()-1,
	)

	// Copy entries before idx.
	nodeAppendRange(
		new,
		old,
		0,
		0,
		idx,
	)

	// Insert merged child.
	nodeAppendKV(
		new,
		idx,
		ptr,
		key,
		nil,
	)

	// Copy entries after idx+1.
	nodeAppendRange(
		new,
		old,
		idx+1,          // dst
		idx+2,          // src
		old.nkeys()-(idx+2),
	)
}


// Should the updated child be merged with a sibling?
func shouldMerge(
	tree *BTree,
	node BNode,
	idx uint16,
	updated BNode,
) (int, BNode) {

	// Not small enough to consider merging.
	if updated.nbytes() > BTREE_PAGE_SIZE/4 {
		return 0, BNode{}
	}

	// Try the left sibling.
	if idx > 0 {
		sibling := tree.get(node.getPtr(idx - 1))

		mergedSize :=
			sibling.nbytes() +
				updated.nbytes() -
				HEADER

		if mergedSize <= BTREE_PAGE_SIZE {
			return -1, sibling
		}
	}

	// Try the right sibling.
	if idx+1 < node.nkeys() {
		sibling := tree.get(node.getPtr(idx + 1))

		mergedSize :=
			sibling.nbytes() +
				updated.nbytes() -
				HEADER

		if mergedSize <= BTREE_PAGE_SIZE {
			return +1, sibling
		}
	}

	return 0, BNode{}
}