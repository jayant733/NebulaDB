package storage

import (
	"fmt"
	"os"
	"sort"

	"github.com/jayant/nebuladb/internal/metrics"
	"github.com/jayant/nebuladb/internal/storage/sstable"
)

func (o Options) compactLimit() int {
	if o.CompactN > 1 {
		return o.CompactN
	}
	return 4
}

func (e *Engine) maybeCompactLocked() error {
	if len(e.ssts) < e.opts.compactLimit() {
		return nil
	}
	return e.compactAllLocked()
}

// Compact merges all SSTables newest-wins and drops tombstones (single level).
func (e *Engine) Compact() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.wal == nil {
		return fmt.Errorf("storage: engine closed")
	}
	if len(e.ssts) < 2 {
		return nil
	}
	return e.compactAllLocked()
}

func (e *Engine) compactAllLocked() error {
	latest := make(map[string]sstable.Entry)
	for _, s := range e.ssts {
		it := s.Iter()
		for it.Next() {
			ent := it.Entry()
			latest[string(ent.Key)] = sstable.Entry{
				Key:       append([]byte(nil), ent.Key...),
				Value:     append([]byte(nil), ent.Value...),
				Tombstone: ent.Tombstone,
			}
		}
		if err := it.Err(); err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(latest))
	for k := range latest {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]sstable.Entry, 0, len(keys))
	for _, k := range keys {
		ent := latest[k]
		if ent.Tombstone {
			continue
		}
		out = append(out, ent)
	}

	id := e.nextID
	path := sstPath(e.opts.Dir, id)
	if err := sstable.Write(path, out); err != nil {
		return err
	}
	r, err := sstable.Open(path)
	if err != nil {
		return err
	}
	if err := saveManifest(e.opts.Dir, []uint64{id}); err != nil {
		_ = r.Close()
		return err
	}

	old := e.ssts
	e.ssts = []*sstable.Reader{r}
	e.nextID = id + 1
	for _, s := range old {
		p := s.Path()
		_ = s.Close()
		_ = os.Remove(p)
	}
	metrics.Default.Inc("nebuladb_compactions_total", "")
	return nil
}
