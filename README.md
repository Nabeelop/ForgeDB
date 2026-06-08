# ForgeDB

ForgeDB is a light-weight, persistent key-value database built from scratch in Go (Golang). It utilizes a block-based storage engine (Pager) and a B-Tree index structure.

## Project Structure

The codebase is organized following standard Go project layouts:

```text
ForgeDB/
├── go.mod                     # Go module definitions
├── cmd/
│   └── forgedb/
│       └── main.go            # CLI entry point; handles signals and configurations
├── internal/
│   ├── storage/
│   │   ├── btree.go           # B-Tree index leaf/internal nodes layout and serialization
│   │   ├── pager.go           # File pager reading/writing 4KB pages to disk
│   │   └── db.go              # Database coordinator exposing thread-safe Get/Put APIs
│   └── server/
│       └── server.go          # TCP server accepting socket queries (GET/PUT)
└── README.md                  # Documentation
```

## How It Works

1. **Pager (`internal/storage/pager.go`)**: Partitions database files into fixed-size `4096-byte` blocks (pages). This aligns memory boundaries with typical disk sector sizes for performance.
2. **B-Tree (`internal/storage/btree.go`)**: Manages keys indexing. Keys are stored sorted within node pages to facilitate fast lookups ($O(\log N)$).
3. **Database (`internal/storage/db.go`)**: The orchestrator. It ensures thread-safety using read-write locks and initializes page 0 as an empty B-Tree root leaf node when booting on a new file.
4. **Server (`internal/server/server.go`)**: Listens for TCP connections (defaulting to `:9090`) and parses incoming requests.

---

## Running the Server

If you have Go installed on your path, you can run the server using:

```bash
go run cmd/forgedb/main.go
```

By default, it will create a database file called `data.db` and listen on port `9090`. You can configure these with flags:

```bash
go run cmd/forgedb/main.go -db mydatabase.db -addr :8080
```

---

## Interacting with the Database

Because the server uses a simple line-based text protocol, you can interact with it using `netcat` or `telnet`:

```bash
nc localhost 9090
```

### Commands

Once connected, you can run:

* **`PUT <key> <value>`**: Sets a key-value pair.
* **`GET <key>`**: Fetches the value for a key.
* **`EXIT`** (or **`QUIT`**): Closes the current connection.

#### Example Session:
```text
$ nc localhost 9090
Welcome to ForgeDB Server!
PUT greeting Hello_World!
OK
GET greeting
VALUE: Hello_World!
EXIT
Goodbye!
```

---

## Next Steps for Implementation

1. **Implement Node Serialization (`internal/storage/btree.go`)**: Implement binary marshaling for keys and values into the 4KB pages in `BTreeNode.Serialize` and `DeserializeBTreeNode`.
2. **Implement Leaf Node Key Insertion (`internal/storage/db.go`)**: Implement key placement and binary search within BTreeNode arrays.
3. **Implement Splitting (`internal/storage/db.go`)**: When a leaf node keys count exceeds page storage capacity during a `Put` operation, split it into two nodes, create a parent internal node (or insert into the existing parent), and update page pointers.
