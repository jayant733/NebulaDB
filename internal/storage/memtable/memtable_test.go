package memtable

import (
	"fmt"
	"sync"
	"testing"
)

func TestPutGetDelete(t *testing.T) {
	m := New()
	m.Put([]byte("b"), []byte("2"))
	m.Put([]byte("a"), []byte("1"))
	m.Put([]byte("c"), []byte("3"))

	if v, ok := m.Get([]byte("a")); !ok || string(v) != "1" {
		t.Fatalf("get a: %q %v", v, ok)
	}
	if _, ok := m.Get([]byte("z")); ok {
		t.Fatal("missing key should be absent")
	}

	m.Delete([]byte("a"))
	if _, ok := m.Get([]byte("a")); ok {
		t.Fatal("tombstone should hide key")
	}
	if m.Len() != 2 {
		t.Fatalf("len=%d", m.Len())
	}

	m.Put([]byte("a"), []byte("again"))
	if v, ok := m.Get([]byte("a")); !ok || string(v) != "again" {
		t.Fatalf("resurrect: %q %v", v, ok)
	}
	if _, st := m.Lookup([]byte("missing")); st != Miss {
		t.Fatalf("missing lookup %v", st)
	}
	m.Delete([]byte("c"))
	if _, st := m.Lookup([]byte("c")); st != Deleted {
		t.Fatalf("tombstone lookup %v", st)
	}
}

func TestScanOrder(t *testing.T) {
	m := New()
	m.Put([]byte("c"), []byte("3"))
	m.Put([]byte("a"), []byte("1"))
	m.Put([]byte("b"), []byte("2"))
	m.Delete([]byte("b"))

	var keys []string
	m.Scan(func(k, v []byte) bool {
		keys = append(keys, string(k)+"="+string(v))
		return true
	})
	want := []string{"a=1", "c=3"}
	if len(keys) != 2 || keys[0] != want[0] || keys[1] != want[1] {
		t.Fatalf("scan=%v want %v", keys, want)
	}
}

func TestGetCopies(t *testing.T) {
	m := New()
	m.Put([]byte("k"), []byte("v"))
	v, _ := m.Get([]byte("k"))
	v[0] = 'x'
	v2, _ := m.Get([]byte("k"))
	if string(v2) != "v" {
		t.Fatal("Get must copy")
	}
}

func TestConcurrentPuts(t *testing.T) {
	m := New()
	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			k := []byte(fmt.Sprintf("k%04d", i))
			m.Put(k, []byte("v"))
			if _, ok := m.Get(k); !ok {
				t.Errorf("missing %s", k)
			}
		}()
	}
	wg.Wait()
	if m.Len() != n {
		t.Fatalf("len=%d want %d", m.Len(), n)
	}
}
