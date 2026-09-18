package engine

import (
	"encoding/binary"
	"fmt"

	"github.com/jayant/nebuladb/internal/sql/ast"
)

func encodeInt(n int64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(n)^(1<<63))
	return b[:]
}

func decodeInt(b []byte) (int64, error) {
	if len(b) != 8 {
		return 0, fmt.Errorf("sql: bad int encoding")
	}
	u := binary.BigEndian.Uint64(b) ^ (1 << 63)
	return int64(u), nil
}

func encodeLiteral(lit ast.Literal, typ ast.Type) ([]byte, error) {
	switch typ {
	case ast.TypeInt:
		if !lit.Num {
			return nil, fmt.Errorf("sql: expected INT")
		}
		return encodeInt(lit.Int), nil
	case ast.TypeText:
		if lit.Num {
			return []byte(lit.Text), nil
		}
		return []byte(lit.Text), nil
	default:
		return nil, fmt.Errorf("sql: unknown type")
	}
}

func display(b []byte, typ ast.Type) string {
	if typ == ast.TypeInt {
		n, err := decodeInt(b)
		if err != nil {
			return "?"
		}
		return fmt.Sprintf("%d", n)
	}
	return string(b)
}

func compareStored(a, b []byte, typ ast.Type) int {
	if typ == ast.TypeInt {
		ia, err1 := decodeInt(a)
		ib, err2 := decodeInt(b)
		if err1 != nil || err2 != nil {
			if err1 != nil {
				return -1
			}
			return 1
		}
		switch {
		case ia < ib:
			return -1
		case ia > ib:
			return 1
		default:
			return 0
		}
	}
	switch {
	case string(a) < string(b):
		return -1
	case string(a) > string(b):
		return 1
	default:
		return 0
	}
}

func matchOp(cmp int, op ast.CmpOp) bool {
	switch op {
	case ast.OpEq:
		return cmp == 0
	case ast.OpNe:
		return cmp != 0
	case ast.OpLt:
		return cmp < 0
	case ast.OpGt:
		return cmp > 0
	case ast.OpLe:
		return cmp <= 0
	case ast.OpGe:
		return cmp >= 0
	default:
		return false
	}
}
