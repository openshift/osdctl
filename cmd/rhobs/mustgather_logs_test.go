package rhobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPodLogWriter(t *testing.T) {
	dir := t.TempDir()
	w := newPodLogWriter(dir)

	for _, tc := range []struct{ pod, msg string }{
		{"pod-a", "line1"},
		{"pod-a", "line2"},
		{"pod-b", "line3"},
		{"", "orphan"},
	} {
		if err := w.write(tc.pod, tc.msg); err != nil {
			t.Fatalf("write(%q): %v", tc.pod, err)
		}
	}
	w.close()

	assertFileContent(t, filepath.Join(dir, "pod-a.log"), "line1\nline2\n")
	assertFileContent(t, filepath.Join(dir, "pod-b.log"), "line3\n")
	// Empty pod names are routed to a stable placeholder rather than dropped.
	assertFileContent(t, filepath.Join(dir, "_unknown.log"), "orphan\n")
}

func TestPodLogWriter_NoWritesLeavesNoFiles(t *testing.T) {
	dir := t.TempDir()
	w := newPodLogWriter(filepath.Join(dir, "pods"))
	w.close()

	// The lazily-created directory must not exist when nothing was written.
	if _, err := os.Stat(filepath.Join(dir, "pods")); !os.IsNotExist(err) {
		t.Errorf("expected no pods dir to be created, stat err = %v", err)
	}
}

func TestHCPLogsExpr(t *testing.T) {
	logs := hcpLogsExpr("my-ns", "ext-123")
	if !strings.Contains(logs, `k8s_namespace_name="my-ns"`) ||
		!strings.Contains(logs, `json_kind != "Event"`) ||
		!strings.Contains(logs, `openshift_cluster_uid = "ext-123"`) {
		t.Errorf("unexpected logs expr: %s", logs)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s: got %q, want %q", path, string(got), want)
	}
}
