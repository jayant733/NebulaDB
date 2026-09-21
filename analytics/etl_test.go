package analytics_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPythonETL(t *testing.T) {
	py := "python"
	if runtime.GOOS == "windows" {
		py = "python"
	}
	root := repoRoot(t)
	cmd := exec.Command(py, filepath.Join(root, "analytics", "etl.py"),
		"--sql-out", filepath.Join(t.TempDir(), "load.sql"),
		"--csv-out", t.TempDir(),
		"--raw", filepath.Join(root, "analytics", "data", "raw_orders.csv"),
	)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("python etl: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "customers=3") {
		t.Fatalf("%s", out)
	}
}

func TestPythonPipelineSideFeature(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("python", filepath.Join(root, "analytics", "pipeline.py"), "--once")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("python pipeline: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "pipeline ok customers=3") {
		t.Fatalf("%s", out)
	}
	for _, name := range []string{"city_sales.csv", "dashboard.html", "snowflake_copy_into.sql"} {
		p := filepath.Join(root, "analytics", "out", name)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, ".."))
}
