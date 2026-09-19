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

	"github.com/jayant/nebuladb/internal/cluster"
	"github.com/jayant/nebuladb/internal/index"
	"github.com/jayant/nebuladb/internal/raft"
	"github.com/jayant/nebuladb/internal/shard"
	"github.com/jayant/nebuladb/internal/sql/engine"
	"github.com/jayant/nebuladb/internal/storage"
)

var errQuit = errors.New("quit")

type repl struct {
	eng   *storage.Engine
	engs  []*storage.Engine
	kv    storage.KV
	idx   *index.Store
	sql   *engine.Engine
	node  *raft.Node
	nodes []*raft.Node
	id    string
}

func main() {
	data := flag.String("data", "./data", "data directory (WAL lives here)")
	nosync := flag.Bool("no-sync", false, "disable fsync (not crash-safe; for experiments)")
	id := flag.String("id", "", "raft node id (cluster mode)")
	peers := flag.String("peers", "", "raft peers id=host:port,...")
	nshards := flag.Int("shards", 1, "shard count (with --peers, one Raft group each)")
	flag.Parse()

	if *nshards < 1 {
		fatalf("--shards must be >= 1")
	}

	sync := storage.SyncAlways
	if *nosync {
		sync = storage.SyncNone
	}

	var (
		eng   *storage.Engine
		engs  []*storage.Engine
		kv    storage.KV
		node  *raft.Node
		nodes []*raft.Node
		err   error
	)
	peersStr := strings.TrimSpace(*peers)
	if *nshards > 1 && peersStr != "" {
		if *id == "" {
			fatalf("--id is required with --peers")
		}
		addrs, ids, err := cluster.ParsePeers(peersStr)
		if err != nil {
			fatalf("%v", err)
		}
		h, err := cluster.StartHost(cluster.HostConfig{
			Dir:    *data,
			Shards: *nshards,
			ID:     raft.ID(*id),
			Peers:  ids,
			Addrs:  addrs,
			Sync:   sync,
		})
		if err != nil {
			fatalf("host: %v", err)
		}
		defer h.Close()
		for _, rep := range h.Replicas {
			engs = append(engs, rep.Local)
			nodes = append(nodes, rep.Node)
		}
		kv = h.Router
		eng = engs[0]
		fmt.Fprintf(os.Stderr, "raft-per-shard id=%s shards=%d\n", *id, *nshards)
	} else if *nshards > 1 {
		var rt *shard.Router
		rt, engs, err = shard.OpenLocal(*data, *nshards, sync)
		if err != nil {
			fatalf("shards: %v", err)
		}
		defer func() {
			for _, e := range engs {
				_ = e.Close()
			}
		}()
		kv = rt
		eng = engs[0]
	} else {
		eng, err = storage.Open(storage.Options{Dir: *data, Sync: sync})
		if err != nil {
			fatalf("open: %v", err)
		}
		defer eng.Close()
		engs = []*storage.Engine{eng}
		kv = eng
		if strings.TrimSpace(*peers) != "" {
			if *id == "" {
				fatalf("--id is required with --peers")
			}
			addrs, ids, err := cluster.ParsePeers(*peers)
			if err != nil {
				fatalf("%v", err)
			}
			addr, ok := addrs[raft.ID(*id)]
			if !ok {
				fatalf("peers must include this --id")
			}
			trans := raft.NewTCP(addrs)
			node, err = raft.Start(raft.Config{
				ID:        raft.ID(*id),
				Peers:     ids,
				Dir:       *data + "/raft",
				Transport: trans,
				Apply:     cluster.Apply(eng),
			})
			if err != nil {
				fatalf("raft: %v", err)
			}
			defer node.Stop()
			ln, err := raft.ListenAndServe(node, addr)
			if err != nil {
				fatalf("raft listen: %v", err)
			}
			defer ln.Close()
			kv = &cluster.Replicated{Node: node, Local: eng}
			fmt.Fprintf(os.Stderr, "raft listening on %s id=%s\n", addr, *id)
		}
	}

	idx, err := index.Wrap(kv)
	if err != nil {
		fatalf("index catalog: %v", err)
	}
	sqleng, err := engine.New(kv)
	if err != nil {
		fatalf("sql: %v", err)
	}
	r := &repl{eng: eng, engs: engs, kv: kv, idx: idx, sql: sqleng, node: node, nodes: nodes, id: *id}

	fmt.Fprintf(os.Stderr, "nebuladb Phase 8 — shards=%d  data=%s  sync=%v\n", *nshards, *data, !*nosync)
	fmt.Fprintln(os.Stderr, "SQL: CREATE/INSERT/SELECT/UPDATE/DELETE | BEGIN/COMMIT/ROLLBACK")
	fmt.Fprintln(os.Stderr, "KV:  set | get | del | flush | help | exit")

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

	switch cmd {
	case "help", "?":
		fmt.Println("SQL:")
		fmt.Println("  CREATE TABLE t (id INT, name TEXT, age INT);")
		fmt.Println("  INSERT INTO t VALUES (1, 'Jayant', 20);")
		fmt.Println("  SELECT * FROM t WHERE age >= 18 ORDER BY name LIMIT 10;")
		fmt.Println("  UPDATE t SET age = 21 WHERE id = 1;")
		fmt.Println("  DELETE FROM t WHERE id = 1;")
		fmt.Println("  BEGIN;  COMMIT;  ROLLBACK;  BEGIN REPEATABLE READ;")
		fmt.Println("KV: set/get/del | row/idxcreate | flush | compact | stats | exit")
		return nil
	case "exit", "quit":
		return errQuit
	case "flush":
		return eachEngine(r, func(e *storage.Engine) error { return e.Flush() })
	case "compact":
		return eachEngine(r, func(e *storage.Engine) error { return e.Compact() })
	case "stats":
		var (
			live        int
			approx, wal int64
			sst         int
			bc, bn      uint64
		)
		dir := ""
		for _, e := range r.engs {
			s := e.Stats()
			if dir == "" {
				dir = s.Dir
			}
			live += s.LiveKeys
			approx += s.ApproxSize
			sst += s.SSTables
			wal += s.WALBytes
			bc += s.BloomChecked
			bn += s.BloomNegatives
		}
		fmt.Printf("dir=%s shards=%d live_keys=%d approx_bytes=%d sstables=%d wal_bytes=%d bloom_checked=%d bloom_negative=%d indexes=%d\n",
			dir, len(r.engs), live, approx, sst, wal, bc, bn, len(r.idx.Indexes()))
		if r.node != nil {
			fmt.Printf("raft id=%s leader=%s is_leader=%v\n", r.id, r.node.LeaderID(), r.node.IsLeader())
		}
		for i, n := range r.nodes {
			fmt.Printf("shard %d raft id=%s leader=%s is_leader=%v\n", i, r.id, n.LeaderID(), n.IsLeader())
		}
		return nil
	case "scan":
		n := 0
		r.kv.ScanPrefix(nil, func(k, v []byte) bool {
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
		v, ok, err := r.kv.Get([]byte(rest))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("(nil)")
			return nil
		}
		fmt.Println(string(v))
		return nil
	case "create", "insert", "select", "update", "begin", "commit", "rollback":
		return runSQL(r, line)
	case "del", "delete":
		if cmd == "delete" && strings.HasPrefix(strings.ToLower(rest), "from") {
			return runSQL(r, line)
		}
		if rest == "" {
			return fmt.Errorf("usage: del <key>")
		}
		return r.kv.Delete([]byte(rest))
	case "set":
		key, val, ok := strings.Cut(rest, " ")
		if !ok || key == "" {
			return fmt.Errorf("usage: set <key> <value>")
		}
		return r.kv.Set([]byte(key), []byte(strings.TrimSpace(val)))
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

func runSQL(r *repl, line string) error {
	res, err := r.sql.Exec(line)
	if err != nil {
		return err
	}
	printSQL(res)
	return nil
}

func printSQL(res *engine.Result) {
	if len(res.Columns) > 0 {
		fmt.Println(strings.Join(res.Columns, "\t"))
		for _, row := range res.Rows {
			fmt.Println(strings.Join(row, "\t"))
		}
		fmt.Printf("(%d rows)\n", len(res.Rows))
		return
	}
	if res.Message != "" {
		fmt.Println(res.Message)
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

func eachEngine(r *repl, fn func(*storage.Engine) error) error {
	for _, e := range r.engs {
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
