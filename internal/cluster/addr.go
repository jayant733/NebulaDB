package cluster

import (
	"fmt"
	"net"
	"strconv"

	"github.com/jayant/nebuladb/internal/raft"
)

// OffsetAddr returns host:port+delta.
func OffsetAddr(addr string, delta int) (string, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("cluster: addr %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("cluster: port %q: %w", portStr, err)
	}
	p := port + delta
	if p <= 0 || p > 65535 {
		return "", fmt.Errorf("cluster: port %d out of range", p)
	}
	return net.JoinHostPort(host, strconv.Itoa(p)), nil
}

// OffsetAddrs copies addrs with each port increased by delta.
func OffsetAddrs(addrs map[raft.ID]string, delta int) (map[raft.ID]string, error) {
	out := make(map[raft.ID]string, len(addrs))
	for id, addr := range addrs {
		a, err := OffsetAddr(addr, delta)
		if err != nil {
			return nil, err
		}
		out[id] = a
	}
	return out, nil
}
