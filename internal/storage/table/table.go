package table

import (
	"fmt"
	"forgedb/internal/storage/btree"

	"encoding/binary"
	"encoding/json"
)

const TABLE_PREFIX_MIN = 3

// Get a single row by the primary key.
// Get a single row by primary key.
func dbGet(
	db *DB,
	tdef *TableDef,
	rec *Record,
) (bool, error) {

	// Just a shortcut for scan operation.
	sc := Scanner{
		Cmp1: btree.CMP_GE,
		Cmp2: btree.CMP_LE,
		Key1: *rec,
		Key2: *rec,
	}

	if err := dbScan(
		db,
		tdef,
		&sc,
	); err != nil {
		return false, err
	}

	if sc.Valid() {

		sc.Deref(rec)

		return true, nil

	} else {

		return false, nil
	}
}

// Helper function,organizes record in order and checks if all primary keys are present
func checkRecord(
	tdef *TableDef,
	rec Record,
	n int,
) ([]Value, error) {

	btree.Assert(n >= tdef.PKeys)
	btree.Assert(n <= len(tdef.Cols))

	// Build:
	// column name -> value
	input := make(map[string]Value)

	for i, col := range rec.Cols {
		input[col] = rec.Vals[i]
	}

	values := make([]Value, len(tdef.Cols))

	// Verify required columns and reorder.
	for i, col := range tdef.Cols {

		val, ok := input[col]

		if ok {
			values[i] = val
			continue
		}

		// Missing required column?
		if i < n {
			return nil, fmt.Errorf(
				"missing column: %s",
				col,
			)
		}
	}

	return values, nil
}

// Get a single row by the primary key.Public Wrapper function
func (db *DB) Get(
	table string,
	rec *Record,
) (bool, error) {

	tdef := getTableDef(db, table)

	if tdef == nil {
		return false, fmt.Errorf(
			"table not found: %s",
			table,
		)
	}

	return dbGet(db, tdef, rec)
}

// add a row to the table
func dbUpdate(db *DB, tdef *TableDef, rec Record, mode int) (bool, error) {
	values, err := checkRecord(tdef, rec, len(tdef.Cols))

	if err != nil {
		return false, err
	}

	key := encodeKey(nil, tdef.Prefix, values[:tdef.PKeys])

	val := encodeValues(nil, values[tdef.PKeys:])

	req := btree.InsertReq{Key: key, Val: val, Mode: mode}

	added, err := db.kv.Update(&req)

	if err != nil ||
		!req.Updated ||
		len(tdef.Indexes) == 0 {

		return added, err
	}

	// Maintain indexes

	// Existing row was modified
	if req.Updated &&
		!req.Added {

		// Recover old row
		decodeValues(
			req.Old,
			values[tdef.PKeys:],
		)

		indexOp(
			db,
			tdef,
			Record{
				Cols: tdef.Cols,
				Vals: values,
			},
			INDEX_DEL,
		)
	}

	// Insert new index entries
	if req.Updated {

		indexOp(
			db,
			tdef,
			rec,
			INDEX_ADD,
		)
	}

	return added, nil

}

func (db *DB) Set(
	table string,
	rec Record,
	mode int,
) (bool, error) {

	tdef := getTableDef(db, table)

	if tdef == nil {
		return false,
			fmt.Errorf(
				"table not found: %s",
				table,
			)
	}

	return dbUpdate(
		db,
		tdef,
		rec,
		mode,
	)
}

func (db *DB) Insert(
	table string,
	rec Record,
) (bool, error) {
	return db.Set(
		table,
		rec,
		btree.MODE_INSERT_ONLY,
	)
}

func (db *DB) Update(
	table string,
	rec Record,
) (bool, error) {
	return db.Set(
		table,
		rec,
		btree.MODE_UPDATE_ONLY,
	)
}

func (db *DB) Upsert(
	table string,
	rec Record,
) (bool, error) {
	return db.Set(
		table,
		rec,
		btree.MODE_UPSERT,
	)
}

// Delete a record by its primary key.
func dbDelete(
	db *DB,
	tdef *TableDef,
	rec Record,
) (bool, error) {

	values, err := checkRecord(
		tdef,
		rec,
		tdef.PKeys,
	)

	if err != nil {
		return false, err
	}

	key := encodeKey(
		nil,
		tdef.Prefix,
		values[:tdef.PKeys],
	)

	// If there are secondary indexes, we must fetch the old record first to know its values
	var oldRec Record
	if len(tdef.Indexes) > 0 {
		oldRec.Cols = tdef.Cols[:tdef.PKeys]
		oldRec.Vals = values[:tdef.PKeys]
		ok, err := dbGet(db, tdef, &oldRec)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}

	deleted, err := db.kv.Del(&btree.DeleteReq{Key: key})

	if err != nil || !deleted || len(tdef.Indexes) == 0 {
		return deleted, err
	}

	//delete indexes
	if deleted {
		indexOp(db, tdef, oldRec, INDEX_DEL)
	}

	return deleted, nil
}

// Delete a record by its primary key.Public Wrapper function
func (db *DB) Delete(
	table string,
	rec Record,
) (bool, error) {

	tdef := getTableDef(
		db,
		table,
	)

	if tdef == nil {
		return false,
			fmt.Errorf(
				"table not found: %s",
				table,
			)
	}

	return dbDelete(
		db,
		tdef,
		rec,
	)
}

// Create a new table.Public wrapper function
func (db *DB) TableNew(
	tdef *TableDef,
) error {

	if err := tableDefCheck(tdef); err != nil {
		return err
	}

	// Check existing table
	table := (&Record{}).
		AddStr("name", []byte(tdef.Name))

	ok, err := dbGet(
		db,
		TDEF_TABLE,
		table,
	)

	btree.Assert(err == nil)

	if ok {
		return fmt.Errorf(
			"table exists: %s",
			tdef.Name,
		)
	}

	// Allocate a new prefix
	btree.Assert(tdef.Prefix == 0)

	tdef.Prefix = TABLE_PREFIX_MIN

	meta := (&Record{}).
		AddStr(
			"key",
			[]byte("next_prefix"),
		)

	ok, err = dbGet(
		db,
		TDEF_META,
		meta,
	)

	btree.Assert(err == nil)

	if ok {

		v := meta.Get("val")

		tdef.Prefix =
			binary.LittleEndian.Uint32(
				v.Str,
			)

		btree.Assert(
			tdef.Prefix >
				TABLE_PREFIX_MIN,
		)

	} else {

		meta.AddStr(
			"val",
			make([]byte, 4),
		)
	}

	// Assign prefixes to indexes
	for i := range tdef.Indexes {

		prefix := tdef.Prefix +
			1 +
			uint32(i)

		tdef.IndexPrefixes = append(
			tdef.IndexPrefixes,
			prefix,
		)
	}

	// Update next available prefix
	// 1 for table + number of indexes
	ntree := 1 +
		uint32(
			len(tdef.Indexes),
		)

	v := meta.Get("val")

	binary.LittleEndian.PutUint32(
		v.Str,
		tdef.Prefix+ntree,
	)

	_, err = dbUpdate(
		db,
		TDEF_META,
		*meta,
		0,
	)

	if err != nil {
		return err
	}

	// Store table definition
	val, err := json.Marshal(tdef)

	btree.Assert(err == nil)

	table.AddStr(
		"def",
		val,
	)

	_, err = dbUpdate(
		db,
		TDEF_TABLE,
		*table,
		0,
	)

	return err
}

// public wrapper function
func (db *DB) Scan(table string, req *Scanner) error {
	tdef := getTableDef(db, table)
	if tdef == nil {
		return fmt.Errorf("table not found:%s", table)
	}
	return dbScan(db, tdef, req)
}
