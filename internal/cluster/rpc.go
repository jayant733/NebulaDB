package cluster

import (
	"errors"
	"net"
	"net/rpc"
	"os"
	"time"

	"github.com/jayant/nebuladb/internal/metrics"
	"github.com/jayant/nebuladb/internal/raft"
	"github.com/jayant/nebuladb/internal/storage"
)

const (
	KVGet uint8 = 1
	KVSet uint8 = 2
	KVDel uint8 = 3
)

// KVArgs is a client KV RPC.
type KVArgs struct {
	Op    uint8
	Key   []byte
	Value []byte
}

// KVReply is a client KV result.
type KVReply struct {
	Value  []byte
	OK     bool
	Err    string
	Leader raft.ID
}

// KV is the net/rpc service for client reads/writes.
type KV struct {
	Host *Host
}

// Admin is the net/rpc service for nebulactl.
type Admin struct {
	Host *Host
}

// ServeKV registers KV and Admin on ln.
func (h *Host) ServeKV(ln net.Listener, peers map[raft.ID]string) error {
	if h.sem == nil {
		h.sem = make(chan struct{}, 32)
	}
	h.rpcPeers = peers
	srv := rpc.NewServer()
	if err := srv.RegisterName("KV", &KV{Host: h}); err != nil {
		return err
	}
	if err := srv.RegisterName("Admin", &Admin{Host: h}); err != nil {
		return err
	}
	h.rpcLn = ln
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.ServeConn(c)
		}
	}()
	return nil
}

// Exec runs one KV operation with backpressure and leader forward.
func (k *KV) Exec(args KVArgs, reply *KVReply) error {
	h := k.Host
	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
	default:
		reply.Err = "netkv: overloaded"
		return nil
	}
	start := time.Now()
	defer func() {
		metrics.Default.Inc("nebuladb_ops_total", "kv")
		metrics.Since("nebuladb_op_duration_seconds", "kv", start)
	}()
	k.exec(args, reply, 0)
	return nil
}

func (k *KV) exec(args KVArgs, reply *KVReply, hop int) {
	h := k.Host
	if hop > 3 {
		reply.Err = raft.ErrNotLeader.Error()
		return
	}
	var err error
	lid := k.leaderOf(args.Key)
	if lid != "" && lid != h.ID && h.rpcPeers[lid] != "" && hop == 0 {
		var fr KVReply
		if err := kvCall(h.rpcPeers[lid], args, &fr, 2*time.Second); err != nil {
			reply.Err = err.Error()
			reply.Leader = lid
			return
		}
		*reply = fr
		return
	}
	switch args.Op {
	case KVGet:
		var v []byte
		var ok bool
		v, ok, err = h.Router.Get(args.Key)
		if err == nil {
			reply.Value = v
			reply.OK = ok
			return
		}
	case KVSet:
		err = h.Router.Set(args.Key, args.Value)
	case KVDel:
		err = h.Router.Delete(args.Key)
	default:
		reply.Err = "netkv: bad op"
		return
	}
	if err == nil {
		return
	}
	if !errors.Is(err, raft.ErrNotLeader) {
		reply.Err = err.Error()
		return
	}
	lid = k.leaderOf(args.Key)
	reply.Leader = lid
	if lid == "" || lid == h.ID || h.rpcPeers[lid] == "" {
		reply.Err = raft.ErrNotLeader.Error()
		return
	}
	var fr KVReply
	if err := kvCall(h.rpcPeers[lid], args, &fr, 2*time.Second); err != nil {
		reply.Err = err.Error()
		reply.Leader = lid
		return
	}
	*reply = fr
}

func (k *KV) leaderOf(key []byte) raft.ID {
	h := k.Host
	if h.Router == nil {
		return ""
	}
	sid := h.Router.Owner(key)
	for _, r := range h.Replicas {
		if r.Shard == sid {
			return r.Node.LeaderID()
		}
	}
	if len(h.Replicas) == 1 {
		return h.Replicas[0].Node.LeaderID()
	}
	return ""
}

func kvCall(addr string, args KVArgs, reply *KVReply, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	c := rpc.NewClient(conn)
	defer c.Close()
	return c.Call("KV.Exec", args, reply)
}

// Client is a KV RPC client with retries.
type Client struct {
	Addrs   []string
	Timeout time.Duration
	Retries int
}

func (c *Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 2 * time.Second
}

func (c *Client) retries() int {
	if c.Retries > 0 {
		return c.Retries
	}
	return 8
}

func (c *Client) do(args KVArgs) (KVReply, error) {
	if len(c.Addrs) == 0 {
		return KVReply{}, errors.New("netkv: no addrs")
	}
	addr := c.Addrs[0]
	var last error
	for i := 0; i < c.retries(); i++ {
		var reply KVReply
		err := kvCall(addr, args, &reply, c.timeout())
		if err == nil && reply.Err == "" {
			return reply, nil
		}
		if err != nil {
			last = err
			addr = c.Addrs[(i+1)%len(c.Addrs)]
			time.Sleep(20 * time.Millisecond)
			continue
		}
		last = errors.New(reply.Err)
		addr = c.Addrs[(i+1)%len(c.Addrs)]
		time.Sleep(20 * time.Millisecond)
	}
	return KVReply{}, last
}

// Get implements a point read (may follow the leader).
func (c *Client) Get(key []byte) ([]byte, bool, error) {
	r, err := c.do(KVArgs{Op: KVGet, Key: key})
	return r.Value, r.OK, err
}

// Set implements storage.KV.
func (c *Client) Set(key, value []byte) error {
	_, err := c.do(KVArgs{Op: KVSet, Key: key, Value: value})
	return err
}

// Delete implements storage.KV.
func (c *Client) Delete(key []byte) error {
	_, err := c.do(KVArgs{Op: KVDel, Key: key})
	return err
}

// ScanPrefix is not served over RPC in Phase 10.
func (c *Client) ScanPrefix(prefix []byte, fn func(key, value []byte) bool) {}

// AdminCall is used by nebulactl.
func AdminCall(addr, method string, args, reply any) error {
	return adminCall(addr, method, args, reply)
}

func adminCall(addr, method string, args, reply any) error {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	c := rpc.NewClient(conn)
	defer c.Close()
	return c.Call(method, args, reply)
}

// Isolate drops outbound Raft RPCs from this process.
func (a *Admin) Isolate(_ struct{}, _ *struct{}) error {
	a.Host.Gate.DropAll(true)
	return nil
}

// Heal clears the partition gate.
func (a *Admin) Heal(_ struct{}, _ *struct{}) error {
	a.Host.Gate.Heal()
	return nil
}

// DiskFail injects n failed writes on every local engine.
func (a *Admin) DiskFail(n int, _ *struct{}) error {
	for _, r := range a.Host.Replicas {
		r.Local.InjectDiskErrors(n)
	}
	return nil
}

// Stop closes the host (Raft + listeners).
func (a *Admin) Stop(_ struct{}, _ *struct{}) error {
	a.Host.Close()
	return nil
}

// Kill exits the process after returning.
func (a *Admin) Kill(_ struct{}, _ *struct{}) error {
	go func() {
		time.Sleep(50 * time.Millisecond)
		os.Exit(1)
	}()
	return nil
}

var _ storage.KV = (*Client)(nil)
