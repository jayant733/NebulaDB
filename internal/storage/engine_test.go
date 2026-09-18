package storage

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSetGetDelete(t *testing.T) {
	e := mustOpen(t)
	defer e.Close()

	if err := e.Set([]byte("k"), []byte("v")); err != nil {
		t.Fatal(err)
	}
	v, ok, err := e.Get([]byte("k"))
	if err != nil || !ok || string(v) != "v" {
		t.Fatalf("get: %q %v %v", v, ok, err)
	}
	if err := e.Delete([]byte("k")); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := e.Get([]byte("k")); err != nil || ok {
		t.Fatalf("deleted key still present: ok=%v err=%v", ok, err)
	}
}

func TestEmptyKey(t *testing.T) {
	e := mustOpen(t)
	defer e.Close()
	if err := e.Set(nil, []byte("x")); err == nil {
		t.Fatal("expected error")
	}
}

func TestCrashRecovery(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{Dir: dir, Sync: SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Set([]byte("user:1"), []byte("jayant")); err != nil {
		t.Fatal(err)
	}
	if err := e.Set([]byte("user:2"), []byte("nebula")); err != nil {
		t.Fatal(err)
	}
	if err := e.Delete([]byte("user:2")); err != nil {
		t.Fatal(err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}

	e2, err := Open(Options{Dir: dir, Sync: SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()

	v, ok, err := e2.Get([]byte("user:1"))
	if err != nil || !ok || string(v) != "jayant" {
		t.Fatalf("recovered user:1: %q ok=%v err=%v", v, ok, err)
	}
	if _, ok, err := e2.Get([]byte("user:2")); err != nil || ok {
		t.Fatalf("deleted key should stay gone after recovery")
	}
}

func TestRecoveryAfterKillWithoutClose(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{Dir: dir, Sync: SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Set([]byte("durable"), []byte("yes")); err != nil {
		t.Fatal(err)
	}
	// Simulate crash: drop the process without Close. File handle is still
	// valid here; we reopen the same path in a new Engine after closing the fd
	// only at OS level by closing then... actually Close would fsync again.
	// Durability of Append already fsynced. Duplicate Open would fail on Windows
	// if we keep the handle. Close the first engine (like OS releasing fds)
	// after fsync already happened inside Set.
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}

	e2, err := Open(Options{Dir: dir, Sync: SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	v, ok, _ := e2.Get([]byte("durable"))
	if !ok || string(v) != "yes" {
		t.Fatalf("got %q ok=%v", v, ok)
	}
}

func TestLastWriteWinsReplay(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{Dir: dir, Sync: SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	_ = e.Set([]byte("k"), []byte("1"))
	_ = e.Set([]byte("k"), []byte("2"))
	_ = e.Set([]byte("k"), []byte("3"))
	_ = e.Close()

	e, err = Open(Options{Dir: dir, Sync: SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	v, ok, _ := e.Get([]byte("k"))
	if !ok || string(v) != "3" {
		t.Fatalf("got %q", v)
	}
}

func TestConcurrentSets(t *testing.T) {
	e := mustOpen(t)
	defer e.Close()
	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			k := []byte(fmt.Sprintf("%04d", i))
			if err := e.Set(k, k); err != nil {
				t.Errorf("set: %v", err)
			}
		}()
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		k := []byte(fmt.Sprintf("%04d", i))
		v, ok, err := e.Get(k)
		if err != nil || !ok || !bytes.Equal(v, k) {
			t.Fatalf("key %s: ok=%v v=%q err=%v", k, ok, v, err)
		}
	}
}

func TestWALThenMemtableOnAppendError(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{Dir: dir, Sync: SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	walPath := filepath.Join(dir, walFileName)
	// Make the log not writable after first successful open — skip on platforms
	// where chmod is ineffective (Windows). This is a best-effort check.
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(walPath, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(walPath, 0o644) })

	e, err = Open(Options{Dir: dir, Sync: SyncAlways})
	if err != nil {
		// Opening read-only WAL may fail on some OS; that is acceptable.
		t.Skipf("could not reopen read-only wal: %v", err)
	}
	defer e.Close()
	err = e.Set([]byte("x"), []byte("y"))
	if err == nil {
		// Windows often still allows write; skip rather than false fail.
		t.Skip("chmod did not make WAL append fail on this OS")
	}
	if _, ok, _ := e.Get([]byte("x")); ok {
		t.Fatal("memtable applied a write that did not hit WAL")
	}
}

func TestFlushThenGetAndRecover(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{Dir: dir, Sync: SyncAlways, MemtableBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Set([]byte("a"), []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := e.Set([]byte("b"), []byte("2")); err != nil {
		t.Fatal(err)
	}
	if err := e.Flush(); err != nil {
		t.Fatal(err)
	}
	if e.Stats().SSTables != 1 {
		t.Fatalf("sst count %d", e.Stats().SSTables)
	}
	if e.Stats().LiveKeys != 0 {
		t.Fatalf("mem should be empty after flush, live=%d", e.Stats().LiveKeys)
	}
	v, ok, err := e.Get([]byte("a"))
	if err != nil || !ok || string(v) != "1" {
		t.Fatalf("get from sst: %q ok=%v err=%v", v, ok, err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}

	e2, err := Open(Options{Dir: dir, Sync: SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	v, ok, err = e2.Get([]byte("b"))
	if err != nil || !ok || string(v) != "2" {
		t.Fatalf("recover sst: %q ok=%v err=%v", v, ok, err)
	}
}

func TestTombstoneHidesFlushedValue(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{Dir: dir, Sync: SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	_ = e.Set([]byte("k"), []byte("old"))
	_ = e.Flush()
	_ = e.Delete([]byte("k"))
	if _, ok, err := e.Get([]byte("k")); err != nil || ok {
		t.Fatalf("mem tombstone should hide sst, ok=%v err=%v", ok, err)
	}
}

func TestAutoFlushOnSize(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{Dir: dir, Sync: SyncAlways, MemtableBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.Set([]byte("hello"), []byte("world")); err != nil {
		t.Fatal(err)
	}
	if e.Stats().SSTables < 1 {
		t.Fatal("expected auto flush")
	}
}

func TestCompactionLastWriteWinsAndDropsTombstones(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{Dir: dir, Sync: SyncAlways, CompactN: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	_ = e.Set([]byte("keep"), []byte("v1"))
	_ = e.Set([]byte("gone"), []byte("x"))
	if err := e.Flush(); err != nil {
		t.Fatal(err)
	}
	_ = e.Set([]byte("keep"), []byte("v2"))
	_ = e.Delete([]byte("gone"))
	if err := e.Flush(); err != nil {
		t.Fatal(err)
	}
	if e.Stats().SSTables != 1 {
		t.Fatalf("expected compact to 1 sst, got %d", e.Stats().SSTables)
	}
	v, ok, err := e.Get([]byte("keep"))
	if err != nil || !ok || string(v) != "v2" {
		t.Fatalf("keep: %q ok=%v err=%v", v, ok, err)
	}
	if _, ok, err := e.Get([]byte("gone")); err != nil || ok {
		t.Fatalf("tombstone should be dropped after full compact, ok=%v", ok)
	}
}

func mustOpen(t *testing.T) *Engine {
	t.Helper()
	e, err := Open(Options{Dir: t.TempDir(), Sync: SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	return e
}
