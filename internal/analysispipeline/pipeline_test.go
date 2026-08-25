package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/module"
)

func TestPolicySnapshotProjectsOwnershipAndDeployUnits(t *testing.T) {
	cfg := config.Config{Version: 1, Modules: map[string]module.ModuleDef{
		"cli": {Owner: "team-cli", DeployUnit: "archfit-cli", Paths: []string{"cmd/**"}},
	}}
	got := PolicySnapshot(cfg)
	if got.Ownership["cli"] != "team-cli" || got.DeployUnits["cli"] != "archfit-cli" {
		t.Fatalf("policy projection = %+v", got)
	}
}

func TestValidationCommandQuotesConfigAndRoot(t *testing.T) {
	got := ValidationCommand("/tmp/policy bundle/it's.yaml", "/tmp/repo root")
	want := "archfit check -c '/tmp/policy bundle/it'\"'\"'s.yaml' --root '/tmp/repo root'"
	if got != want {
		t.Fatalf("validationCommand = %q, want %q", got, want)
	}
}

func TestOnDiskWithinRejectsEscapesAndAcceptsLocalFile(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "pkg", "file.go")
	if err := ensureTestFile(local); err != nil {
		t.Fatal(err)
	}
	within := OnDiskWithin(root)
	for _, test := range []struct {
		path string
		want bool
	}{
		{path: "pkg/file.go", want: true},
		{path: "../outside.go", want: false},
		{path: filepath.Join(root, "pkg", "file.go"), want: false},
		{path: "/etc/passwd", want: false},
	} {
		path, want := test.path, test.want
		if got := within(path); got != want {
			t.Errorf("onDiskWithin(%q) = %t, want %t", path, got, want)
		}
	}
}

func TestBaseRunContextDoesNotMutateHead(t *testing.T) {
	head := RunContext{ConfigSource: "policy.yaml", BundleDir: "/bundle", ScanRoot: "/head", EvaluatedAt: time.Unix(1, 0)}
	base := baseRunContext(head, "/base")
	if base.ScanRoot != "/base" {
		t.Fatalf("base ScanRoot = %q, want /base", base.ScanRoot)
	}
	if head.ScanRoot != "/head" {
		t.Fatalf("head ScanRoot mutated to %q", head.ScanRoot)
	}
	if base.ConfigSource != head.ConfigSource || base.BundleDir != head.BundleDir || !base.EvaluatedAt.Equal(head.EvaluatedAt) {
		t.Fatalf("base context did not preserve shared inputs: base=%+v head=%+v", base, head)
	}
}

func ensureTestFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("package pkg\n"), 0o600)
}
