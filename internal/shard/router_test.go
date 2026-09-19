package shard

import (
	"bytes"
	"testing"

	"github.com/jayant/nebuladb/internal/storage"
)

func TestRouterPointAndScan(t *testing.T) {
	rt, engs := openRouter(t, 3)
	defer closeEngines(engs)

	if err := rt.Set([]byte("alpha"), []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := rt.Set([]byte("beta"), []byte("2")); err != nil {
		t.Fatal(err)
	}
	if err := rt.Set([]byte("gamma"), []byte("3")); err != nil {
		t.Fatal(err)
	}
	v, ok, err := rt.Get([]byte("beta"))
	if err != nil || !ok || string(v) != "2" {
		t.Fatalf("get: %q %v %v", v, ok, err)
	}
	if err := rt.Delete([]byte("alpha")); err != nil {
		t.Fatal(err)
	}
	if _, ok, err = rt.Get([]byte("alpha")); err != nil || ok {
		t.Fatalf("deleted still present: %v %v", ok, err)
	}

	var keys []string
	rt.ScanPrefix(nil, func(k, v []byte) bool {
		keys = append(keys, string(k))
		return true
	})
	if len(keys) != 2 || keys[0] != "beta" || keys[1] != "gamma" {
		t.Fatalf("scan %v", keys)
	}

	owners := map[ID]bool{}
	for _, k := range []string{"beta", "gamma", "delta", "eps", "zeta"} {
		owners[rt.Owner([]byte(k))] = true
	}
	if len(owners) < 2 {
		t.Fatalf("expected keys on more than one shard, got %v", owners)
	}

	// Point read hits only the owning engine.
	key := []byte("beta")
	owner := rt.Owner(key)
	hits := 0
	for id, e := range mapEngines(rt, engs) {
		_, ok, err := e.Get(key)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			hits++
			if id != owner {
				t.Fatalf("key on %s want %s", id, owner)
			}
		}
	}
	if hits != 1 {
		t.Fatalf("hits %d", hits)
	}
}

func TestRouterScanPrefix(t *testing.T) {
	rt, engs := openRouter(t, 3)
	defer closeEngines(engs)
	_ = rt.Set([]byte("p/a"), []byte("1"))
	_ = rt.Set([]byte("p/b"), []byte("2"))
	_ = rt.Set([]byte("q/a"), []byte("3"))
	var got []string
	rt.ScanPrefix([]byte("p/"), func(k, v []byte) bool {
		got = append(got, string(k))
		return true
	})
	if len(got) != 2 || got[0] != "p/a" || got[1] != "p/b" {
		t.Fatalf("%v", got)
	}
}

func TestRouterSQLRoundTrip(t *testing.T) {
	rt, engs := openRouter(t, 3)
	defer closeEngines(engs)
	// catalog + row keys hash independently; SELECT still scatter-gathers.
	if err := rt.Set([]byte("p\x00t\x00pk"), []byte("row")); err != nil {
		t.Fatal(err)
	}
	var n int
	rt.ScanPrefix([]byte("p\x00t\x00"), func(k, v []byte) bool {
		if bytes.Equal(v, []byte("row")) {
			n++
		}
		return true
	})
	if n != 1 {
		t.Fatalf("rows %d", n)
	}
}

func openRouter(t *testing.T, n int) (*Router, []*storage.Engine) {
	t.Helper()
	ids := NumericIDs(n)
	ring, err := NewRing(ids, 32)
	if err != nil {
		t.Fatal(err)
	}
	stores := map[ID]storage.KV{}
	engs := make([]*storage.Engine, n)
	for i, id := range ids {
		e, err := storage.Open(storage.Options{Dir: t.TempDir(), Sync: storage.SyncNone})
		if err != nil {
			t.Fatal(err)
		}
		engs[i] = e
		stores[id] = e
	}
	rt, err := NewRouter(ring, stores)
	if err != nil {
		t.Fatal(err)
	}
	return rt, engs
}

func closeEngines(engs []*storage.Engine) {
	for _, e := range engs {
		_ = e.Close()
	}
}

func mapEngines(rt *Router, engs []*storage.Engine) map[ID]*storage.Engine {
	out := map[ID]*storage.Engine{}
	ids := rt.ring.IDs()
	for i, id := range ids {
		out[id] = engs[i]
	}
	return out
}
