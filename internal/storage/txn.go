package storage

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Isolation is a transaction isolation level.
type Isolation int

const (
	// ReadCommitted: no dirty reads; each read sees latest committed data.
	ReadCommitted Isolation = iota
	// RepeatableRead: snapshot at Begin; commit aborts on write-write conflict.
	RepeatableRead
)

var (
	// ErrConflict is returned when Repeatable Read COMMIT hits a write-write conflict.
	ErrConflict = errors.New("storage: write-write conflict")
	// ErrTxnDone is returned if the transaction already committed or rolled back.
	ErrTxnDone = errors.New("storage: transaction is closed")
)

type writeOp struct {
	del bool
	val []byte
}

// Txn is a buffered transaction over an Engine.
type Txn struct {
	e       *Engine
	mu      sync.Mutex
	iso     Isolation
	writes  map[string]writeOp
	snap    map[string][]byte
	snapSeq uint64
	done    bool
}

// Begin starts a transaction.
func (e *Engine) Begin(iso Isolation) *Txn {
	t := &Txn{e: e, iso: iso, writes: map[string]writeOp{}}
	if iso == RepeatableRead {
		e.mu.Lock()
		t.snapSeq = e.seq
		t.snap = map[string][]byte{}
		e.scanLiveLocked(func(k, v []byte) bool {
			t.snap[string(k)] = append([]byte(nil), v...)
			return true
		})
		e.mu.Unlock()
	}
	return t
}

func (t *Txn) check() error {
	if t.done {
		return ErrTxnDone
	}
	return nil
}

// Get implements KV.
func (t *Txn) Get(key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, fmt.Errorf("storage: empty key")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.check(); err != nil {
		return nil, false, err
	}
	if w, ok := t.writes[string(key)]; ok {
		if w.del {
			return nil, false, nil
		}
		return append([]byte(nil), w.val...), true, nil
	}
	if t.iso == RepeatableRead {
		v, ok := t.snap[string(key)]
		if !ok {
			return nil, false, nil
		}
		return append([]byte(nil), v...), true, nil
	}
	return t.e.Get(key)
}

// Set implements KV.
func (t *Txn) Set(key, value []byte) error {
	if len(key) == 0 {
		return fmt.Errorf("storage: empty key")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.check(); err != nil {
		return err
	}
	t.writes[string(key)] = writeOp{val: append([]byte(nil), value...)}
	return nil
}

// Delete implements KV.
func (t *Txn) Delete(key []byte) error {
	if len(key) == 0 {
		return fmt.Errorf("storage: empty key")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.check(); err != nil {
		return err
	}
	t.writes[string(key)] = writeOp{del: true}
	return nil
}

// ScanPrefix implements KV.
func (t *Txn) ScanPrefix(prefix []byte, fn func(key, value []byte) bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return
	}
	latest := map[string][]byte{}
	if t.iso == RepeatableRead {
		for k, v := range t.snap {
			if len(prefix) == 0 || bytes.HasPrefix([]byte(k), prefix) {
				latest[k] = v
			}
		}
	} else {
		t.e.ScanPrefix(prefix, func(k, v []byte) bool {
			latest[string(k)] = append([]byte(nil), v...)
			return true
		})
	}
	for k, w := range t.writes {
		kb := []byte(k)
		if len(prefix) != 0 && !bytes.HasPrefix(kb, prefix) {
			continue
		}
		if w.del {
			delete(latest, k)
			continue
		}
		latest[k] = w.val
	}
	keys := make([]string, 0, len(latest))
	for k := range latest {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !fn([]byte(k), latest[k]) {
			return
		}
	}
}

// Commit applies the write set. Other sessions then see the writes.
func (t *Txn) Commit() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.check(); err != nil {
		return err
	}
	t.e.mu.Lock()
	defer t.e.mu.Unlock()
	if t.iso == RepeatableRead {
		for k := range t.writes {
			if t.e.versions[k] > t.snapSeq {
				t.done = true
				return ErrConflict
			}
		}
	}
	keys := make([]string, 0, len(t.writes))
	for k := range t.writes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		w := t.writes[k]
		var err error
		if w.del {
			err = t.e.deleteLocked([]byte(k))
		} else {
			err = t.e.setLocked([]byte(k), w.val)
		}
		if err != nil {
			t.done = true
			return err
		}
	}
	t.done = true
	return nil
}

// Rollback discards the write set.
func (t *Txn) Rollback() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.check(); err != nil {
		return err
	}
	t.done = true
	t.writes = nil
	return nil
}
