package raft

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

func init() {
	gob.Register(Command{})
	gob.Register(Entry{})
}

type persistent struct {
	mu    sync.Mutex
	path  string
	state hardState
}

type hardState struct {
	CurrentTerm uint64
	VotedFor    ID
	Log         []Entry
}

func openPersistent(dir string) (*persistent, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	p := &persistent{path: filepath.Join(dir, "state.gob")}
	f, err := os.Open(p.path)
	if err != nil {
		if os.IsNotExist(err) {
			p.state.Log = []Entry{{Term: 0, Index: 0}}
			return p, p.save()
		}
		return nil, err
	}
	defer f.Close()
	if err := gob.NewDecoder(f).Decode(&p.state); err != nil {
		return nil, fmt.Errorf("raft: persist decode: %w", err)
	}
	if len(p.state.Log) == 0 {
		p.state.Log = []Entry{{Term: 0, Index: 0}}
	}
	return p, nil
}

func (p *persistent) save() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	tmp := p.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := gob.NewEncoder(f)
	if err := enc.Encode(&p.state); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_ = os.Remove(p.path)
	return os.Rename(tmp, p.path)
}
