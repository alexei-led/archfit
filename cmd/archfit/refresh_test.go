package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type refreshDiag struct {
	SchemaVersion string            `json:"schema_version"`
	FileFacts     []refreshFileFact `json:"file_facts"`
	Findings      []refreshFinding  `json:"findings"`
}

type refreshFileFact struct {
	Module string `json:"module"`
}

type refreshFinding struct {
	RuleID    string            `json:"rule_id"`
	Severity  string            `json:"severity"`
	Status    string            `json:"status"`
	MatchedBy map[string]string `json:"matched_by"`
	Edge      struct {
		Kind string `json:"kind"`
		From struct {
			Path   string `json:"path"`
			Module string `json:"module"`
		} `json:"from"`
		To struct {
			Path   string `json:"path"`
			Module string `json:"module"`
		} `json:"to"`
	} `json:"edge"`
	Locations []struct {
		File string `json:"file"`
		Line int    `json:"line"`
	} `json:"locations"`
}

type refreshFindingShape struct {
	RuleID        string
	Severity      string
	Status        string
	EdgeKind      string
	FromPath      string
	FromModule    string
	ToPath        string
	ToModule      string
	LocationCount int
	MatchedByKeys []string
}

func TestRun_Check_RefreshFlagAccepted(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)

	var stdout, stderr bytes.Buffer
	code := RunWithStderr([]string{cmdCheck, "--refresh", "-c", cfgPath}, &stdout, &stderr)
	if code != 0 && code != 1 {
		t.Fatalf("check --refresh: exit = %d, want 0 or 1 (flag parsed, no exit-3 parser error)\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
}

func TestRun_Check_NoCacheRejected(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)

	var stdout, stderr bytes.Buffer
	code := RunWithStderr([]string{cmdCheck, "--no-cache", "-c", cfgPath}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("check --no-cache: exit = %d, want 3\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--no-cache") {
		t.Errorf("stderr = %q, want the removed flag named in the parse error", stderr.String())
	}
}

func TestRun_Check_RefreshMatchesColdAndWarmJSON(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)

	cold := runRefreshCheckJSON(t, cfgPath)
	warm := runRefreshCheckJSON(t, cfgPath)
	refresh := runRefreshCheckJSON(t, cfgPath, "--refresh")

	if cold.SchemaVersion == "" {
		t.Fatal("cold run returned empty schema_version")
	}
	for name, got := range map[string]refreshDiag{"warm": warm, "refresh": refresh} {
		if got.SchemaVersion != cold.SchemaVersion {
			t.Errorf("%s schema_version = %q, want %q", name, got.SchemaVersion, cold.SchemaVersion)
		}
		if len(got.FileFacts) != len(cold.FileFacts) {
			t.Errorf("%s module fact count = %d, want %d", name, len(got.FileFacts), len(cold.FileFacts))
		}
		if !reflect.DeepEqual(refreshFindingShapes(got.Findings), refreshFindingShapes(cold.Findings)) {
			t.Errorf("%s findings shape changed across cold/warm/refresh runs", name)
		}
	}
}

func TestRun_Check_RefreshWritesToCache(t *testing.T) {
	t.Parallel()

	repoDir, err := os.MkdirTemp("", "archfit-refresh-cache-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(repoDir) })
	cfgPath := writeCacheableRepoAt(t, repoDir)

	runRefreshCheckJSON(t, cfgPath, "--refresh")

	cacheDir := filepath.Join(filepath.Dir(cfgPath), ".archfit-cache")
	files := regularFileCount(t, cacheDir)
	if files == 0 {
		t.Fatalf("--refresh wrote no cache files under %s", cacheDir)
	}
}

func TestRun_Check_BadRootWarnsOnStderr(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	badRoot := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := RunWithStderr([]string{cmdCheck, flagJSON, "-q", flagRoot, badRoot, "-c", cfgPath}, &stdout, &stderr)
	if code == 3 {
		t.Fatalf("check --root bad-dir: exit = 3\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "no source files matched") {
		t.Fatalf("stderr = %q, want \"no source files matched\" warning", stderr.String())
	}
}

func TestRun_Check_JSONStdoutStaysCleanWhenWarningsGoToStderr(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	badRoot := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := RunWithStderr([]string{cmdCheck, flagJSON, "-q", flagRoot, badRoot, "-c", cfgPath}, &stdout, &stderr)
	if code == 3 {
		t.Fatalf("check --json with warning: exit = 3\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "no source files matched") {
		t.Fatalf("stderr = %q, want warning on stderr", stderr.String())
	}
	if strings.Contains(stdout.String(), "warning:") || strings.Contains(stdout.String(), "no source files matched") {
		t.Fatalf("stdout polluted with warning text:\n%s", stdout.String())
	}

	var diag refreshDiag
	if err := json.Unmarshal(stdout.Bytes(), &diag); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if diag.SchemaVersion == "" {
		t.Fatalf("stdout JSON missing schema_version:\n%s", stdout.String())
	}
}

func runRefreshCheckJSON(t *testing.T, cfgPath string, extraArgs ...string) refreshDiag {
	t.Helper()
	args := append([]string{cmdCheck, flagJSON, "-c", cfgPath}, extraArgs...)

	var stdout, stderr bytes.Buffer
	code := RunWithStderr(args, &stdout, &stderr)
	if code != 0 && code != 1 {
		t.Fatalf("archfit check exited %d, want 0 or 1\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	var diag refreshDiag
	if err := json.Unmarshal(stdout.Bytes(), &diag); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	return diag
}

func refreshFindingShapes(findings []refreshFinding) []refreshFindingShape {
	out := make([]refreshFindingShape, 0, len(findings))
	for _, f := range findings {
		keys := make([]string, 0, len(f.MatchedBy))
		for key := range f.MatchedBy {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out = append(out, refreshFindingShape{
			RuleID:        f.RuleID,
			Severity:      f.Severity,
			Status:        f.Status,
			EdgeKind:      f.Edge.Kind,
			FromPath:      f.Edge.From.Path,
			FromModule:    f.Edge.From.Module,
			ToPath:        f.Edge.To.Path,
			ToModule:      f.Edge.To.Module,
			LocationCount: len(f.Locations),
			MatchedByKeys: keys,
		})
	}
	return out
}

func regularFileCount(t *testing.T, root string) int {
	t.Helper()

	count := 0
	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return count
}

func writeCacheableRepoAt(t *testing.T, dir string) string {
	t.Helper()
	files := map[string]string{
		markerGoMod: goModStub,
		filePkgAA: "package a\n\nimport \"example.com/test/pkg/b/impl\"\n\n" +
			"func Use() string { return impl.Secret() }\n",
		"pkg/b/impl/impl.go": implSource(),
		defaultConfigPath: `version: 1
modules:
  a:
    paths: ["pkg/a/**"]
  b:
    paths: ["pkg/b/**"]
`,
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitInitFixtureRepo(t, dir)
	return filepath.Join(dir, defaultConfigPath)
}
