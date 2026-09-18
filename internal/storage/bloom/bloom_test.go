package bloom

import (
	"testing"
)

func TestMayContain(t *testing.T) {
	f := New(100, 0.01)
	keys := []string{"a", "user:1", "zzz"}
	for _, k := range keys {
		f.Add([]byte(k))
	}
	for _, k := range keys {
		if !f.MayContain([]byte(k)) {
			t.Fatalf("false negative for %q", k)
		}
	}
	if f.MayContain([]byte("definitely-missing-key-xyz")) && f.MayContain([]byte("another-missing")) {
		// possible but both at 1% is uncommon; don't fail — only assert no false negatives
	}
}

func TestRoundTrip(t *testing.T) {
	f := New(50, 0.01)
	f.Add([]byte("k1"))
	f.Add([]byte("k2"))
	g, err := Decode(f.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if !g.MayContain([]byte("k1")) || !g.MayContain([]byte("k2")) {
		t.Fatal("decoded filter lost keys")
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	if _, err := Decode([]byte{1}); err == nil {
		t.Fatal("expected error")
	}
}
