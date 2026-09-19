// Command nebulactl is the operator CLI for fault injection (Phase 9).
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jayant/nebuladb/internal/cluster"
)

func main() {
	if len(os.Args) < 3 {
		usage()
		os.Exit(2)
	}
	admin := os.Args[1]
	cmd := os.Args[2]
	var (
		none struct{}
		err  error
	)
	switch cmd {
	case "kill-node":
		err = cluster.AdminCall(admin, "Admin.Kill", struct{}{}, &none)
	case "stop":
		err = cluster.AdminCall(admin, "Admin.Stop", struct{}{}, &none)
	case "isolate":
		err = cluster.AdminCall(admin, "Admin.Isolate", struct{}{}, &none)
	case "heal":
		err = cluster.AdminCall(admin, "Admin.Heal", struct{}{}, &none)
	case "disk-fail":
		n := 1
		if len(os.Args) > 3 {
			n, err = strconv.Atoi(os.Args[3])
			if err != nil {
				fatal(err)
			}
		}
		err = cluster.AdminCall(admin, "Admin.DiskFail", n, &none)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fatal(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: nebulactl <admin-addr> kill-node|stop|isolate|heal|disk-fail [n]")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
