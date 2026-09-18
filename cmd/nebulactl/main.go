// Command nebulactl is the operator CLI. Cluster commands land in later phases.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "nebulactl: no cluster commands until Phase 6+")
	os.Exit(1)
}
