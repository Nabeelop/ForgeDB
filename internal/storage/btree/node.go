package btree

import "encoding/binary"

// BNode is a single page in the B-Tree, stored as a flat byte slice
// that can be dumped directly to disk.
type BNode struct {
	data []byte
}

// Node type constants.
const (
	BNODE_NODE = 1 // internal node
	BNODE_LEAF = 2 // leaf node
)

// Layout constants.
const (
	HEADER             = 4
	BTREE_PAGE_SIZE    = 4096
	BTREE_MAX_KEY_SIZE = 1000
	BTREE_MAX_VAL_SIZE = 3000
)

func Assert(cond bool) {
	if !cond {
		panic("assertion failed")
	}
}

// NewBNode wraps a raw page byte slice into a BNode.
// Used by the storage layer to convert mmap'd memory into a BNode.
func NewBNode(data []byte) BNode {
	return BNode{data: data}
}

// Data returns the underlying raw byte slice of the BNode.
func (node BNode) Data() []byte {
	return node.data
}

func init() {
	node1max := HEADER + 8 + 2 + 4 + BTREE_MAX_KEY_SIZE + BTREE_MAX_VAL_SIZE
	Assert(node1max <= BTREE_PAGE_SIZE)
}

// ── Header ────────────────────────────────────────────────────────────────────

func (node BNode) btype() uint16 {
	return binary.LittleEndian.Uint16(node.data)
}

func (node BNode) nkeys() uint16 {
	return binary.LittleEndian.Uint16(node.data[2:4])
}

func (node BNode) setHeader(btype uint16, nkeys uint16) {
	binary.LittleEndian.PutUint16(node.data[0:2], btype)
	binary.LittleEndian.PutUint16(node.data[2:4], nkeys)
}

// ── Pointers ──────────────────────────────────────────────────────────────────

func (node BNode) getPtr(idx uint16) uint64 {
	Assert(idx < node.nkeys())
	pos := HEADER + 8*idx
	return binary.LittleEndian.Uint64(node.data[pos:])
}

func (node BNode) setPtr(idx uint16, val uint64) {
	Assert(idx < node.nkeys())
	pos := HEADER + 8*idx
	binary.LittleEndian.PutUint64(node.data[pos:], val)
}

// ── Offset list ───────────────────────────────────────────────────────────────

func offsetPos(node BNode, idx uint16) uint16 {
	Assert(1 <= idx && idx <= node.nkeys())
	return HEADER + 8*node.nkeys() + 2*(idx-1)
}

func (node BNode) getOffset(idx uint16) uint16 {
	if idx == 0 {
		return 0
	}
	return binary.LittleEndian.Uint16(node.data[offsetPos(node, idx):])
}

func (node BNode) setOffset(idx uint16, offset uint16) {
	binary.LittleEndian.PutUint16(node.data[offsetPos(node, idx):], offset)
}

// ── Key-value accessors ───────────────────────────────────────────────────────

// kvPos returns the byte offset of the KV pair at idx within the data slice.
func (node BNode) kvPos(idx uint16) uint16 {
	Assert(idx <= node.nkeys())
	return HEADER + 8*node.nkeys() + 2*node.nkeys() + node.getOffset(idx)
}

func (node BNode) getKey(idx uint16) []byte {
	Assert(idx < node.nkeys())
	pos := node.kvPos(idx)
	klen := binary.LittleEndian.Uint16(node.data[pos:])
	return node.data[pos+4:][:klen]
}

func (node BNode) getVal(idx uint16) []byte {
	Assert(idx < node.nkeys())
	pos := node.kvPos(idx)
	klen := binary.LittleEndian.Uint16(node.data[pos+0:])
	vlen := binary.LittleEndian.Uint16(node.data[pos+2:])
	return node.data[pos+4+klen:][:vlen]
}

// nbytes returns the total bytes used by this node.
func (node BNode) nbytes() uint16 {
	return node.kvPos(node.nkeys())
}
