package table

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"forgedb/internal/storage/btree"
)

func TestTableBasic(t *testing.T) {
	dbFile := "test_table_temp.db"
	_ = os.Remove(dbFile)
	defer os.Remove(dbFile)

	db := &DB{Path: dbFile}
	if err := db.Open(); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// 1. Define schema
	tdef := &TableDef{
		Name:  "users",
		Types: []uint32{TYPE_INT64, TYPE_BYTES, TYPE_INT64},
		Cols:  []string{"id", "name", "age"},
		PKeys: 1,
		Indexes: [][]string{
			{"name"},
		},
	}

	// 2. Create Table
	if err := db.TableNew(tdef); err != nil {
		t.Fatalf("TableNew failed: %v", err)
	}

	// 3. Insert rows
	rec1 := (&Record{}).AddInt64("id", 1).AddStr("name", []byte("Alice")).AddInt64("age", 25)
	if _, err := db.Insert("users", *rec1); err != nil {
		t.Fatalf("Insert rec1 failed: %v", err)
	}

	rec2 := (&Record{}).AddInt64("id", 2).AddStr("name", []byte("Bob")).AddInt64("age", 30)
	if _, err := db.Insert("users", *rec2); err != nil {
		t.Fatalf("Insert rec2 failed: %v", err)
	}

	rec3 := (&Record{}).AddInt64("id", 3).AddStr("name", []byte("Charlie")).AddInt64("age", 35)
	if _, err := db.Insert("users", *rec3); err != nil {
		t.Fatalf("Insert rec3 failed: %v", err)
	}

	// 4. Point Query (Get)
	getRec := (&Record{}).AddInt64("id", 2)
	ok, err := db.Get("users", getRec)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !ok {
		t.Fatalf("Record id=2 not found")
	}

	nameVal := getRec.Get("name")
	if nameVal == nil || !bytes.Equal(nameVal.Str, []byte("Bob")) {
		t.Fatalf("Expected Bob, got %v", nameVal)
	}

	ageVal := getRec.Get("age")
	if ageVal == nil || ageVal.I64 != 30 {
		t.Fatalf("Expected age 30, got %v", ageVal)
	}

	// 5. Primary Key Range Scan (id >= 1 and id <= 2)
	scannerPK := &Scanner{
		Cmp1: btree.CMP_GE,
		Cmp2: btree.CMP_LE,
		Key1: *(&Record{}).AddInt64("id", 1),
		Key2: *(&Record{}).AddInt64("id", 2),
	}
	if err := db.Scan("users", scannerPK); err != nil {
		t.Fatalf("PK Scan failed: %v", err)
	}

	count := 0
	expectedNames := [][]byte{[]byte("Alice"), []byte("Bob")}
	for scannerPK.Valid() {
		var r Record
		scannerPK.Deref(&r)
		name := r.Get("name").Str
		if !bytes.Equal(name, expectedNames[count]) {
			t.Fatalf("PK Scan index %d: expected %s, got %s", count, expectedNames[count], name)
		}
		count++
		scannerPK.Next()
	}
	if count != 2 {
		t.Fatalf("Expected 2 records from PK Scan, got %d", count)
	}

	// DEBUG LOG
	{
		t.Logf("--- B-TREE KEYS ---")
		iter := db.kv.Tree.Seek([]byte{}, btree.CMP_GE)
		for iter.Valid() {
			k, v := iter.Deref()
			pref := uint32(0)
			if len(k) >= 4 {
				pref = binary.BigEndian.Uint32(k[:4])
			}
			t.Logf("  KEY: %v (prefix: %d), VAL: %v", k, pref, v)
			iter.Next()
		}
	}

	// 6. Secondary Index Range Scan (name >= "Bob" and name <= "Charlie")
	scannerSec := &Scanner{
		Cmp1: btree.CMP_GE,
		Cmp2: btree.CMP_LE,
		Key1: *(&Record{}).AddStr("name", []byte("Bob")),
		Key2: *(&Record{}).AddStr("name", []byte("Charlie")),
	}
	if err := db.Scan("users", scannerSec); err != nil {
		t.Fatalf("Secondary Index Scan failed: %v", err)
	}

	count = 0
	expectedSecNames := [][]byte{[]byte("Bob"), []byte("Charlie")}
	for scannerSec.Valid() {
		var r Record
		scannerSec.Deref(&r)
		name := r.Get("name").Str
		if !bytes.Equal(name, expectedSecNames[count]) {
			t.Fatalf("Secondary Scan index %d: expected %s, got %s", count, expectedSecNames[count], name)
		}
		count++
		scannerSec.Next()
	}
	if count != 2 {
		t.Fatalf("Expected 2 records from Secondary Scan, got %d", count)
	}

	// 7. Update Record
	updateRec := (&Record{}).AddInt64("id", 2).AddStr("name", []byte("Bobby")).AddInt64("age", 31)
	if _, err := db.Update("users", *updateRec); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify update in point query
	getRec2 := (&Record{}).AddInt64("id", 2)
	ok, err = db.Get("users", getRec2)
	if err != nil || !ok || !bytes.Equal(getRec2.Get("name").Str, []byte("Bobby")) {
		t.Fatalf("Verify update failed: ok %t, err %v", ok, err)
	}

	// 8. Delete Record
	delRec := (&Record{}).AddInt64("id", 2)
	deleted, err := db.Delete("users", *delRec)
	if err != nil || !deleted {
		t.Fatalf("Delete failed: deleted %t, err %v", deleted, err)
	}

	// Verify deletion
	getRec3 := (&Record{}).AddInt64("id", 2)
	ok, err = db.Get("users", getRec3)
	if err != nil || ok {
		t.Fatalf("Expected record to be deleted, ok=%t, err=%v", ok, err)
	}

	// 9. Open-ended range scans
	// Forward scan with no end key (name >= "Alice")
	scannerOpenEnd := &Scanner{
		Cmp1: btree.CMP_GE,
		Cmp2: 0,
		Key1: *(&Record{}).AddStr("name", []byte("Alice")),
	}
	if err := db.Scan("users", scannerOpenEnd); err != nil {
		t.Fatalf("Open-ended forward scan failed: %v", err)
	}

	count = 0
	expectedOpenNames := [][]byte{[]byte("Alice"), []byte("Charlie")} // Bob (id=2) was deleted!
	for scannerOpenEnd.Valid() {
		var r Record
		scannerOpenEnd.Deref(&r)
		name := r.Get("name").Str
		if !bytes.Equal(name, expectedOpenNames[count]) {
			t.Fatalf("Open-ended forward scan index %d: expected %s, got %s", count, expectedOpenNames[count], name)
		}
		count++
		scannerOpenEnd.Next()
	}
	if count != 2 {
		t.Fatalf("Expected 2 records from open-ended forward scan, got %d", count)
	}
}
