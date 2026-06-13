package table

const (
	TYPE_ERROR = 0
	TYPE_BYTES = 1
	TYPE_INT64 = 2
)

// Table cell.
type Value struct {
	Type uint32

	I64 int64
	Str []byte
}

// Table row.
type Record struct {
	Cols []string
	Vals []Value
}

// Adds a string record
func (rec *Record) AddStr(
	key string,
	val []byte,
) *Record {

	rec.Cols = append(rec.Cols, key)

	rec.Vals = append(rec.Vals,
		Value{
			Type: TYPE_BYTES,
			Str:  val,
		},
	)

	return rec
}

// Adds an int64 record
func (rec *Record) AddInt64(key string, val int64) *Record {

	rec.Cols = append(rec.Cols, key)

	rec.Vals = append(rec.Vals, Value{
		Type: TYPE_INT64,
		I64:  val,
	})

	return rec
}

// Get returns the cell for a column.
func (rec *Record) Get(key string) *Value {
	for i, col := range rec.Cols {
		if col == key {
			return &rec.Vals[i]
		}
	}
	return nil
}
