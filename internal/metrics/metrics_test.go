package metrics

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWritePrometheus(t *testing.T) {
	r := New()
	r.Inc("nebuladb_wal_fsyncs_total", "")
	r.Inc("nebuladb_ops_total", "set")
	r.Observe("nebuladb_op_duration_seconds", "set", 2*time.Millisecond)
	var b strings.Builder
	r.WritePrometheus(&b)
	s := b.String()
	for _, want := range []string{
		"# TYPE nebuladb_wal_fsyncs_total counter",
		"nebuladb_wal_fsyncs_total 1",
		`nebuladb_ops_total{op="set"} 1`,
		"# TYPE nebuladb_op_duration_seconds histogram",
		`nebuladb_op_duration_seconds_bucket{op="set",le="0.005"}`,
		"nebuladb_op_duration_seconds_count",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in\n%s", want, s)
		}
	}
}

func TestHandler(t *testing.T) {
	r := New()
	r.Inc("nebuladb_compactions_total", "")
	ts := httptest.NewServer(Handler(r))
	defer ts.Close()
	resp, err := ts.Client().Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "nebuladb_compactions_total 1") {
		t.Fatalf("%s", body)
	}
}
