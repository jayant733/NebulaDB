package cluster

import "testing"

func TestParsePeers(t *testing.T) {
	m, ids, err := ParsePeers("n1=127.0.0.1:7001,n2=127.0.0.1:7002")
	if err != nil || len(ids) != 2 || m["n1"] != "127.0.0.1:7001" {
		t.Fatalf("%v %v %v", m, ids, err)
	}
}
