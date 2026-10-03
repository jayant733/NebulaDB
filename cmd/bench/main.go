// Command bench measures sequential KV Set/Get on one local engine.
// Numbers are only valid for the machine and flags used. Print, do not invent.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/jayant/nebuladb/internal/storage"
)

func main() {
	n := flag.Int("n", 2000, "operations per phase")
	nosync := flag.Bool("no-sync", false, "skip WAL fsync")
	dir := flag.String("dir", "", "data dir (default: temp)")
	flag.Parse()

	sync := storage.SyncAlways
	if *nosync {
		sync = storage.SyncNone
	}
	data := *dir
	if data == "" {
		var err error
		data, err = os.MkdirTemp("", "nebuladb-bench-*")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer os.RemoveAll(data)
	}

	e, err := storage.Open(storage.Options{Dir: data, Sync: sync})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer e.Close()

	fmt.Printf("go=%s os=%s arch=%s n=%d sync=%v dir=%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH, *n, !*nosync, data)
	report("set", *n, func(i int) error {
		k := []byte(fmt.Sprintf("k%08d", i))
		return e.Set(k, []byte("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))
	})
	report("get", *n, func(i int) error {
		k := []byte(fmt.Sprintf("k%08d", i))
		_, _, err := e.Get(k)
		return err
	})
}

func report(op string, n int, fn func(int) error) {
	durs := make([]time.Duration, n)
	start := time.Now()
	for i := 0; i < n; i++ {
		t0 := time.Now()
		if err := fn(i); err != nil {
			fmt.Fprintf(os.Stderr, "%s #%d: %v\n", op, i, err)
			os.Exit(1)
		}
		durs[i] = time.Since(t0)
	}
	elapsed := time.Since(start)
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	p50 := durs[n/2]
	p99 := durs[(n*99)/100]
	qps := float64(n) / elapsed.Seconds()
	mean := elapsed / time.Duration(n)
	fmt.Printf("%s n=%d elapsed_ms=%d qps=%.0f mean_ns=%d p50_ns=%d p99_ns=%d\n",
		op, n, elapsed.Milliseconds(), qps, mean.Nanoseconds(), p50.Nanoseconds(), p99.Nanoseconds())
}
