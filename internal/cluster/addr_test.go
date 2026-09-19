package cluster

import "testing"

func TestOffsetAddr(t *testing.T) {
	got, err := OffsetAddr("127.0.0.1:7100", 2)
	if err != nil || got != "127.0.0.1:7102" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := OffsetAddr("no-port", 1); err == nil {
		t.Fatal("expected error")
	}
}
