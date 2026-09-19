package raft

import (
	"net"
	"net/rpc"
	"time"
)

// RPC exposes Raft RPCs over net/rpc.
type RPC struct {
	N *Node
}

// RequestVote is the net/rpc method.
func (r *RPC) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) error {
	*reply = r.N.RequestVote(args)
	return nil
}

// AppendEntries is the net/rpc method.
func (r *RPC) AppendEntries(args AppendEntriesArgs, reply *AppendEntriesReply) error {
	*reply = r.N.AppendEntries(args)
	return nil
}

// TCP is a Transport over net/rpc.
type TCP struct {
	addrs   map[ID]string
	timeout time.Duration
}

// NewTCP maps node IDs to host:port.
func NewTCP(addrs map[ID]string) *TCP {
	return &TCP{addrs: addrs, timeout: 200 * time.Millisecond}
}

func (t *TCP) call(to ID, method string, args, reply any) error {
	addr, ok := t.addrs[to]
	if !ok {
		return errUnreachable
	}
	conn, err := net.DialTimeout("tcp", addr, t.timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	c := rpc.NewClient(conn)
	defer c.Close()
	return c.Call(method, args, reply)
}

// SendRequestVote implements Transport.
func (t *TCP) SendRequestVote(to ID, args RequestVoteArgs) (RequestVoteReply, error) {
	var reply RequestVoteReply
	err := t.call(to, "RaftRPC.RequestVote", args, &reply)
	return reply, err
}

// SendAppendEntries implements Transport.
func (t *TCP) SendAppendEntries(to ID, args AppendEntriesArgs) (AppendEntriesReply, error) {
	var reply AppendEntriesReply
	err := t.call(to, "RaftRPC.AppendEntries", args, &reply)
	return reply, err
}

// Serve accepts Raft RPCs on an existing listener.
func Serve(n *Node, ln net.Listener) error {
	srv := rpc.NewServer()
	if err := srv.RegisterName("RaftRPC", &RPC{N: n}); err != nil {
		return err
	}
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

// ListenAndServe starts a Raft RPC server on addr.
func ListenAndServe(n *Node, addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	if err := Serve(n, ln); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}
