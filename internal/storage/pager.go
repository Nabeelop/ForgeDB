package storage

import (
	"errors"
	"fmt"
	"os"
)

const BTREE_PAGE_SIZE = 4096

//callbackfor BTree,dereference apointer.
func (db *KV)pageGet(ptr uint64)BNode {
start:=uint64(0)
for _,chunk:=range db.mmap.chunks {
end :=start+ uint64(len(chunk))/BTREE_PAGE_SIZE
ifptr <end {
offset :=BTREE_PAGE_SIZE * (ptr-start)
return BNode{chunk[offset : offset+BTREE_PAGE_SIZE]}
}
start= end
}
panic("bad ptr")
}