package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRunContext pins the run-context value table: which field feeds the
// config hash, the config bundle (labels + fact cache), the analysis
// boundary, and the evaluation instant, for every run shape the pipeline
// serves. Before the context existed one config path stood in for all four,
// so a comparison run could not measure a candidate config while reading
// labels and cached facts from the current bundle.
func TestRunContext(t *testing.T) {
	t.Parallel()

	const (
		repoRoot     = "/repo"
		currentCfg   = repoRoot + "/.archfit.yaml"
		candidateCfg = "/tmp/candidate.archfit.yaml"
		subtree      = repoRoot + "/services/api"
		baseWorktree = repoRoot + "/.archfit-cache/worktrees/abc123/wt"
		nestedCfg    = repoRoot + "/ci/policy/.archfit.yaml"
		nestedDir    = repoRoot + "/ci/policy"
	)
	evaluated := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)

	t.Run("value table", func(t *testing.T) {
		t.Parallel()
		// One row per run shape in the plan's required value table.
		cases := []struct {
			name             string
			rc               runContext
			wantConfigSource string
			wantBundleDir    string
			wantScanRoot     string
		}{
			{
				name:             "normal analysis",
				rc:               newRunContext(currentCfg, ""),
				wantConfigSource: currentCfg,
				wantBundleDir:    repoRoot,
				wantScanRoot:     "", // empty: scope.Resolve falls through to the git root
			},
			{
				name:             "normal analysis with --root subtree",
				rc:               newRunContext(currentCfg, subtree),
				wantConfigSource: currentCfg,
				wantBundleDir:    repoRoot,
				wantScanRoot:     subtree,
			},
			{
				name:             "config in a nested directory",
				rc:               newRunContext(nestedCfg, ""),
				wantConfigSource: nestedCfg,
				wantBundleDir:    nestedDir,
				wantScanRoot:     "",
			},
			{
				name:             "git base",
				rc:               baseRunContext(newRunContext(currentCfg, ""), baseWorktree),
				wantConfigSource: currentCfg, // head config: isolates code drift from config drift
				wantBundleDir:    repoRoot,   // head bundle: labels and facts stay current-side
				wantScanRoot:     baseWorktree,
			},
			{
				name:             "compare current",
				rc:               newRunContext(currentCfg, subtree),
				wantConfigSource: currentCfg,
				wantBundleDir:    repoRoot,
				wantScanRoot:     subtree,
			},
			{
				name: "compare candidate",
				rc: runContext{
					ConfigSource: candidateCfg,
					BundleDir:    filepath.Dir(currentCfg),
					ScanRoot:     subtree,
				},
				wantConfigSource: candidateCfg, // candidate bytes are hashed
				wantBundleDir:    repoRoot,     // labels and fact cache stay current-side
				wantScanRoot:     subtree,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				if got := tc.rc.ConfigSource; got != tc.wantConfigSource {
					t.Errorf("ConfigSource = %q, want %q", got, tc.wantConfigSource)
				}
				if got := tc.rc.BundleDir; got != tc.wantBundleDir {
					t.Errorf("BundleDir = %q, want %q", got, tc.wantBundleDir)
				}
				if got := tc.rc.ScanRoot; got != tc.wantScanRoot {
					t.Errorf("ScanRoot = %q, want %q", got, tc.wantScanRoot)
				}
			})
		}
	})

	t.Run("scan root stays empty without --root", func(t *testing.T) {
		t.Parallel()
		// Regression guard: defaulting ScanRoot to BundleDir would move the
		// analysis boundary from the git root to the config directory AND
		// stamp a spurious --root on every emitted repair command.
		rc := newRunContext(currentCfg, "")
		if rc.ScanRoot != "" {
			t.Fatalf("ScanRoot = %q, want empty", rc.ScanRoot)
		}
		if got, want := rc.scanDir(), repoRoot; got != want {
			t.Errorf("scanDir() = %q, want %q (bundle dir anchors git resolution)", got, want)
		}
		if got, want := validationCommand(rc.ConfigSource, rc.ScanRoot), "archfit check -c "+currentCfg; got != want {
			t.Errorf("validationCommand = %q, want %q", got, want)
		}

		withRoot := newRunContext(currentCfg, subtree)
		if got := withRoot.scanDir(); got != subtree {
			t.Errorf("scanDir() = %q, want %q (--root wins)", got, subtree)
		}
		if got, want := validationCommand(withRoot.ConfigSource, withRoot.ScanRoot), "archfit check -c "+currentCfg+" --root "+subtree; got != want {
			t.Errorf("validationCommand = %q, want %q", got, want)
		}
	})

	t.Run("evaluated at", func(t *testing.T) {
		t.Parallel()
		// A normal single run leaves the field zero and samples the clock.
		before := time.Now()
		sampled := newRunContext(currentCfg, "").evaluatedAt()
		if sampled.Before(before) {
			t.Errorf("zero EvaluatedAt sampled %v, want >= %v", sampled, before)
		}
		// An explicit instant passes straight through.
		rc := newRunContext(currentCfg, "")
		rc.EvaluatedAt = evaluated
		if got := rc.evaluatedAt(); !got.Equal(evaluated) {
			t.Errorf("evaluatedAt() = %v, want %v", got, evaluated)
		}
	})

	t.Run("git base shares the head evaluation time", func(t *testing.T) {
		t.Parallel()
		// Waiver expiry and staleness read this instant. Two samples taken
		// minutes apart would let a finding age out on one side of the same
		// comparison and not the other.
		head := newRunContext(currentCfg, subtree)
		head.EvaluatedAt = evaluated
		base := baseRunContext(head, baseWorktree)
		if !base.evaluatedAt().Equal(head.evaluatedAt()) {
			t.Errorf("base evaluatedAt = %v, want head %v", base.evaluatedAt(), head.evaluatedAt())
		}
		if base.ScanRoot != baseWorktree {
			t.Errorf("base ScanRoot = %q, want %q", base.ScanRoot, baseWorktree)
		}
		if head.ScanRoot != subtree {
			t.Errorf("baseRunContext mutated the head ScanRoot: %q", head.ScanRoot)
		}
	})
}

// TestOnDiskWithin_RejectsNonLocalPaths pins the files[] containment contract
// at the I/O boundary: the resolver's slash-only guard cannot see OS-specific
// separators, so the onDisk closure itself must reject candidates that are
// absolute or escape the scan root after OS-path conversion — even when the
// escaped target really exists on disk.
func TestOnDiskWithin_RejectsNonLocalPaths(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "real.go"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "outside.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	onDisk := onDiskWithin(root)
	cases := []struct {
		rel  string
		want bool
	}{
		{"pkg/real.go", true},
		{"pkg", true},
		{"../outside.txt", false},        // exists, but escapes the root
		{"sub/../../outside.txt", false}, // cleans to an escape
		{"/etc", false},                  // absolute
		{"", false},
		{"pkg/missing.go", false},
	}
	for _, tc := range cases {
		if got := onDisk(tc.rel); got != tc.want {
			t.Errorf("onDisk(%q) = %v, want %v", tc.rel, got, tc.want)
		}
	}
}
