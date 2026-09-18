// Command nebuladb is the single-node KV / indexed-row process (REPL).
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jayant/nebuladb/internal/index"
	"github.com/jayant/nebuladb/internal/storage"
)

var errQuit = errors.New("quit")

type repl struct {
	eng *storage.Engine
	idx *index.Store
}

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
	idx, err := index.Wrap(eng)
	if err != nil {
		fatalf("index catalog: %v", err)
	}
	r := &repl{eng: eng, idx: idx}

	fmt.Fprintf(os.Stderr, "nebuladb Phase 3 — LSM + indexes  data=%s  sync=%v\n", *data, !*nosync)
	fmt.Fprintln(os.Stderr, "commands: set/get/del | row/getrow | idxcreate/idxfind | flush | help | exit")

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
		if err := run(r, line); err != nil {
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

func run(r *repl, line string) error {
	cmd, rest, _ := strings.Cut(line, " ")
	cmd = strings.ToLower(cmd)
	rest = strings.TrimSpace(rest)
	eng := r.eng

	switch cmd {
	case "help", "?":
		fmt.Println("set <key> <value>           — raw KV put")
		fmt.Println("get <key>                   — raw KV read")
		fmt.Println("del <key>                   — raw KV tombstone")
		fmt.Println("row <pk> <f0> [f1 ...]      — put indexed tuple")
		fmt.Println("getrow <pk>                 — get tuple")
		fmt.Println("delrow <pk>                 — delete tuple + index entries")
		fmt.Println("idxcreate <name> <field>    — secondary index (0-based field)")
		fmt.Println("idxdrop <name>")
		fmt.Println("idxfind <name> <value>")
		fmt.Println("idxrange <name> <lo> <hi>")
		fmt.Println("idxlist")
		fmt.Println("scan | flush | compact | stats | exit")
		return nil
	case "exit", "quit":
		return errQuit
	case "flush":
		return eng.Flush()
	case "compact":
		return eng.Compact()
	case "stats":
		s := eng.Stats()
		fmt.Printf("dir=%s live_keys=%d approx_bytes=%d sstables=%d wal_bytes=%d bloom_checked=%d bloom_negative=%d indexes=%d\n",
			s.Dir, s.LiveKeys, s.ApproxSize, s.SSTables, s.WALBytes, s.BloomChecked, s.BloomNegatives, len(r.idx.Indexes()))
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
	case "row":
		parts := strings.Fields(rest)
		if len(parts) < 2 {
			return fmt.Errorf("usage: row <pk> <f0> [f1 ...]")
		}
		fields := make([][]byte, 0, len(parts)-1)
		for _, p := range parts[1:] {
			fields = append(fields, []byte(p))
		}
		return r.idx.PutRow([]byte(parts[0]), fields)
	case "getrow":
		if rest == "" {
			return fmt.Errorf("usage: getrow <pk>")
		}
		fields, ok, err := r.idx.GetRow([]byte(rest))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("(nil)")
			return nil
		}
		out := make([]string, len(fields))
		for i, f := range fields {
			out[i] = string(f)
		}
		fmt.Println(strings.Join(out, "\t"))
		return nil
	case "delrow":
		if rest == "" {
			return fmt.Errorf("usage: delrow <pk>")
		}
		return r.idx.DeleteRow([]byte(rest))
	case "idxcreate":
		name, fs, ok := strings.Cut(rest, " ")
		if !ok {
			return fmt.Errorf("usage: idxcreate <name> <field>")
		}
		field, err := strconv.Atoi(strings.TrimSpace(fs))
		if err != nil {
			return fmt.Errorf("usage: idxcreate <name> <field>")
		}
		return r.idx.CreateIndex(name, field)
	case "idxdrop":
		if rest == "" {
			return fmt.Errorf("usage: idxdrop <name>")
		}
		return r.idx.DropIndex(rest)
	case "idxfind":
		name, val, ok := strings.Cut(rest, " ")
		if !ok {
			return fmt.Errorf("usage: idxfind <name> <value>")
		}
		pks, err := r.idx.Find(name, []byte(strings.TrimSpace(val)))
		if err != nil {
			return err
		}
		printPKs(pks)
		return nil
	case "idxrange":
		parts := strings.Fields(rest)
		if len(parts) != 3 {
			return fmt.Errorf("usage: idxrange <name> <lo> <hi>")
		}
		pks, err := r.idx.RangeFind(parts[0], []byte(parts[1]), []byte(parts[2]))
		if err != nil {
			return err
		}
		printPKs(pks)
		return nil
	case "idxlist":
		for _, sp := range r.idx.Indexes() {
			fmt.Printf("%s\tfield=%d\n", sp.Name, sp.Field)
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q (try help)", cmd)
	}
}

func printPKs(pks [][]byte) {
	if len(pks) == 0 {
		fmt.Println("(none)")
		return
	}
	for _, pk := range pks {
		fmt.Println(string(pk))
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
