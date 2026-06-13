package table

import (
	"fmt"
	"forgedb/internal/storage/btree"
)

// table definition
type TableDef struct {
	// user defined
	Name string

	// schema
	Types []uint32 // column types
	Cols  []string // column names

	Indexes [][]string
	//auto-assigned B-tree key prefixes for different tables/indexes

	// primary key
	PKeys int // the first `PKeys` columns are the primary key

	// internal
	Prefix uint32 // auto-assigned B-tree key prefixes for different tables

	IndexPrefixes []uint32
}

// Internal table: metadata
var TDEF_META = &TableDef{
	Prefix: 1,
	Name:   "@meta",
	Types: []uint32{
		TYPE_BYTES,
		TYPE_BYTES,
	},
	Cols: []string{
		"key",
		"val",
	},
	PKeys: 1,
}

// Internal table: table schemas
var TDEF_TABLE = &TableDef{
	Prefix: 2,
	Name:   "@table",
	Types: []uint32{
		TYPE_BYTES,
		TYPE_BYTES,
	},
	Cols: []string{
		"name",
		"def",
	},
	PKeys: 1,
}

func tableDefCheck(tdef *TableDef) error {

	if tdef == nil {
		return fmt.Errorf("nil table definition")
	}

	if tdef.Name == "" {
		return fmt.Errorf("table name is empty")
	}

	if len(tdef.Cols) == 0 {
		return fmt.Errorf("table has no columns")
	}

	if len(tdef.Cols) != len(tdef.Types) {
		return fmt.Errorf(
			"column/type count mismatch",
		)
	}

	if tdef.PKeys <= 0 {
		return fmt.Errorf(
			"table must have a primary key",
		)
	}

	if tdef.PKeys > len(tdef.Cols) {
		return fmt.Errorf(
			"invalid number of primary keys",
		)
	}

	seen := make(map[string]bool)

	for i, col := range tdef.Cols {

		if col == "" {
			return fmt.Errorf(
				"empty column name",
			)
		}

		if seen[col] {
			return fmt.Errorf(
				"duplicate column: %s",
				col,
			)
		}

		seen[col] = true

		switch tdef.Types[i] {

		case TYPE_BYTES:
		case TYPE_INT64:

		default:
			return fmt.Errorf(
				"invalid type for column %s",
				col,
			)
		}
	}

	// Verify indexes
	for i, index := range tdef.Indexes {

		index, err := checkIndexKeys(
			tdef,
			index,
		)

		if err != nil {
			return err
		}

		tdef.Indexes[i] = index
	}

	return nil
}

// Validate an index definition and append
// primary key columns if missing.
func checkIndexKeys(
	tdef *TableDef,
	index []string,
) ([]string, error) {

	// Track columns already present
	icols := map[string]bool{}

	// Check user-specified index columns
	for _, c := range index {

		// Column must exist in table
		if colIndex(tdef, c) < 0 {
			return nil, fmt.Errorf(
				"column does not exist: %s",
				c,
			)
		}

		// No duplicate columns allowed
		if icols[c] {
			return nil, fmt.Errorf(
				"duplicate index column: %s",
				c,
			)
		}

		icols[c] = true
	}

	// Append primary key columns if not already present
	for _, c := range tdef.Cols[:tdef.PKeys] {

		if !icols[c] {

			index = append(
				index,
				c,
			)
		}
	}

	// Index should not contain every table column
	btree.Assert(
		len(index) < len(tdef.Cols),
	)

	return index, nil
}

// Return position of column in table definition.
// Returns -1 if column does not exist.
func colIndex(
	tdef *TableDef,
	col string,
) int {

	for i, c := range tdef.Cols {

		if c == col {
			return i
		}
	}

	return -1
}

const (
	INDEX_ADD = 1
	INDEX_DEL = 2
)

// Maintain indexes after a record is
// added or removed.
func indexOp(
	db *DB,
	tdef *TableDef,
	rec Record,
	op int,
) {

	key := make(
		[]byte,
		0,
		256,
	)

	irec := make(
		[]Value,
		len(tdef.Cols),
	)

	for i, index := range tdef.Indexes {

		// Build indexed key
		for j, c := range index {

			irec[j] =
				*rec.Get(c)
		}

		// Encode index key
		key = encodeKey(
			key[:0],
			tdef.IndexPrefixes[i],
			irec[:len(index)],
		)

		done := false
		var err error

		switch op {

		case INDEX_ADD:

			done, err =
				db.kv.Update(&btree.InsertReq{Key: key})

		case INDEX_DEL:

			done, err =
				db.kv.Del(&btree.DeleteReq{Key: key})

		default:
			panic("what?")
		}

		// Will be fixed later with transactions
		btree.Assert(err == nil)
		btree.Assert(done)
	}
}

func findIndex(
	tdef *TableDef,
	keys []string,
) (int, error) {

	pk := tdef.Cols[:tdef.PKeys]

	// Use primary key.
	// Also works for full table scans.
	if isPrefix(pk, keys) {
		return -1, nil
	}

	// Find suitable secondary index
	winner := -2

	for i, index := range tdef.Indexes {

		if !isPrefix(index, keys) {
			continue
		}

		if winner == -2 ||
			len(index) <
				len(tdef.Indexes[winner]) {

			winner = i
		}
	}

	if winner == -2 {
		return -2,
			fmt.Errorf(
				"no index found",
			)
	}

	return winner, nil
}

func isPrefix(
	long []string,
	short []string,
) bool {

	if len(long) < len(short) {
		return false
	}

	for i, c := range short {

		if long[i] != c {
			return false
		}
	}

	return true
}
