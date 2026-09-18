package row

import (
	"bytes"
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	in := [][]byte{[]byte("20"), []byte("jayant"), []byte("")}
	b := Encode(in)
	out, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || !bytes.Equal(out[0], in[0]) || !bytes.Equal(out[1], in[1]) || out[2] == nil && len(in[2]) == 0 {
		t.Fatalf("%q", out)
	}
	if _, ok := Field(out, 1); !ok {
		t.Fatal("field 1")
	}
	if _, ok := Field(out, 9); ok {
		t.Fatal("field 9 should miss")
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	if _, err := Decode([]byte{1}); err == nil {
		t.Fatal("expected error")
	}
}
