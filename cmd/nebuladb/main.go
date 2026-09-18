// Command nebuladb is the Phase 1 single-node KV process (REPL).
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "nebuladb: storage engine not wired yet (Phase 1 in progress)")
	os.Exit(1)
}
