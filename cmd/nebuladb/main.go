// Command nebuladb is the Phase 1 single-node KV process (REPL).
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jayant/nebuladb/internal/storage"
)

var errQuit = errors.New("quit")

func main() {
	data := flag.String("data", "./data", "data directory (WAL lives here)")
	nosync := flag.Bool("no-sync", false, "disable fsync (not crash-safe; for experiments)")
	flag.Parse()

	sync := storage.SyncAlways
	if *nosync {
		sync = storage.SyncNone
	}

	eng, err := storage.Open(storage.Options{Dir: *data, Sync: sync})
	if err != nil {
		fatalf("open: %v", err)
	}
	defer eng.Close()

	fmt.Fprintf(os.Stderr, "nebuladb Phase 2 — LSM KV  data=%s  sync=%v\n", *data, !*nosync)
	fmt.Fprintln(os.Stderr, "commands: set | get | del | scan | flush | compact | stats | help | exit")

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for {
		fmt.Fprint(os.Stdout, "nebula> ")
		if !sc.Scan() {
			break
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if err := run(eng, line); err != nil {
			if errors.Is(err, errQuit) {
				break
			}
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}
	if err := sc.Err(); err != nil {
		fatalf("stdin: %v", err)
	}
}

func run(eng *storage.Engine, line string) error {
	cmd, rest, _ := strings.Cut(line, " ")
	cmd = strings.ToLower(cmd)
	rest = strings.TrimSpace(rest)

	switch cmd {
	case "help", "?":
		fmt.Println("set <key> <value>  — durable put")
		fmt.Println("get <key>          — read")
		fmt.Println("del <key>          — tombstone")
		fmt.Println("scan               — ordered live keys")
		fmt.Println("flush              — MemTable → SSTable, rotate WAL")
		fmt.Println("compact            — merge SSTables, drop tombstones")
		fmt.Println("stats              — memtable / WAL / SST / Bloom counters")
		fmt.Println("exit               — close WAL and quit")
		return nil
	case "exit", "quit":
		return errQuit
	case "flush":
		return eng.Flush()
	case "compact":
		return eng.Compact()
	case "stats":
		s := eng.Stats()
		fmt.Printf("dir=%s live_keys=%d approx_bytes=%d sstables=%d wal_bytes=%d bloom_checked=%d bloom_negative=%d\n",
			s.Dir, s.LiveKeys, s.ApproxSize, s.SSTables, s.WALBytes, s.BloomChecked, s.BloomNegatives)
		return nil
	case "scan":
		n := 0
		eng.Scan(func(k, v []byte) bool {
			fmt.Printf("%s\t%s\n", k, v)
			n++
			return true
		})
		fmt.Printf("(%d keys)\n", n)
		return nil
	case "get":
		if rest == "" {
			return fmt.Errorf("usage: get <key>")
		}
		v, ok, err := eng.Get([]byte(rest))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("(nil)")
			return nil
		}
		fmt.Println(string(v))
		return nil
	case "del", "delete":
		if rest == "" {
			return fmt.Errorf("usage: del <key>")
		}
		return eng.Delete([]byte(rest))
	case "set":
		key, val, ok := strings.Cut(rest, " ")
		if !ok || key == "" {
			return fmt.Errorf("usage: set <key> <value>")
		}
		return eng.Set([]byte(key), []byte(strings.TrimSpace(val)))
	default:
		return fmt.Errorf("unknown command %q (try help)", cmd)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
