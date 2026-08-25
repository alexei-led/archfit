package acquisition_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/evidence/acquisition"
)

func TestEffectiveConfigHash(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".archfit.yaml")
	if err := os.WriteFile(path, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	withCfg := acquisition.ConfigHash(path)
	if withCfg == "" {
		t.Error("acquisition.ConfigHash(existing) = \"\", want a hash")
	}

	if got := acquisition.ConfigHash(filepath.Join(dir, "absent.yaml")); got != "" {
		t.Errorf("acquisition.ConfigHash(absent) = %q, want \"\"", got)
	}

	if err := os.WriteFile(path, []byte("version: 1\nmodules: {}\n"), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if mutated := acquisition.ConfigHash(path); mutated == withCfg {
		t.Error("effectiveConfigHash did not change after config mutation")
	}
}

// TestConfigToolGate verifies the coverage-tool → config-key gate resolution:
// an unmapped tool and an empty gate both default to warn; a configured gate on
// the mapped key (e.g. tools.go.gate for go/packages) is surfaced verbatim.
