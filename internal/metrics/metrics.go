// Package metrics is an in-process Prometheus text exposition.
package metrics

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// Default is the process-wide registry.
	Default = New()
	buckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 1, 5}
)

// Registry holds counters, gauges, and histograms.
type Registry struct {
	mu    sync.Mutex
	ctr   map[string]*atomic.Uint64
	gauge map[string]*atomic.Uint64 // float bits
	hist  map[string]*histogram
}

type histogram struct {
	counts []atomic.Uint64
	sum    atomic.Uint64 // microseconds
	n      atomic.Uint64
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{
		ctr:   map[string]*atomic.Uint64{},
		gauge: map[string]*atomic.Uint64{},
		hist:  map[string]*histogram{},
	}
}

func labelsKey(name, op string) string {
	if op == "" {
		return name
	}
	return name + `{op="` + op + `"}`
}

// Inc adds 1 to a counter (optional op label).
func (r *Registry) Inc(name, op string) {
	k := labelsKey(name, op)
	r.mu.Lock()
	c, ok := r.ctr[k]
	if !ok {
		c = &atomic.Uint64{}
		r.ctr[k] = c
	}
	r.mu.Unlock()
	c.Add(1)
}

// Add adds n to a counter.
func (r *Registry) Add(name, op string, n uint64) {
	k := labelsKey(name, op)
	r.mu.Lock()
	c, ok := r.ctr[k]
	if !ok {
		c = &atomic.Uint64{}
		r.ctr[k] = c
	}
	r.mu.Unlock()
	c.Add(n)
}

// Observe records a duration histogram for op.
func (r *Registry) Observe(name, op string, d time.Duration) {
	k := labelsKey(name, op)
	r.mu.Lock()
	h, ok := r.hist[k]
	if !ok {
		h = &histogram{counts: make([]atomic.Uint64, len(buckets)+1)}
		r.hist[k] = h
	}
	r.mu.Unlock()
	sec := d.Seconds()
	idx := len(buckets)
	for i, b := range buckets {
		if sec <= b {
			idx = i
			break
		}
	}
	h.counts[idx].Add(1)
	h.n.Add(1)
	us := d.Microseconds()
	if us < 0 {
		us = 0
	}
	h.sum.Add(uint64(us))
}

// WritePrometheus writes exposition format to w.
func (r *Registry) WritePrometheus(w io.Writer) {
	r.mu.Lock()
	type pair struct{ k, line string }
	var ctrs []pair
	for k, c := range r.ctr {
		base := k
		if i := strings.IndexByte(k, '{'); i >= 0 {
			base = k[:i]
		}
		ctrs = append(ctrs, pair{k, fmt.Sprintf("# TYPE %s counter\n%s %d\n", base, k, c.Load())})
	}
	var hists []pair
	for k, h := range r.hist {
		base := k
		lbl := ""
		if i := strings.IndexByte(k, '{'); i >= 0 {
			base = k[:i]
			lbl = k[i:] // {op="set"}
		}
		var b strings.Builder
		b.WriteString("# TYPE " + base + " histogram\n")
		var cum uint64
		for i, bound := range buckets {
			cum += h.counts[i].Load()
			le := fmt.Sprintf("%g", bound)
			b.WriteString(histBucket(base, lbl, le, cum))
		}
		cum += h.counts[len(buckets)].Load()
		b.WriteString(histBucket(base, lbl, "+Inf", cum))
		sum := float64(h.sum.Load()) / 1e6
		b.WriteString(fmt.Sprintf("%s_sum%s %g\n%s_count%s %d\n", base, lbl, sum, base, lbl, h.n.Load()))
		hists = append(hists, pair{k, b.String()})
	}
	r.mu.Unlock()
	sort.Slice(ctrs, func(i, j int) bool { return ctrs[i].k < ctrs[j].k })
	sort.Slice(hists, func(i, j int) bool { return hists[i].k < hists[j].k })
	for _, p := range ctrs {
		_, _ = io.WriteString(w, p.line)
	}
	for _, p := range hists {
		_, _ = io.WriteString(w, p.line)
	}
}

func histBucket(base, lbl, le string, cum uint64) string {
	inner := `le="` + le + `"`
	if lbl != "" {
		// {op="set"} -> {op="set",le="0.001"}
		inner = strings.TrimSuffix(strings.TrimPrefix(lbl, "{"), "}") + "," + inner
	}
	return fmt.Sprintf("%s_bucket{%s} %d\n", base, inner, cum)
}

// Handler serves GET /metrics.
func Handler(r *Registry) http.Handler {
	if r == nil {
		r = Default
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/metrics" && req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		r.WritePrometheus(w)
	})
}

// Listen serves /metrics on addr. Returns the listener.
func Listen(addr string, r *Registry) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", Handler(r))
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	return ln, nil
}

// Since observes time since t on Default.
func Since(name, op string, t time.Time) {
	Default.Observe(name, op, time.Since(t))
}
