// Package storage is the durable key-value engine (WAL + MemTable + SSTables).
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/jayant/nebuladb/internal/storage/memtable"
	"github.com/jayant/nebuladb/internal/storage/sstable"
	"github.com/jayant/nebuladb/internal/storage/wal"
)

// SyncMode controls when the WAL is fsynced.
type SyncMode int

const (
	// SyncAlways fsyncs after every mutation (crash-safe).
	SyncAlways SyncMode = iota
	// SyncNone skips fsync (benchmarks only).
	SyncNone
)

const walFileName = "wal-000001.log"

// Options configure Engine.Open.
type Options struct {
	Dir            string
	Sync           SyncMode
	MemtableBytes  int64 // flush when MemTable reaches this size; 0 → 1 MiB
	CompactN       int   // compact all SSTables when count ≥ N; 0 → 4
}

func (o Options) memLimit() int64 {
	if o.MemtableBytes > 0 {
		return o.MemtableBytes
	}
	return 1 << 20
}

// Engine is a single-node KV store.
type Engine struct {
	mu     sync.Mutex
	opts   Options
	wal    *wal.WAL
	mem    *memtable.Table
	ssts   []*sstable.Reader // oldest → newest
	nextID uint64
}

// Open creates dir if needed, loads SSTables from MANIFEST, then replays the WAL.
func Open(opts Options) (*Engine, error) {
	if opts.Dir == "" {
		return nil, fmt.Errorf("storage: empty data dir")
	}
	if err := os.MkdirAll(filepath.Join(opts.Dir, sstDirName), 0o755); err != nil {
		return nil, err
	}

	ids, err := loadManifest(opts.Dir)
	if err != nil {
		return nil, err
	}
	var ssts []*sstable.Reader
	var maxID uint64
	for _, id := range ids {
		r, err := sstable.Open(sstPath(opts.Dir, id))
		if err != nil {
			for _, s := range ssts {
				_ = s.Close()
			}
			return nil, err
		}
		ssts = append(ssts, r)
		if id > maxID {
			maxID = id
		}
	}

	path := filepath.Join(opts.Dir, walFileName)
	mem := memtable.New()
	if err := wal.Replay(path, func(r wal.Record) error {
		switch r.Type {
		case wal.RecPut:
			mem.Put(r.Key, r.Value)
		case wal.RecDelete:
			mem.Delete(r.Key)
		default:
			return fmt.Errorf("storage: unknown wal type %d", r.Type)
		}
		return nil
	}); err != nil {
		for _, s := range ssts {
			_ = s.Close()
		}
		return nil, err
	}

	sync := opts.Sync == SyncAlways
	w, err := wal.Open(path, sync)
	if err != nil {
		for _, s := range ssts {
			_ = s.Close()
		}
		return nil, err
	}
	return &Engine{opts: opts, wal: w, mem: mem, ssts: ssts, nextID: maxID + 1}, nil
}

// Close syncs the WAL and closes SSTables.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.wal == nil {
		return nil
	}
	err := e.wal.Close()
	e.wal = nil
	for _, s := range e.ssts {
		if cerr := s.Close(); err == nil {
			err = cerr
		}
	}
	e.ssts = nil
	return err
}

// Set writes key=value. Empty keys are rejected.
func (e *Engine) Set(key, value []byte) error {
	if len(key) == 0 {
		return fmt.Errorf("storage: empty key")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.wal == nil {
		return fmt.Errorf("storage: engine closed")
	}
	if err := e.wal.Append(wal.Record{Type: wal.RecPut, Key: key, Value: value}); err != nil {
		return err
	}
	e.mem.Put(key, value)
	return e.maybeFlushLocked()
}

// Delete tombstones key.
func (e *Engine) Delete(key []byte) error {
	if len(key) == 0 {
		return fmt.Errorf("storage: empty key")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.wal == nil {
		return fmt.Errorf("storage: engine closed")
	}
	if err := e.wal.Append(wal.Record{Type: wal.RecDelete, Key: key}); err != nil {
		return err
	}
	e.mem.Delete(key)
	return e.maybeFlushLocked()
}

func (e *Engine) maybeFlushLocked() error {
	if e.mem.ApproxSize() < e.opts.memLimit() {
		return nil
	}
	return e.flushLocked()
}

// Flush writes the MemTable to a new SSTable and rotates the WAL.
func (e *Engine) Flush() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.wal == nil {
		return fmt.Errorf("storage: engine closed")
	}
	return e.flushLocked()
}

func (e *Engine) flushLocked() error {
	var entries []sstable.Entry
	e.mem.ScanAll(func(key, value []byte, deleted bool) bool {
		entries = append(entries, sstable.Entry{
			Key:       append([]byte(nil), key...),
			Value:     append([]byte(nil), value...),
			Tombstone: deleted,
		})
		return true
	})
	if len(entries) == 0 {
		return e.wal.Reset()
	}

	id := e.nextID
	path := sstPath(e.opts.Dir, id)
	if err := sstable.Write(path, entries); err != nil {
		return err
	}
	r, err := sstable.Open(path)
	if err != nil {
		return err
	}

	ids := make([]uint64, 0, len(e.ssts)+1)
	for _, s := range e.ssts {
		ids = append(ids, parseSSTID(s.Path()))
	}
	ids = append(ids, id)
	if err := saveManifest(e.opts.Dir, ids); err != nil {
		_ = r.Close()
		return err
	}
	if err := e.wal.Reset(); err != nil {
		_ = r.Close()
		return err
	}
	e.ssts = append(e.ssts, r)
	e.nextID = id + 1
	e.mem = memtable.New()
	return e.maybeCompactLocked()
}

func parseSSTID(path string) uint64 {
	var id uint64
	_, _ = fmt.Sscanf(filepath.Base(path), "%d.sst", &id)
	return id
}

// Get reads MemTable then SSTables newest-first. A MemTable tombstone hides SST values.
func (e *Engine) Get(key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, fmt.Errorf("storage: empty key")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.wal == nil {
		return nil, false, fmt.Errorf("storage: engine closed")
	}
	if v, st := e.mem.Lookup(key); st == memtable.Found {
		return v, true, nil
	} else if st == memtable.Deleted {
		return nil, false, nil
	}
	for i := len(e.ssts) - 1; i >= 0; i-- {
		v, tomb, ok, err := e.ssts[i].Get(key)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			continue
		}
		if tomb {
			return nil, false, nil
		}
		return v, true, nil
	}
	return nil, false, nil
}

// Stats is a snapshot of engine state for the REPL.
type Stats struct {
	LiveKeys   int
	ApproxSize int64
	Dir        string
	SSTables   int
}

// Stats returns MemTable counters and SST count.
func (e *Engine) Stats() Stats {
	e.mu.Lock()
	defer e.mu.Unlock()
	return Stats{
		LiveKeys:   e.mem.Len(),
		ApproxSize: e.mem.ApproxSize(),
		Dir:        e.opts.Dir,
		SSTables:   len(e.ssts),
	}
}

// Scan visits live keys (MemTable + SSTables, newest wins). Fine for small data.
func (e *Engine) Scan(fn func(key, value []byte) bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	latest := map[string]sstable.Entry{}
	for _, s := range e.ssts {
		it := s.Iter()
		for it.Next() {
			ent := it.Entry()
			latest[string(ent.Key)] = ent
		}
	}
	e.mem.ScanAll(func(key, value []byte, deleted bool) bool {
		latest[string(key)] = sstable.Entry{Key: append([]byte(nil), key...), Value: append([]byte(nil), value...), Tombstone: deleted}
		return true
	})
	keys := make([]string, 0, len(latest))
	for k, ent := range latest {
		if !ent.Tombstone {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		ent := latest[k]
		if !fn([]byte(k), ent.Value) {
			return
		}
	}
}
