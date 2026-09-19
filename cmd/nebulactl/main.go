// Command nebulactl is the operator CLI. Cluster commands land in later phases.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "nebulactl: operator commands are Phase 9+; start a cluster with nebuladb --id --peers")
	os.Exit(1)
}
