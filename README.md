# ForgeDB

A persistent key-value database engine built from scratch in Go. ForgeDB implements a **copy-on-write B-Tree** backed by a **page-based disk storage engine** — no external libraries, no frameworks, just raw Go and binary encoding.

---

## Project Structure

```text
ForgeDB/
├── go.mod
├── cmd/
│   └── forgedb/
│       └── main.go                  # Server entry point (TCP listener, CLI flags)
├── internal/
│   ├── storage/
│   │   ├── btree/
│   │   │   ├── btree.go             # BNode layout, accessors, constants
│   │   │   ├── insert.go            # Insertion logic (leaf + internal nodes)
│   │   │   └── delete.go            # Deletion logic (leaf + internal nodes)
│   │   ├── pager.go                 # File-backed page manager (4KB blocks)
│   │   └── db.go                    # DB coordinator (Get / Put API)
│   └── server/
│       └── server.go                # TCP server + text wire protocol
└── README.md
```

---

## How It Works

### 1. Pager (`internal/storage/pager.go`)

The pager divides the database file into fixed-size **4096-byte pages**. Each page maps to one B-Tree node on disk. The pager handles:

- `ReadPage(pageNumber)` — reads a page from disk
- `WritePage(pageNumber, data)` — writes a page to disk
- `Sync()` — flushes writes to disk (fsync)

### 2. B-Tree Node Layout (`internal/storage/btree/btree.go`)

Every node is stored as a flat `[]byte` of exactly `4096` bytes so it can be dumped directly to disk. The binary layout inside each page is:

```
| header (4B) | pointers (8B × nkeys) | offsets (2B × nkeys) | KV pairs... |
```

- **Header** — `[2B type][2B nkeys]`
  - Type `1` = internal node (`BNODE_NODE`)
  - Type `2` = leaf node (`BNODE_LEAF`)
- **Pointers** — child page numbers (only used in internal nodes)
- **Offsets** — relative byte positions of each KV pair in the KV area
- **KV pairs** — `[2B klen][2B vlen][key bytes][val bytes]`

Constants:
```go
BTREE_PAGE_SIZE    = 4096
BTREE_MAX_KEY_SIZE = 1000
BTREE_MAX_VAL_SIZE = 3000
HEADER             = 4
```

Key accessors implemented:
- `btype()` / `nkeys()` / `setHeader()`
- `getPtr(idx)` / `setPtr(idx, val)`
- `getOffset(idx)` / `setOffset(idx, offset)` / `offsetPos()`
- `kvPos(idx)` — byte position of KV pair in the data buffer
- `getKey(idx)` / `getVal(idx)`
- `nbytes()` — total bytes used by this node

### 3. B-Tree Structure (`internal/storage/btree/btree.go`)

```go
type BTree struct {
    root uint64

    // Callbacks for page management
    get func(uint64) BNode  // dereference a page pointer
    new func(BNode) uint64  // allocate a new page
    del func(uint64)        // deallocate a page
}
```

The tree is **decoupled from disk I/O** via callbacks. This makes it easy to swap in an in-memory backend for testing, or a file-backed pager for production.

### 4. Insertion (`internal/storage/btree/insert.go`)

Insertion is **copy-on-write** — the original node is never modified in place. A fresh buffer is always allocated and the modified data is written into it.

Key functions:

| Function | Description |
|----------|-------------|
| `nodeLookupLE(node, key)` | Binary search — finds the largest key index ≤ target |
| `nodeAppendKV(new, idx, ptr, key, val)` | Writes a single KV entry into a node at a given index |
| `nodeAppendRange(new, old, dst, src, n)` | Bulk-copies n KV entries from one node to another |
| `leafInsert(new, old, idx, key, val)` | Inserts a new KV into a leaf node |
| `leafUpdate(new, old, idx, key, val)` | Updates an existing KV in a leaf node |
| `treeInsert(tree, node, key, val)` | Recursive insert — dispatches to leaf or internal path |
| `nodeInsert(tree, new, node, idx, key, val)` | Internal node insert — recurses into child, then splits |
| `nodeSplit2(left, right, old)` | Splits an oversized node into two |
| `nodeSplit3(old)` | Splits into up to 3 nodes if needed |
| `nodeReplaceKidN(tree, new, old, idx, kids...)` | Replaces one child pointer with multiple after a split |

### 5. Deletion (`internal/storage/btree/delete.go`)

Deletion is also copy-on-write.

| Function | Description |
|----------|-------------|
| `leafDelete(new, old, idx)` | Removes a key from a leaf node |
| `treeDelete(tree, node, key)` | Recursive delete — dispatches to leaf or internal path |

### 6. TCP Server (`internal/server/server.go`)

Listens for client connections and exposes a simple line-based text protocol:

| Command | Description |
|---------|-------------|
| `GET <key>` | Fetch a value |
| `PUT <key> <value>` | Insert or update a key-value pair |
| `EXIT` / `QUIT` | Close the connection |

---

## Running

> Requires Go 1.21+

```bash
go run cmd/forgedb/main.go
```

Default settings:
- Database file: `data.db`
- TCP address: `:9090`

Custom settings:
```bash
go run cmd/forgedb/main.go -db mydb.db -addr :8080
```

---

## Connecting

```bash
nc localhost 9090
```

Example session:
```
Welcome to ForgeDB Server!
PUT name Nabeel
OK
GET name
VALUE: Nabeel
EXIT
Goodbye!
```

---

## Status

| Component | Status |
|-----------|--------|
| Page-based disk storage (Pager) | ✅ Done |
| B-Tree node binary layout | ✅ Done |
| B-Tree insertion (leaf + split) | ✅ Done |
| B-Tree deletion (leaf) | ✅ Done |
| TCP server + wire protocol | ✅ Done |
| Public `Insert` / `Delete` root API | 🔧 In progress |
| B-Tree lookup / `Get` | 🔧 In progress |
| Pager integration with B-Tree | 🔧 In progress |
