package btree

import (
	"bytes"
)

type BIter struct {
	tree *BTree

	// path from root to leaf
	path []BNode

	// indexes inside each node
	pos []uint16
}

// Deref returns the current key/value pair.
// Precondition: iter.Valid() == true
func (iter *BIter) Deref() ([]byte, []byte) {

	Assert(iter.Valid())

	last := len(iter.path) - 1

	node := iter.path[last]
	idx := iter.pos[last]

	return node.getKey(idx), node.getVal(idx)
}

// Valid checks whether the iterator points
// to a valid key/value pair.
func (iter *BIter) Valid() bool {

	if len(iter.path) == 0 {
		return false
	}

	last := len(iter.path) - 1

	node := iter.path[last]
	idx := iter.pos[last]

	return idx < node.nkeys()
}

func iterPrev(iter *BIter, level int) {

	if iter.pos[level] > 0 {

		// Move within this node.
		iter.pos[level]--

	} else if level > 0 {

		// Move to a sibling via parent.
		iterPrev(iter, level-1)

	} else {

		// Reached the dummy key.
		return
	}

	if level+1 < len(iter.pos) {

		// Update child node.
		node := iter.path[level]

		kid := iter.tree.get(
			node.getPtr(iter.pos[level]),
		)

		iter.path[level+1] = kid

		iter.pos[level+1] =
			kid.nkeys() - 1
	}
}

func (iter *BIter) Prev() {
	iterPrev(
		iter,
		len(iter.path)-1,
	)
}

func iterNext(iter *BIter, level int) {

	Assert(level < len(iter.pos))

	if iter.pos[level] < iter.path[level].nkeys() {

		// Advance within current node
		iter.pos[level]++

	} else if level > 0 {

		// Move up to parent
		iterNext(iter, level-1)

	} else {

		// End of tree
		return
	}

	// Descend to the leftmost child if we moved down
	if level+1 < len(iter.path) {

		node := iter.path[level]
		child := iter.tree.get(node.getPtr(iter.pos[level]))
		iter.path[level+1] = child
		iter.pos[level+1] = 0
	}
}

func (iter *BIter) Next() {
	iterNext(iter, len(iter.path)-1)
}

// Find the closest position that is <= key.
func (tree *BTree) SeekLE(
	key []byte,
) *BIter {

	iter := &BIter{
		tree: tree,
	}

	for ptr := tree.root; ptr != 0; {

		node := tree.get(ptr)

		idx := nodeLookupLE(
			node,
			key,
		)

		iter.path = append(
			iter.path,
			node,
		)

		iter.pos = append(
			iter.pos,
			idx,
		)

		if node.btype() == BNODE_NODE {

			ptr = node.getPtr(idx)

		} else {

			ptr = 0
		}
	}

	return iter
}

const (
	CMP_GE = +3 // >=
	CMP_GT = +2 // >
	CMP_LT = -2 // <
	CMP_LE = -3 // <=
)

// Find the closest position to a key
// with respect to the cmp relation.
func (tree *BTree) Seek(
	key []byte,
	cmp int,
) *BIter {

	iter := tree.SeekLE(key)

	if cmp != CMP_LE && iter.Valid() {

		cur, _ := iter.Deref()

		if !CmpOK(cur, cmp, key) {

			// Off by one.
			if cmp > 0 {
				iter.Next()
			} else {
				iter.Prev()
			}
		}
	}

	return iter
}

// Check whether:
//
//	key cmp ref
func CmpOK(
	key []byte,
	cmp int,
	ref []byte,
) bool {

	r := bytes.Compare(key, ref)

	switch cmp {

	case CMP_GE:
		return r >= 0

	case CMP_GT:
		return r > 0

	case CMP_LT:
		return r < 0

	case CMP_LE:
		return r <= 0

	default:
		panic("unknown comparison operator")
	}
}
