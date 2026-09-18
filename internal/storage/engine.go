// Package storage is the Phase 1 durable key-value engine (WAL + MemTable).
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/jayant/nebuladb/internal/storage/memtable"
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
	Dir  string
	Sync SyncMode
}

// Engine is a single-node KV store. The WAL is the source of truth on disk.
type Engine struct {
	mu   sync.Mutex // serializes WAL appends + apply
	opts Options
	wal  *wal.WAL
	mem  *memtable.Table
}

// Open creates dir if needed, opens the WAL, and replays into a MemTable.
func Open(opts Options) (*Engine, error) {
	if opts.Dir == "" {
		return nil, fmt.Errorf("storage: empty data dir")
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, err
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
		return nil, err
	}

	sync := opts.Sync == SyncAlways
	w, err := wal.Open(path, sync)
	if err != nil {
		return nil, err
	}
	return &Engine{opts: opts, wal: w, mem: mem}, nil
}

// Close syncs and closes the WAL.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.wal == nil {
		return nil
	}
	err := e.wal.Close()
	e.wal = nil
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
	return nil
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
	return nil
}

// Get reads the latest MemTable value.
func (e *Engine) Get(key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, fmt.Errorf("storage: empty key")
	}
	e.mu.Lock()
	closed := e.wal == nil
	e.mu.Unlock()
	if closed {
		return nil, false, fmt.Errorf("storage: engine closed")
	}
	v, ok := e.mem.Get(key)
	return v, ok, nil
}

// Stats is a snapshot of engine state for the REPL.
type Stats struct {
	LiveKeys   int
	ApproxSize int64
	Dir        string
}

// Stats returns MemTable counters.
func (e *Engine) Stats() Stats {
	return Stats{
		LiveKeys:   e.mem.Len(),
		ApproxSize: e.mem.ApproxSize(),
		Dir:        e.opts.Dir,
	}
}

// Scan visits live keys in order.
func (e *Engine) Scan(fn func(key, value []byte) bool) {
	e.mem.Scan(fn)
}
