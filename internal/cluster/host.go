package cluster

import (
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/jayant/nebuladb/internal/raft"
	"github.com/jayant/nebuladb/internal/shard"
	"github.com/jayant/nebuladb/internal/storage"
)

// Replica is this process's copy of one shard (engine + Raft).
type Replica struct {
	Shard shard.ID
	Local *storage.Engine
	Node  *raft.Node
	KV    *Replicated
	ln    net.Listener
}

// Host is one OS process: a replica of every shard plus a local router.
type Host struct {
	ID       raft.ID
	Replicas []*Replica
	Router   *shard.Router
	Gate     *raft.Gate
	rpcLn    net.Listener
	rpcPeers map[raft.ID]string
	sem      chan struct{}
}

// HostConfig starts N Raft groups on this process.
type HostConfig struct {
	Dir         string
	Shards      int
	ID          raft.ID
	Peers       []raft.ID
	Addrs       map[raft.ID]string // base host:port per node; required for TCP
	Sync        storage.SyncMode
	Transports  []raft.Transport // per shard; nil means TCP with OffsetAddrs
	Heartbeat   time.Duration
	ElectionMin time.Duration
	ElectionMax time.Duration
	Gate        *raft.Gate // optional; created if TCP and nil
}

// StartHost opens engines and Raft nodes for each shard.
func StartHost(cfg HostConfig) (*Host, error) {
	if cfg.Shards < 2 {
		return nil, fmt.Errorf("cluster: need at least 2 shards")
	}
	if cfg.ID == "" || len(cfg.Peers) == 0 {
		return nil, fmt.Errorf("cluster: id and peers required")
	}
	if cfg.Transports != nil && len(cfg.Transports) != cfg.Shards {
		return nil, fmt.Errorf("cluster: want %d transports", cfg.Shards)
	}
	if cfg.Transports == nil {
		if cfg.Addrs[cfg.ID] == "" {
			return nil, fmt.Errorf("cluster: missing listen addr for %s", cfg.ID)
		}
	}

	sids := shard.NumericIDs(cfg.Shards)
	h := &Host{ID: cfg.ID, Gate: cfg.Gate, sem: make(chan struct{}, 32)}
	if h.Gate == nil {
		h.Gate = raft.NewGate()
	}
	cfg.Gate = h.Gate
	stores := make(map[shard.ID]storage.KV, cfg.Shards)
	for i, sid := range sids {
		rep, err := startReplica(cfg, i, sid)
		if err != nil {
			h.Close()
			return nil, err
		}
		h.Replicas = append(h.Replicas, rep)
		stores[sid] = rep.KV
	}
	ring, err := shard.NewRing(sids, 0)
	if err != nil {
		h.Close()
		return nil, err
	}
	rt, err := shard.NewRouter(ring, stores)
	if err != nil {
		h.Close()
		return nil, err
	}
	h.Router = rt
	if cfg.Transports == nil {
		rpcAddr, err := OffsetAddr(cfg.Addrs[cfg.ID], cfg.Shards)
		if err != nil {
			h.Close()
			return nil, err
		}
		peers, err := OffsetAddrs(cfg.Addrs, cfg.Shards)
		if err != nil {
			h.Close()
			return nil, err
		}
		ln, err := net.Listen("tcp", rpcAddr)
		if err != nil {
			h.Close()
			return nil, err
		}
		if err := h.ServeKV(ln, peers); err != nil {
			_ = ln.Close()
			h.Close()
			return nil, err
		}
	}
	return h, nil
}

func startReplica(cfg HostConfig, i int, sid shard.ID) (*Replica, error) {
	dir := filepath.Join(cfg.Dir, "shard-"+string(sid))
	eng, err := storage.Open(storage.Options{Dir: dir, Sync: cfg.Sync})
	if err != nil {
		return nil, err
	}
	var trans raft.Transport
	if cfg.Transports != nil {
		trans = cfg.Transports[i]
	} else {
		addrs, err := OffsetAddrs(cfg.Addrs, i)
		if err != nil {
			_ = eng.Close()
			return nil, err
		}
		trans = raft.NewTCP(addrs).WithGate(cfg.ID, cfg.Gate)
	}
	node, err := raft.Start(raft.Config{
		ID:          cfg.ID,
		Peers:       cfg.Peers,
		Dir:         filepath.Join(dir, "raft"),
		Transport:   trans,
		Apply:       Apply(eng),
		Heartbeat:   cfg.Heartbeat,
		ElectionMin: cfg.ElectionMin,
		ElectionMax: cfg.ElectionMax,
	})
	if err != nil {
		_ = eng.Close()
		return nil, err
	}
	if m, ok := trans.(*raft.Memory); ok {
		m.Register(node)
	}
	rep := &Replica{
		Shard: sid,
		Local: eng,
		Node:  node,
		KV:    &Replicated{Node: node, Local: eng},
	}
	if cfg.Transports == nil {
		addr, err := OffsetAddr(cfg.Addrs[cfg.ID], i)
		if err != nil {
			node.Stop()
			_ = eng.Close()
			return nil, err
		}
		ln, err := raft.ListenAndServe(node, addr)
		if err != nil {
			node.Stop()
			_ = eng.Close()
			return nil, err
		}
		rep.ln = ln
	}
	return rep, nil
}

// Close stops Raft, listeners, and engines.
func (h *Host) Close() {
	if h == nil {
		return
	}
	if h.rpcLn != nil {
		_ = h.rpcLn.Close()
		h.rpcLn = nil
	}
	for _, r := range h.Replicas {
		if r == nil {
			continue
		}
		if r.Node != nil {
			r.Node.Stop()
		}
		if r.ln != nil {
			_ = r.ln.Close()
		}
		if r.Local != nil {
			_ = r.Local.Close()
		}
	}
}

// LeaderRouter builds a KV router that always writes to the current shard leader.
func LeaderRouter(hosts []*Host) (*shard.Router, error) {
	if len(hosts) == 0 || hosts[0] == nil || len(hosts[0].Replicas) == 0 {
		return nil, fmt.Errorf("cluster: no hosts")
	}
	n := len(hosts[0].Replicas)
	sids := shard.NumericIDs(n)
	stores := make(map[shard.ID]storage.KV, n)
	for i, sid := range sids {
		members := make([]*Replicated, 0, len(hosts))
		for _, h := range hosts {
			if i >= len(h.Replicas) {
				return nil, fmt.Errorf("cluster: host shard mismatch")
			}
			members = append(members, h.Replicas[i].KV)
		}
		stores[sid] = &LeaderPick{Members: members}
	}
	ring, err := shard.NewRing(sids, 0)
	if err != nil {
		return nil, err
	}
	return shard.NewRouter(ring, stores)
}
