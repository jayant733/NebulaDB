package wal

import (
	"strings"
	"testing"

	"github.com/jayant/nebuladb/internal/metrics"
)

func TestAppendCountsFsync(t *testing.T) {
	dir := t.TempDir()
	w, err := Open(dir+"/log", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Record{Type: RecPut, Key: []byte("k"), Value: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	var b strings.Builder
	metrics.Default.WritePrometheus(&b)
	if !strings.Contains(b.String(), "nebuladb_wal_fsyncs_total") {
		t.Fatalf("%s", b.String())
	}
}
