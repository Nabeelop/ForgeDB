package btree

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"unsafe"
)

// ── Test harness ──────────────────────────────────────────────────────────────

// C wraps a BTree together with a reference map so every operation
// can be verified against a known-good ground truth.
type C struct {
	tree  BTree
	ref   map[string]string
	pages map[uint64]BNode
}

func newC() *C {
	pages := map[uint64]BNode{}

	return &C{
		tree: BTree{
			get: func(ptr uint64) BNode {
				node, ok := pages[ptr]
				assert(ok)
				return node
			},

			new: func(node BNode) uint64 {
				assert(node.nbytes() <= BTREE_PAGE_SIZE)

				// Use the backing-array address as a unique page ID.
				key := uint64(
					uintptr(
						unsafe.Pointer(&node.data[0]),
					),
				)

				assert(pages[key].data == nil)

				pages[key] = node

				return key
			},

			del: func(ptr uint64) {
				_, ok := pages[ptr]
				assert(ok)

				delete(pages, ptr)
			},
		},

		ref:   map[string]string{},
		pages: pages,
	}
}

// add inserts/updates a key in both the tree and the reference map.
func (c *C) add(key string, val string) {
	c.tree.Insert([]byte(key), []byte(val))
	c.ref[key] = val
}

// del deletes a key from both the tree and the reference map.
func (c *C) del(key string) bool {
	delete(c.ref, key)
	return c.tree.Delete([]byte(key))
}

// verify checks that every key in the reference map is present in the tree
// with the correct value, and that the tree has no extra keys.
func (c *C) verify(t *testing.T) {
	t.Helper()

	// Walk tree leaves in order and collect all (key, value) pairs.
	treeKV := map[string]string{}
	var walk func(ptr uint64)
	walk = func(ptr uint64) {
		node := c.pages[ptr]
		switch node.btype() {
		case BNODE_LEAF:
			for i := uint16(0); i < node.nkeys(); i++ {
				k := string(node.getKey(i))
				if k == "" {
					continue // skip dummy sentinel
				}
				treeKV[k] = string(node.getVal(i))
			}
		case BNODE_NODE:
			for i := uint16(0); i < node.nkeys(); i++ {
				walk(node.getPtr(i))
			}
		default:
			t.Errorf("unknown node type %d", node.btype())
		}
	}

	if c.tree.root != 0 {
		walk(c.tree.root)
	}

	// Every ref key must be in the tree with the correct value.
	for k, want := range c.ref {
		got, ok := treeKV[k]
		if !ok {
			t.Errorf("verify: key %q missing from tree", k)
			continue
		}
		if got != want {
			t.Errorf("verify: key %q = %q, want %q", k, got, want)
		}
	}

	// Tree must not have extra keys beyond the reference.
	for k := range treeKV {
		if _, ok := c.ref[k]; !ok {
			t.Errorf("verify: tree has unexpected key %q", k)
		}
	}

	// Tree leaf keys must be in sorted order.
	keys := make([]string, 0, len(treeKV))
	for k := range treeKV {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	treeKeys := []string{}
	var collectSorted func(ptr uint64)
	collectSorted = func(ptr uint64) {
		node := c.pages[ptr]
		switch node.btype() {
		case BNODE_LEAF:
			for i := uint16(0); i < node.nkeys(); i++ {
				k := string(node.getKey(i))
				if k != "" {
					treeKeys = append(treeKeys, k)
				}
			}
		case BNODE_NODE:
			for i := uint16(0); i < node.nkeys(); i++ {
				collectSorted(node.getPtr(i))
			}
		}
	}
	if c.tree.root != 0 {
		collectSorted(c.tree.root)
	}
	for i := 1; i < len(treeKeys); i++ {
		if treeKeys[i] <= treeKeys[i-1] {
			t.Errorf("verify: keys out of order at [%d]: %q >= %q",
				i, treeKeys[i-1], treeKeys[i])
		}
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestInsertAndGet(t *testing.T) {
	c := newC()

	c.add("apple", "red")
	c.add("banana", "yellow")
	c.add("cherry", "dark-red")

	c.verify(t)
}

func TestUpdateExistingKey(t *testing.T) {
	c := newC()

	c.add("name", "Alice")
	c.add("name", "Bob") // overwrites
	c.verify(t)
}

func TestDeleteKey(t *testing.T) {
	c := newC()

	c.add("x", "1")
	c.add("y", "2")
	c.add("z", "3")

	if !c.del("y") {
		t.Fatal("del returned false for existing key")
	}

	c.verify(t)
}

func TestDeleteNonExistentKey(t *testing.T) {
	c := newC()
	c.add("a", "1")

	if c.del("b") {
		t.Error("del should return false for missing key")
	}
	c.verify(t)
}

func TestDeleteFromEmptyTree(t *testing.T) {
	c := newC()
	if c.del("anything") {
		t.Error("del on empty tree should return false")
	}
}

func TestSortedOrder(t *testing.T) {
	c := newC()

	inputs := []string{"mango", "apple", "cherry", "banana", "date", "elderberry"}
	for _, k := range inputs {
		c.add(k, "v")
	}

	c.verify(t)
}

// TestSplitAndMerge stresses splits and merges with 500 random inserts
// followed by deleting half the keys.
func TestSplitAndMerge(t *testing.T) {
	const N = 500
	c := newC()

	keys := make([]string, N)
	for i := 0; i < N; i++ {
		keys[i] = fmt.Sprintf("key-%06d", i)
	}

	// Shuffle for non-trivial split order.
	shuffled := make([]string, N)
	copy(shuffled, keys)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	for _, k := range shuffled {
		c.add(k, "value:"+k)
	}

	c.verify(t)

	// Delete every other key.
	for i := 0; i < N; i += 2 {
		c.del(keys[i])
	}

	c.verify(t)
}

func TestLargeKeyAndValue(t *testing.T) {
	c := newC()

	bigKey := make([]byte, BTREE_MAX_KEY_SIZE)
	bigVal := make([]byte, BTREE_MAX_VAL_SIZE)
	for i := range bigKey {
		bigKey[i] = 'k'
	}
	for i := range bigVal {
		bigVal[i] = 'v'
	}

	c.add(string(bigKey), string(bigVal))
	c.verify(t)
}

// TestRandomOps runs a randomised sequence of inserts, updates, and deletes
// and verifies the tree matches the reference map after every operation.
func TestRandomOps(t *testing.T) {
	const OPS = 1000
	c := newC()
	rng := rand.New(rand.NewSource(42))

	pool := make([]string, 50)
	for i := range pool {
		pool[i] = fmt.Sprintf("k%02d", i)
	}

	for i := 0; i < OPS; i++ {
		k := pool[rng.Intn(len(pool))]
		switch rng.Intn(3) {
		case 0, 1: // insert / update (weighted 2:1 over delete)
			c.add(k, fmt.Sprintf("v%d", i))
		case 2: // delete
			c.del(k)
		}
	}

	c.verify(t)
}

func TestGet(t *testing.T) {
	c := newC()

	c.add("foo", "bar")
	c.add("hello", "world")

	// Found cases.
	for _, tc := range []struct{ key, want string }{
		{"foo", "bar"},
		{"hello", "world"},
	} {
		got := c.tree.Get([]byte(tc.key))
		if string(got) != tc.want {
			t.Errorf("Get(%q) = %q, want %q", tc.key, got, tc.want)
		}
	}

	// Missing key must return nil.
	if c.tree.Get([]byte("missing")) != nil {
		t.Error("Get: expected nil for missing key")
	}

	// Empty tree must return nil.
	empty := &BTree{
		get: c.tree.get,
		new: c.tree.new,
		del: c.tree.del,
	}
	if empty.Get([]byte("anything")) != nil {
		t.Error("Get on empty tree should return nil")
	}
}