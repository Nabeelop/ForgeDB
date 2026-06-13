package storage

import (
	"bytes"
	"os"
	"testing"

	"forgedb/internal/storage/btree"
)

func TestKVBasic(t *testing.T) {
	// Create temporary db file
	dbFile := "test_temp.db"
	_ = os.Remove(dbFile)
	defer os.Remove(dbFile)

	db, err := Open(dbFile)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// 1. Insert some items
	k1 := []byte("key1")
	v1 := []byte("value1")
	if err := db.Set(&btree.InsertReq{Key: k1, Val: v1, Mode: btree.MODE_UPSERT}); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, ok := db.Get(k1)
	if !ok || !bytes.Equal(val, v1) {
		t.Fatalf("Get key1 failed: got %s, ok %t", val, ok)
	}

	// 2. Insert more to allocate several pages
	for i := 0; i < 100; i++ {
		k := []byte(string(rune(i)) + "_key")
		v := []byte(string(rune(i)) + "_val")
		if err := db.Set(&btree.InsertReq{Key: k, Val: v, Mode: btree.MODE_UPSERT}); err != nil {
			t.Fatalf("Set key at %d failed: %v", i, err)
		}
	}

	// 3. Delete keys to populate the FreeList
	for i := 0; i < 50; i++ {
		k := []byte(string(rune(i)) + "_key")
		ok, err := db.Del(&btree.DeleteReq{Key: k})
		if err != nil {
			t.Fatalf("Del key at %d failed: %v", i, err)
		}
		if !ok {
			t.Fatalf("Del key at %d not found", i)
		}
	}

	// 4. Set new keys (this should trigger page reuse)
	for i := 0; i < 50; i++ {
		k := []byte(string(rune(i+100)) + "_key_new")
		v := []byte(string(rune(i+100)) + "_val_new")
		if err := db.Set(&btree.InsertReq{Key: k, Val: v, Mode: btree.MODE_UPSERT}); err != nil {
			t.Fatalf("Set new key at %d failed: %v", i, err)
		}
	}

	// 5. Verify persistence across close/reopen
	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	db, err = Open(dbFile)
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}

	// Verify key1 is still there
	val, ok = db.Get(k1)
	if !ok || !bytes.Equal(val, v1) {
		t.Fatalf("Get key1 after reopen failed: got %s, ok %t", val, ok)
	}

	// Verify new keys are still there
	for i := 0; i < 50; i++ {
		k := []byte(string(rune(i+100)) + "_key_new")
		v := []byte(string(rune(i+100)) + "_val_new")
		val, ok := db.Get(k)
		if !ok || !bytes.Equal(val, v) {
			t.Fatalf("Get new key %s after reopen failed: got %s, ok %t", k, val, ok)
		}
	}
}
