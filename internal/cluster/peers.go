package cluster

import (
	"fmt"
	"strings"

	"github.com/jayant/nebuladb/internal/raft"
)

// ParsePeers parses "id=host:port,id=host:port".
func ParsePeers(s string) (map[raft.ID]string, []raft.ID, error) {
	out := map[raft.ID]string{}
	var ids []raft.ID
	if strings.TrimSpace(s) == "" {
		return out, ids, nil
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		id, addr, ok := strings.Cut(part, "=")
		if !ok || id == "" || addr == "" {
			return nil, nil, fmt.Errorf("cluster: bad peer %q (want id=host:port)", part)
		}
		rid := raft.ID(id)
		out[rid] = addr
		ids = append(ids, rid)
	}
	return out, ids, nil
}
