package k8s

import (
	_ "embed"
	"strings"
	"testing"
)

//go:embed nebuladb.yaml
var manifest string

func TestStatefulSetContract(t *testing.T) {
	need := []string{
		"kind: StatefulSet",
		"clusterIP: None",
		"path: /livez",
		"path: /readyz",
		"volumeClaimTemplates",
		"mountPath: /data",
		"--serve",
		"terminationGracePeriodSeconds: 30",
	}
	for _, s := range need {
		if !strings.Contains(manifest, s) {
			t.Fatalf("manifest missing %q", s)
		}
	}
}
