package shard

import (
	"fmt"
	"path/filepath"

	"github.com/jayant/nebuladb/internal/storage"
)

// OpenLocal opens n engines under dir/shard-<id> and a router in front.
func OpenLocal(dir string, n int, sync storage.SyncMode) (*Router, []*storage.Engine, error) {
	if n < 2 {
		return nil, nil, fmt.Errorf("shard: need at least 2 shards")
	}
	ids := NumericIDs(n)
	ring, err := NewRing(ids, 0)
	if err != nil {
		return nil, nil, err
	}
	stores := make(map[ID]storage.KV, n)
	engs := make([]*storage.Engine, n)
	for i, id := range ids {
		e, err := storage.Open(storage.Options{
			Dir:  filepath.Join(dir, "shard-"+string(id)),
			Sync: sync,
		})
		if err != nil {
			for j := 0; j < i; j++ {
				_ = engs[j].Close()
			}
			return nil, nil, err
		}
		engs[i] = e
		stores[id] = e
	}
	rt, err := NewRouter(ring, stores)
	if err != nil {
		for _, e := range engs {
			_ = e.Close()
		}
		return nil, nil, err
	}
	return rt, engs, nil
}
