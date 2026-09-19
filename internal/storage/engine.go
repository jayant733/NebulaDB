// Package storage is the durable key-value engine (WAL + MemTable + SSTables).
package storage

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/jayant/nebuladb/internal/storage/memtable"
	"github.com/jayant/nebuladb/internal/storage/sstable"
	"github.com/jayant/nebuladb/internal/storage/wal"
)

// ErrDisk is returned when a test injector fails a write before WAL append.
var ErrDisk = errors.New("storage: injected disk error")

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
	Dir           string
	Sync          SyncMode
	MemtableBytes int64 // flush when MemTable reaches this size; 0 → 1 MiB
	CompactN      int   // compact all SSTables when count ≥ N; 0 → 4
}

func (o Options) memLimit() int64 {
	if o.MemtableBytes > 0 {
		return o.MemtableBytes
	}
	return 1 << 20
}

// Engine is a single-node KV store.
type Engine struct {
	mu         sync.Mutex
	opts       Options
	wal        *wal.WAL
	mem        *memtable.Table
	ssts       []*sstable.Reader // oldest → newest
	nextID     uint64
	seq        uint64
	versions   map[string]uint64
	failWrites int
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
	return &Engine{opts: opts, wal: w, mem: mem, ssts: ssts, nextID: maxID + 1, versions: map[string]uint64{}}, nil
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
	return e.setLocked(key, value)
}

func (e *Engine) setLocked(key, value []byte) error {
	if e.wal == nil {
		return fmt.Errorf("storage: engine closed")
	}
	if err := e.faultLocked(); err != nil {
		return err
	}
	if err := e.wal.Append(wal.Record{Type: wal.RecPut, Key: key, Value: value}); err != nil {
		return err
	}
	e.mem.Put(key, value)
	e.bumpVersion(key)
	return e.maybeFlushLocked()
}

// Delete tombstones key.
func (e *Engine) Delete(key []byte) error {
	if len(key) == 0 {
		return fmt.Errorf("storage: empty key")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.deleteLocked(key)
}

func (e *Engine) deleteLocked(key []byte) error {
	if e.wal == nil {
		return fmt.Errorf("storage: engine closed")
	}
	if err := e.faultLocked(); err != nil {
		return err
	}
	if err := e.wal.Append(wal.Record{Type: wal.RecDelete, Key: key}); err != nil {
		return err
	}
	e.mem.Delete(key)
	e.bumpVersion(key)
	return e.maybeFlushLocked()
}

// InjectDiskErrors fails the next n Set/Delete calls before WAL append.
func (e *Engine) InjectDiskErrors(n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.failWrites = n
}

func (e *Engine) faultLocked() error {
	if e.failWrites <= 0 {
		return nil
	}
	e.failWrites--
	return ErrDisk
}

func (e *Engine) bumpVersion(key []byte) {
	e.seq++
	if e.versions == nil {
		e.versions = map[string]uint64{}
	}
	e.versions[string(key)] = e.seq
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
	return e.getLocked(key)
}

func (e *Engine) getLocked(key []byte) ([]byte, bool, error) {
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
	LiveKeys       int
	ApproxSize     int64
	Dir            string
	SSTables       int
	WALBytes       int64
	BloomChecked   uint64
	BloomNegatives uint64
}

// Stats returns MemTable, WAL, SST, and Bloom counters.
func (e *Engine) Stats() Stats {
	e.mu.Lock()
	defer e.mu.Unlock()
	var walBytes int64
	if e.wal != nil {
		walBytes, _ = e.wal.Size()
	}
	var checked, neg uint64
	for _, s := range e.ssts {
		c, n := s.BloomStats()
		checked += c
		neg += n
	}
	return Stats{
		LiveKeys:       e.mem.Len(),
		ApproxSize:     e.mem.ApproxSize(),
		Dir:            e.opts.Dir,
		SSTables:       len(e.ssts),
		WALBytes:       walBytes,
		BloomChecked:   checked,
		BloomNegatives: neg,
	}
}

// Scan visits live keys (MemTable + SSTables, newest wins). Fine for small data.
func (e *Engine) Scan(fn func(key, value []byte) bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.scanLiveLocked(fn)
}

func (e *Engine) scanLiveLocked(fn func(key, value []byte) bool) {
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

// ScanPrefix visits live keys with the given prefix, in order.
func (e *Engine) ScanPrefix(prefix []byte, fn func(key, value []byte) bool) {
	e.Scan(func(k, v []byte) bool {
		if len(prefix) == 0 {
			return fn(k, v)
		}
		if bytes.HasPrefix(k, prefix) {
			return fn(k, v)
		}
		if bytes.Compare(k, prefix) > 0 {
			return false
		}
		return true
	})
}
