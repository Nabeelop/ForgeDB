package btree

import (
	"bytes"
	"encoding/binary"
)

type BNode struct{
	data []byte         //can be dumped to the disk,storing in disk format 
}

const(
	BNODE_NODE=1 //header for internal node
	BNODE_LEAF=2 //header for leaf node
)

func assert(cond bool) {
	if !cond {
		panic("assertion failed")
	}
}

type BTree struct{
	root uint64

  //callback functions
	get func(uint64) BNode  //dereference a pointer
	new func(BNode)  uint64 //allocate new page
	del func(uint64)        //deallocate a page
}

const HEADER=4

const BTREE_PAGE_SIZE=4096
const BTREE_MAX_KEY_SIZE=1000
const BTREE_MAX_VAL_SIZE=3000

func init(){
	node1max:=HEADER+8+2+4+BTREE_MAX_KEY_SIZE+BTREE_MAX_VAL_SIZE
	assert(node1max<= BTREE_PAGE_SIZE)
}

//header
func (node BNode)btype()uint16 {
return binary.LittleEndian.Uint16(node.data)
}
func (node BNode)nkeys()uint16 {
return binary.LittleEndian.Uint16(node.data[2:4])
}
func (node BNode)setHeader(btype uint16,nkeys uint16){
binary.LittleEndian.PutUint16(node.data[0:2],btype)
binary.LittleEndian.PutUint16(node.data[2:4],nkeys)
}

//pointers
func (node BNode)getPtr(idx uint16)uint64 {
assert(idx <node.nkeys())
pos :=HEADER +8*idx
return binary.LittleEndian.Uint64(node.data[pos:])
}
func (node BNode)setPtr(idx uint16,val uint64){
assert(idx <node.nkeys())
pos :=HEADER +8*idx
binary.LittleEndian.PutUint64(node.data[pos:], val)
}

// offset list

func offsetPos(node BNode, idx uint16) uint16 {
	assert(1 <= idx && idx <= node.nkeys())

	return HEADER + 8*node.nkeys() + 2*(idx-1)
}

func (node BNode) getOffset(idx uint16) uint16 {
	if idx == 0 {
		return 0
	}

	return binary.LittleEndian.Uint16(
		node.data[offsetPos(node, idx):],
	)
}

func (node BNode) setOffset(idx uint16, offset uint16) {
	binary.LittleEndian.PutUint16(
		node.data[offsetPos(node, idx):],
		offset,
	)
}

//key-values
func (node BNode)kvPos(idx uint16) uint16 {
assert(idx <=node.nkeys())
return HEADER +8*node.nkeys()+ 2*node.nkeys()+ node.getOffset(idx)
}


func (node BNode)getKey(idx uint16)[]byte {
assert(idx <node.nkeys())
pos :=node.kvPos(idx)
klen :=binary.LittleEndian.Uint16(node.data[pos:])
return node.data[pos+4:][:klen]
}


func (node BNode)getVal(idx uint16)[]byte {
assert(idx <node.nkeys())
pos :=node.kvPos(idx)
klen :=binary.LittleEndian.Uint16(node.data[pos+0:])
vlen :=binary.LittleEndian.Uint16(node.data[pos+2:])
return node.data[pos+4+klen:][:vlen]
}


func (node BNode)nbytes()uint16 {
return node.kvPos(node.nkeys())
}


// Get returns the value for a key, or nil if the key does not exist.
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
			return nil // key not found

		case BNODE_NODE:
			node = tree.get(node.getPtr(idx))

		default:
			panic("bad node type")
		}
	}
}

func (tree *BTree) Insert(
	key []byte,
	val []byte,
) {
	assert(len(key) != 0)
	assert(len(key) <= BTREE_MAX_KEY_SIZE)
	assert(len(val) <= BTREE_MAX_VAL_SIZE)

	// Empty tree.
	if tree.root == 0 {

		root := BNode{
			data: make([]byte, BTREE_PAGE_SIZE),
		}

		root.setHeader(
			BNODE_LEAF,
			2,
		)

		// Dummy key.
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
			key,
			val,
		)

		tree.root = tree.new(root)
		return
	}

	node := tree.get(tree.root)

	tree.del(tree.root)

	node = treeInsert(
		tree,
		node,
		key,
		val,
	)

	nsplit, splitted := nodeSplit3(node)

	if nsplit > 1 {

		// Create a new root.
		root := BNode{
			data: make([]byte, BTREE_PAGE_SIZE),
		}

		root.setHeader(
			BNODE_NODE,
			nsplit,
		)

		for i, knode := range splitted[:nsplit] {

			ptr := tree.new(knode)
			key := knode.getKey(0)

			nodeAppendKV(
				root,
				uint16(i),
				ptr,
				key,
				nil,
			)
		}

		tree.root = tree.new(root)

	} else {

		tree.root = tree.new(splitted[0])
	}
}


func (tree *BTree) Delete(key []byte) bool {
	assert(len(key) != 0)
	assert(len(key) <= BTREE_MAX_KEY_SIZE)

	if tree.root == 0 {
		return false
	}

	updated := treeDelete(
		tree,
		tree.get(tree.root),
		key,
	)

	if len(updated.data) == 0 {
		return false // key not found
	}

	tree.del(tree.root)

	// Remove one level if the root has only one child.
	if updated.btype() == BNODE_NODE &&
		updated.nkeys() == 1 {

		tree.root = updated.getPtr(0)

	} else {
		tree.root = tree.new(updated)
	}

	return true
}