package table

import (
	"bytes"
	"encoding/binary"
	"forgedb/internal/storage/btree"
)

// Order-preserving encoding of values.
// The encoded bytes can be compared directly using bytes.Compare().
func encodeValues(
	out []byte,
	vals []Value,
) []byte {

	for _, v := range vals {

		switch v.Type {

		case TYPE_INT64:

			var buf [8]byte

			// Shift signed int64 into unsigned range
			// so that negative values come first.
			u := uint64(v.I64) + (1 << 63)

			binary.BigEndian.PutUint64(
				buf[:],
				u,
			)

			out = append(
				out,
				buf[:]...,
			)

		case TYPE_BYTES:

			out = append(
				out,
				escapeString(v.Str)...,
			)

			// Null terminator
			out = append(out, 0)

		default:
			panic("unknown value type")
		}
	}

	return out
}

func decodeValues(
	in []byte,
	out []Value,
) {

	pos := 0

	for i := range out {

		switch out[i].Type {

		case TYPE_INT64:

			btree.Assert(
				pos+8 <= len(in),
			)

			u := binary.BigEndian.Uint64(
				in[pos : pos+8],
			)

			out[i].I64 = int64(
				u - (1 << 63),
			)

			pos += 8

		case TYPE_BYTES:

			var str []byte

			for {

				btree.Assert(
					pos < len(in),
				)

				ch := in[pos]
				pos++

				// End of string.
				if ch == 0 {
					break
				}

				// Escaped byte.
				if ch == 1 {

					btree.Assert(
						pos < len(in),
					)

					esc := in[pos]
					pos++

					str = append(
						str,
						esc-1,
					)

				} else {

					str = append(
						str,
						ch,
					)
				}
			}

			out[i].Str = str

		default:
			panic("unknown value type")
		}
	}
}

// For primary keys.
func encodeKey(
	out []byte,
	prefix uint32,
	vals []Value,
) []byte {

	var buf [4]byte

	binary.BigEndian.PutUint32(
		buf[:],
		prefix,
	)

	out = append(out, buf[:]...)

	out = encodeValues(out, vals)

	return out
}

// Escape null bytes so that encoded strings
// never contain a raw 0x00 byte.
func escapeString(in []byte) []byte {

	zeros := bytes.Count(
		in,
		[]byte{0},
	)

	ones := bytes.Count(
		in,
		[]byte{1},
	)

	if zeros+ones == 0 {
		return in
	}

	out := make(
		[]byte,
		len(in)+zeros+ones,
	)

	pos := 0

	if len(in) > 0 && in[0] >= 0xfe {
		out[0] = 0xfe
		out[1] = in[0]
		pos += 2
		in = in[1:]
	}

	for _, ch := range in {

		if ch <= 1 {

			out[pos+0] = 0x01
			out[pos+1] = ch + 1

			pos += 2

		} else {

			out[pos] = ch

			pos++
		}
	}

	return out
}

// The range key can be a prefix of the index key.
// We may have to encode missing columns
// to make comparison work correctly.
func encodeKeyPartial(
	out []byte,
	prefix uint32,
	values []Value,
	tdef *TableDef,
	keys []string,
	cmp int,
) []byte {

	out = encodeKey(
		out,
		prefix,
		values,
	)

	// Encode missing columns as either
	// minimum or maximum values
	// depending on comparison operator.
	//
	// 1. Empty suffix already behaves like MIN,
	//    so nothing needed for CMP_LT and CMP_GE.
	//
	// 2. For CMP_GT and CMP_LE,
	//    append MAX values.

	max := cmp == btree.CMP_GT ||
		cmp == btree.CMP_LE

loop:
	for i := len(values); max &&
		i < len(keys); i++ {

		switch tdef.Types[colIndex(
			tdef,
			keys[i],
		)] {

		case TYPE_BYTES:

			out = append(
				out,
				0xff,
			)

			// No string encoding can
			// start with 0xff
			break loop

		case TYPE_INT64:

			out = append(
				out,
				0xff, 0xff,
				0xff, 0xff,
				0xff, 0xff,
				0xff, 0xff,
			)

		default:
			panic("what?")
		}
	}

	return out
}
