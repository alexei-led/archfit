package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/initcfg"
)

const (
	flagMigrationOnly = "--migration-only"
	cmdUpdate         = "update"
	subtestRefresh    = "refresh"
)

// readConfigFile reads a test-owned config path. The path always comes from
// t.TempDir(), so the gosec file-inclusion warning has no applicable threat.
func readConfigFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test-owned t.TempDir() path
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// legacyConfig is a v1 config carrying both retired coupling-gate knobs and
// authored content the migration must not disturb.
const legacyConfig = `# hand-authored — keep these comments
version: 1
coupling:
  min_severity: medium
  gate:
    min_band: serviceable
    max_drop: 5
modules:
  a:
    paths: ["pkg/a/**"]
    owner: team-a
`

// TestMigrationTargetsCurrentSchema pins the one duplicated constant in the
// migration path. initcfg is a support-layer YAML editor and may not import the
// config lifecycle, so it restates the schema version; cmd is a composition
// root and can see both, which makes this the only place the two can be
// compared. Drift here means the migration writes a version the binary rejects.
func TestMigrationTargetsCurrentSchema(t *testing.T) {
	if initcfg.TargetSchemaVersion != config.SchemaVersion {
		t.Errorf("initcfg.TargetSchemaVersion = %d, config.SchemaVersion = %d — the migration would write a schema the binary refuses to analyse",
			initcfg.TargetSchemaVersion, config.SchemaVersion)
	}
}

func writeLegacyConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), defaultConfigPath)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestConfigUpdateMigrationOnly_PreviewDoesNotWrite pins the preview contract:
// a preview reports the edits and leaves the file byte-identical, so a user can
// read what would change before anything changes.
func TestConfigUpdateMigrationOnly_PreviewDoesNotWrite(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{cmdConfig, cmdUpdate, flagMigrationOnly},
		{cmdConfig, cmdUpdate, flagMigrationOnly, flagJSON},
	} {
		t.Run(strings.Join(args[2:], " "), func(t *testing.T) {
			t.Parallel()
			path := writeLegacyConfig(t, legacyConfig)
			code, stdout, stderr := runArchfit(t, append(args, "-c", path)...)
			if code != 0 {
				t.Fatalf("exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
			}
			if after := readConfigFile(t, path); after != legacyConfig {
				t.Errorf("preview modified the config:\n%s", after)
			}
			if !strings.Contains(stdout, "distributed_monolith") {
				t.Errorf("preview did not report the replacement stanza:\n%s", stdout)
			}
		})
	}
}

// TestConfigUpdateMigrationOnly_JSONEnvelope pins the machine-readable report.
func TestConfigUpdateMigrationOnly_JSONEnvelope(t *testing.T) {
	t.Parallel()
	path := writeLegacyConfig(t, legacyConfig)

	code, stdout, stderr := runArchfit(t, cmdConfig, cmdUpdate, flagMigrationOnly, flagJSON, "-c", path)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr:\n%s", code, stderr)
	}
	var got struct {
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		FromVersion   int    `json:"from_version"`
		ToVersion     int    `json:"to_version"`
		Config        string `json:"config"`
		Changes       []struct {
			Key    string `json:"key"`
			Action string `json:"action"`
		} `json:"changes"`
		PolicyChanges []string `json:"policy_changes"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if got.SchemaVersion != initcfg.MigrationSchemaVersion {
		t.Errorf("schema_version = %q, want %q", got.SchemaVersion, initcfg.MigrationSchemaVersion)
	}
	if got.Status != initcfg.MigrationRequired {
		t.Errorf("status = %q, want %q", got.Status, initcfg.MigrationRequired)
	}
	if got.FromVersion != 1 || got.ToVersion != config.SchemaVersion {
		t.Errorf("versions = %d → %d, want 1 → %d", got.FromVersion, got.ToVersion, config.SchemaVersion)
	}
	if got.Config != path {
		t.Errorf("config = %q, want the file the report describes (%q)", got.Config, path)
	}
	if len(got.Changes) == 0 {
		t.Error("changes = [] on a config that needs migrating")
	}
	if len(got.PolicyChanges) == 0 {
		t.Error("policy_changes = [] while retiring a gate — a silent policy change is the failure mode")
	}
}

// TestConfigUpdateMigrationOnly_ApplyIsIdempotent pins the sweep contract: the
// first apply migrates, the second changes nothing at all, and the result loads
// cleanly through the analysis path that rejected the original.
func TestConfigUpdateMigrationOnly_ApplyIsIdempotent(t *testing.T) {
	t.Parallel()
	path := writeLegacyConfig(t, legacyConfig)

	if code, stdout, stderr := runArchfit(t, cmdConfig, cmdUpdate, flagMigrationOnly, "--apply", "-c", path); code != 0 {
		t.Fatalf("first apply: exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	first := readConfigFile(t, path)
	if first == legacyConfig {
		t.Fatal("apply did not write the migrated config")
	}
	if _, err := config.Load(t.Context(), path); err != nil {
		t.Errorf("migrated config does not load: %v\n%s", err, first)
	}

	if code, stdout, stderr := runArchfit(t, cmdConfig, cmdUpdate, flagMigrationOnly, "--apply", "-c", path); code != 0 {
		t.Fatalf("second apply: exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if second := readConfigFile(t, path); second != first {
		t.Errorf("second apply changed bytes:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestConfigUpdateMigrationOnly_RejectsAnalysisFlags pins the usage contract:
// the conflicting flags all mean "analyse the tree", which a one-file schema
// rewrite must never do, and the refusal is an exit-3 usage error.
func TestConfigUpdateMigrationOnly_RejectsAnalysisFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		extra []string
	}{
		{"ai-classify", []string{"--ai-classify"}},
		{subtestRefresh, []string{flagRefresh}},
		{"json with apply", []string{flagJSON, "--apply"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := writeLegacyConfig(t, legacyConfig)
			args := append([]string{cmdConfig, cmdUpdate, flagMigrationOnly, "-c", path}, tc.extra...)
			code, stdout, stderr := runArchfit(t, args...)
			if code != 3 {
				t.Errorf("exit = %d, want 3\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
			}
			if after := readConfigFile(t, path); after != legacyConfig {
				t.Errorf("a rejected invocation still wrote the config:\n%s", after)
			}
		})
	}
}

// TestConfigUpdateMigrationOnly_AlreadyCurrentIsANoOp pins the sweep-safe case:
// running the migration over an already-current config reports so and writes
// nothing.
func TestConfigUpdateMigrationOnly_AlreadyCurrentIsANoOp(t *testing.T) {
	t.Parallel()
	const current = "version: 2\nmodules:\n  a:\n    paths: [\"pkg/a/**\"]\n"
	path := writeLegacyConfig(t, current)

	code, stdout, stderr := runArchfit(t, cmdConfig, cmdUpdate, flagMigrationOnly, "--apply", "-c", path)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "already schema v2") {
		t.Errorf("stdout did not report the no-op:\n%s", stdout)
	}
	if after := readConfigFile(t, path); after != current {
		t.Errorf("a no-op migration rewrote the file:\n%s", after)
	}
}

// TestConfigUpdateMigrationOnly_RefusesAnUnversionedConfig pins the one shape
// that used to report success on a file the binary cannot load: no root
// `version:` key. MigrateToV2 only ever rewrote an EXISTING version line, so
// the result was "already current" and exit 0 — while `check` refused the same
// file with `version must be 2 (got 0)`. The only advertised escape from an
// unloadable config must not claim there is nothing to fix.
func TestConfigUpdateMigrationOnly_RefusesAnUnversionedConfig(t *testing.T) {
	t.Parallel()
	const unversioned = "modules:\n  a:\n    paths: [\"pkg/a/**\"]\n"

	t.Run("apply exits 3 and leaves the file alone", func(t *testing.T) {
		t.Parallel()
		path := writeLegacyConfig(t, unversioned)
		code, stdout, stderr := runArchfit(t, cmdConfig, cmdUpdate, flagMigrationOnly, "--apply", "-c", path)
		if code != 3 {
			t.Errorf("exit = %d, want 3\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		if strings.Contains(stdout+stderr, "already schema") {
			t.Errorf("an unversioned config was reported as current:\n%s%s", stdout, stderr)
		}
		if after := readConfigFile(t, path); after != unversioned {
			t.Errorf("a refused migration rewrote the config:\n%s", after)
		}
	})

	t.Run("preview names the missing key", func(t *testing.T) {
		t.Parallel()
		path := writeLegacyConfig(t, unversioned)
		code, stdout, stderr := runArchfit(t, cmdConfig, cmdUpdate, flagMigrationOnly, "-c", path)
		if code != 0 {
			t.Fatalf("preview exit = %d, want 0\nstderr:\n%s", code, stderr)
		}
		if !strings.Contains(stdout, "version:") || strings.Contains(stdout, "already schema") {
			t.Errorf("preview did not name the missing root version key:\n%s", stdout)
		}
	})

	t.Run("json reports the unversioned status", func(t *testing.T) {
		t.Parallel()
		path := writeLegacyConfig(t, unversioned)
		code, stdout, stderr := runArchfit(t, cmdConfig, cmdUpdate, flagMigrationOnly, "--json", "-c", path)
		if code != 0 {
			t.Fatalf("json exit = %d, want 0\nstderr:\n%s", code, stderr)
		}
		var got struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("decoding migration report: %v\n%s", err, stdout)
		}
		if got.Status != initcfg.MigrationUnversioned {
			t.Errorf("status = %q, want %q", got.Status, initcfg.MigrationUnversioned)
		}
	})
}

// TestConfigUpdateMigrationOnly_ApplyRefusesAnUnloadableResult pins the
// post-condition the write protocol enforces. MigrateToV2 is a line transform
// over YAML it never parses, so a shape it cannot see — here a flow-mapping
// gate whose retired keys survive the transform — would otherwise ship a file
// stamped v2 that the schema still rejects, and the only advertised escape from
// a v1 config would have made the file worse. Exit 3, original untouched.
func TestConfigUpdateMigrationOnly_ApplyRefusesAnUnloadableResult(t *testing.T) {
	t.Parallel()
	const flowGate = `version: 1
coupling:
  gate: {min_band: serviceable, max_drop: 5}
modules:
  a:
    paths: ["pkg/a/**"]
`
	path := writeLegacyConfig(t, flowGate)

	code, stdout, stderr := runArchfit(t, cmdConfig, cmdUpdate, flagMigrationOnly, "--apply", "-c", path)
	if code != 3 {
		t.Errorf("exit = %d, want 3\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if after := readConfigFile(t, path); after != flowGate {
		t.Errorf("a refused migration still rewrote the config:\n%s", after)
	}
}
