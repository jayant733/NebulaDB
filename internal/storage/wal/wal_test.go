package wal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendReplay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal.log")

	w, err := Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	records := []Record{
		{Type: RecPut, Key: []byte("a"), Value: []byte("1")},
		{Type: RecPut, Key: []byte("b"), Value: []byte("2")},
		{Type: RecDelete, Key: []byte("a")},
		{Type: RecPut, Key: []byte("c"), Value: []byte("")},
	}
	for _, r := range records {
		if err := w.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	var got []Record
	if err := Replay(path, func(r Record) error {
		got = append(got, Record{Type: r.Type, Key: append([]byte(nil), r.Key...), Value: append([]byte(nil), r.Value...)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(records) {
		t.Fatalf("len=%d want %d", len(got), len(records))
	}
	for i := range records {
		if got[i].Type != records[i].Type || string(got[i].Key) != string(records[i].Key) || string(got[i].Value) != string(records[i].Value) {
			t.Fatalf("record %d: %+v want %+v", i, got[i], records[i])
		}
	}
}

func TestEmptyKeyRejected(t *testing.T) {
	dir := t.TempDir()
	w, err := Open(filepath.Join(dir, "wal.log"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Append(Record{Type: RecPut, Key: nil, Value: []byte("x")}); err == nil {
		t.Fatal("expected error")
	}
}

func TestTornWriteTruncated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal.log")
	w, err := Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Record{Type: RecPut, Key: []byte("ok"), Value: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0x01, 0x02, 0x03}); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := Replay(path, func(Record) error {
		n++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("replayed %d records, want 1", n)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= 3 {
		t.Fatalf("log was not truncated back; size=%d", info.Size())
	}

	// Second replay should still see exactly one record (garbage gone).
	n = 0
	if err := Replay(path, func(Record) error {
		n++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("after truncate replayed %d", n)
	}
}

func TestCRCMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal.log")
	w, err := Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Record{Type: RecPut, Key: []byte("k"), Value: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b[len(b)-1] ^= 0xff
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}

	err = Replay(path, func(Record) error { return nil })
	if err == nil {
		t.Fatal("expected crc error")
	}
}

func TestReplayMissingFile(t *testing.T) {
	if err := Replay(filepath.Join(t.TempDir(), "nope.log"), func(Record) error {
		t.Fatal("should not be called")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
