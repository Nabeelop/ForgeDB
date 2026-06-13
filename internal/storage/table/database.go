package table

import (
	"encoding/json"
	"fmt"
	"forgedb/internal/storage"
	"forgedb/internal/storage/btree"
)

type DB struct {
	Path string
	//internals
	kv     storage.KV
	tables map[string]*TableDef //cachedtable definition
}

func (db *DB) Open() error {
	db.kv.Path = db.Path
	return db.kv.Open()
}

func (db *DB) Close() error {
	return db.kv.Close()
}

// Get the table definition by name.
func getTableDef(
	db *DB,
	name string,
) *TableDef {

	tdef, ok := db.tables[name]

	if !ok {

		if db.tables == nil {
			db.tables = map[string]*TableDef{}
		}

		tdef = getTableDefDB(db, name)

		if tdef != nil {
			db.tables[name] = tdef
		}
	}

	return tdef
}

func getTableDefDB(
	db *DB,
	name string,
) *TableDef {

	rec := (&Record{}).
		AddStr("name", []byte(name))

	ok, err := dbGet(
		db,
		TDEF_TABLE,
		rec,
	)

	btree.Assert(err == nil)

	if !ok {
		return nil
	}

	tdef := &TableDef{}

	err = json.Unmarshal(
		rec.Get("def").Str,
		tdef,
	)

	btree.Assert(err == nil)

	return tdef
}

func checkQueryRecord(tdef *TableDef, rec Record) ([]Value, error) {
	values := make([]Value, len(tdef.Cols))
	input := make(map[string]Value)
	for i, col := range rec.Cols {
		input[col] = rec.Vals[i]
	}
	for i, col := range tdef.Cols {
		val, ok := input[col]
		if ok {
			val.Type = tdef.Types[i]
			values[i] = val
		} else {
			values[i] = Value{Type: TYPE_ERROR}
		}
	}
	for _, col := range rec.Cols {
		if colIndex(tdef, col) < 0 {
			return nil, fmt.Errorf("column does not exist: %s", col)
		}
	}
	return values, nil
}

func dbScan(
	db *DB,
	tdef *TableDef,
	req *Scanner,
) error {

	if req.Cmp1 == 0 {
		req.Cmp1 = btree.CMP_GE
	}

	// Sanity checks
	if req.Cmp1 != 0 && req.Cmp2 != 0 {
		switch {
		case req.Cmp1 > 0 && req.Cmp2 < 0:
		case req.Cmp2 > 0 && req.Cmp1 < 0:
		default:
			return fmt.Errorf(
				"bad range",
			)
		}
	}

	// Validate query records
	values1, err := checkQueryRecord(tdef, req.Key1)
	if err != nil {
		return err
	}

	values2, err := checkQueryRecord(tdef, req.Key2)
	if err != nil {
		return err
	}

	// Select index
	indexNo, err := findIndex(
		tdef,
		req.Key1.Cols,
	)

	if err != nil {
		return err
	}

	// Default → primary key tree
	index := tdef.Cols[:tdef.PKeys]
	prefix := tdef.Prefix

	// Secondary index selected
	if indexNo >= 0 {
		index = tdef.Indexes[indexNo]
		prefix = tdef.IndexPrefixes[indexNo]
	}

	// Save scanner state
	req.db = db
	req.tdef = tdef
	req.indexNo = indexNo
	req.prefix = prefix

	// Build the actual values in index order (filtering out any unselected/TYPE_ERROR columns)
	qvals1 := make([]Value, len(req.Key1.Cols))
	for i, col := range index[:len(req.Key1.Cols)] {
		qvals1[i] = values1[colIndex(tdef, col)]
	}

	qvals2 := make([]Value, len(req.Key2.Cols))
	for i, col := range index[:len(req.Key2.Cols)] {
		qvals2[i] = values2[colIndex(tdef, col)]
	}

	// Build start key
	keyStart := encodeKeyPartial(
		nil,
		prefix,
		qvals1,
		tdef,
		index,
		req.Cmp1,
	)

	// Build end key
	req.keyEnd = encodeKeyPartial(
		nil,
		prefix,
		qvals2,
		tdef,
		index,
		req.Cmp2,
	)

	// Position iterator
	req.iter =
		db.kv.Tree.Seek(
			keyStart,
			req.Cmp1,
		)

	return nil
}
