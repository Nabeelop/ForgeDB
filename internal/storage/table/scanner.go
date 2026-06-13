package table

import (
	"encoding/binary"
	"forgedb/internal/storage/btree"
)

type Scanner struct {

	// Range boundaries
	Cmp1 int
	Cmp2 int

	Key1 Record
	Key2 Record

	// Internal
	db      *DB
	indexNo int
	prefix  uint32
	tdef    *TableDef
	iter    *btree.BIter
	keyEnd  []byte
}

// Checks is the current key is valid
func (sc *Scanner) Valid() bool {

	if !sc.iter.Valid() {
		return false
	}

	key, _ := sc.iter.Deref()

	if len(key) < 4 || binary.BigEndian.Uint32(key[:4]) != sc.prefix {
		return false
	}

	if sc.Cmp2 != 0 {
		return btree.CmpOK(
			key,
			sc.Cmp2,
			sc.keyEnd,
		)
	}

	return true
}

// Moves the iterator in correct direction based on scan direction
func (sc *Scanner) Next() {

	btree.Assert(sc.Valid())

	if sc.Cmp1 > 0 {

		sc.iter.Next()

	} else {

		sc.iter.Prev()
	}
}

// / Fetch the current row.
func (sc *Scanner) Deref(
	rec *Record,
) {

	btree.Assert(
		sc.Valid(),
	)

	tdef := sc.tdef

	rec.Cols = tdef.Cols
	rec.Vals = rec.Vals[:0]

	key, val := sc.iter.Deref()

	// Scanning primary key tree
	if sc.indexNo < 0 {

		// Decode primary key from key
		pkVals := make(
			[]Value,
			tdef.PKeys,
		)

		for i := 0; i < tdef.PKeys; i++ {

			pkVals[i].Type =
				tdef.Types[i]
		}

		decodeValues(
			key[4:], // skip prefix
			pkVals,
		)

		rec.Vals = append(
			rec.Vals,
			pkVals...,
		)

		// Decode remaining columns from value
		otherVals := make(
			[]Value,
			len(tdef.Cols)-tdef.PKeys,
		)

		for i := tdef.PKeys; i < len(tdef.Cols); i++ {

			otherVals[i-tdef.PKeys].Type = tdef.Types[i]
		}

		decodeValues(
			val,
			otherVals,
		)

		rec.Vals = append(
			rec.Vals,
			otherVals...,
		)

	} else {

		// Scanning secondary index

		// Index values do not store KV value
		btree.Assert(
			len(val) == 0,
		)

		// Decode index key
		index :=
			tdef.Indexes[sc.indexNo]

		ival := make(
			[]Value,
			len(index),
		)

		for i, c := range index {

			ival[i].Type =
				tdef.Types[colIndex(
					tdef,
					c,
				)]
		}

		// Decode index key (skip prefix)
		decodeValues(
			key[4:],
			ival,
		)

		icol := Record{
			Cols: index,
			Vals: ival,
		}

		// Extract primary key
		rec.Cols =
			tdef.Cols[:tdef.PKeys]

		for _, c := range rec.Cols {

			rec.Vals = append(
				rec.Vals,
				*icol.Get(c),
			)
		}

		// Fetch actual row
		// (can optimize later if covering index)
		ok, err := dbGet(
			sc.db,
			tdef,
			rec,
		)

		btree.Assert(
			ok && err == nil,
		)
	}
}
