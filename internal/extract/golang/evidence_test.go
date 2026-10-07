package golang

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/scope"
)

const (
	evidenceSourceFile = "main.go"
	evidenceGoModFile  = "go.mod"
)

func TestExtractZeroLoadedFilesUsesIndependentSourceInventory(t *testing.T) {
	for _, tc := range []struct {
		name       string
		file       string
		content    string
		exclusions []string
		want       string
	}{
		{name: "failed load over source", file: evidenceSourceFile, content: "package main\n", want: statusPartial},
		{name: "empty module", want: statusAbsent},
		{name: "excluded source", file: "ignored/main.go", content: "package ignored\n", exclusions: []string{"ignored/**"}, want: statusAbsent},
		{name: "ignored Go directory", file: "testdata/main.go", content: "package fixture\n", want: statusAbsent},
		{name: "inactive build constraint", file: evidenceSourceFile, content: "//go:build archfit_evidence_inactive\n\npackage main\n", want: statusAbsent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/evidence\n\ngo 1.26\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.file != "" {
				file := filepath.Join(root, tc.file)
				if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(tc.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			ex := New(evidenceports.ExtractConfig{Exclusions: tc.exclusions})
			ex.load = func(*packages.Config, ...string) ([]*packages.Package, error) {
				return []*packages.Package{{PkgPath: "./...", Errors: []packages.Error{{Msg: "build cache unavailable", Kind: packages.ListError}}}}, nil
			}
			_, coverage, err := ex.Extract(context.Background(), scope.Scope{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			if coverage.Status != tc.want {
				t.Fatalf("coverage = %+v, want %s", coverage, tc.want)
			}
			if tc.want == statusPartial && (coverage.Reason == "" || coverage.FilesApplicable != 1 || coverage.UnresolvedInputsMissing == 0) {
				t.Fatalf("missing failed-load evidence: %+v", coverage)
			}
		})
	}
}

func TestExtractUnavailableBuildCacheDoesNotReportAbsent(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		evidenceGoModFile:  "module example.com/cachefailure\n\ngo 1.26\n",
		evidenceSourceFile: "package cachefailure\n",
		"blocked-cache":    "not a directory",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GOCACHE", filepath.Join(root, "blocked-cache"))
	t.Setenv("GOWORK", "off")
	ext := New(evidenceports.ExtractConfig{})
	_, coverage, err := ext.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Status != statusPartial || coverage.Reason == "" {
		t.Fatalf("unavailable cache hid failed source loading: %+v", coverage)
	}
}

// TestExtractDisclosesFilesExcludedByBuildConstraints pins the disclosure of Go
// source the host build configuration leaves out of the load: a file for another
// GOOS and a tag-gated file are never parsed, so an import in them reaches no
// rule. The count rides the go/packages reason and survives the fact cache; the
// status stays ok because the load itself is complete. Constrained _test.go files
// are not counted — tests are never loaded on any platform.
func TestExtractDisclosesFilesExcludedByBuildConstraints(t *testing.T) {
	otherOS := "windows"
	if runtime.GOOS == otherOS {
		otherOS = "linux"
	}
	otherOSFile := "store_" + otherOS + ".go"
	root := t.TempDir()
	for name, content := range map[string]string{
		evidenceGoModFile:               "module example.com/constrained\n\ngo 1.26\n",
		"app.go":                        "package constrained\n",
		otherOSFile:                     "package constrained\n\nimport _ \"os\"\n",
		"enterprise.go":                 "//go:build customtag\n\npackage constrained\n",
		"store_" + otherOS + "_test.go": "package constrained\n",
		"internal/feature/feature.go":   "package feature\n",
		"internal/feature/feature_x.go": "//go:build customtag\n\npackage feature\n",
	} {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	customTagFlags := []string{"-tags", "customtag"}
	for _, tc := range []struct {
		name string
		cfg  evidenceports.ExtractConfig
		want string
	}{
		{name: "host build", want: "3 Go file(s) excluded by build constraints"},
		{name: "tag enabled", cfg: evidenceports.ExtractConfig{BuildFlags: customTagFlags}, want: "1 Go file(s) excluded by build constraints"},
		{name: "config exclusion wins", cfg: evidenceports.ExtractConfig{Exclusions: []string{"internal/**", otherOSFile}}, want: "1 Go file(s) excluded by build constraints"},
		{name: "nothing left out", cfg: evidenceports.ExtractConfig{BuildFlags: customTagFlags, Exclusions: []string{otherOSFile}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loads := 0
			ex := New(tc.cfg)
			ex.Cache = factcache.NewStore(t.TempDir())
			ex.load = func(cfg *packages.Config, patterns ...string) ([]*packages.Package, error) {
				loads++
				return packages.Load(cfg, patterns...)
			}
			reasons := make([]string, 0, 2)
			for range 2 {
				_, coverage, err := ex.Extract(context.Background(), scope.Scope{Root: root})
				if err != nil {
					t.Fatal(err)
				}
				if coverage.Status != statusOK {
					t.Fatalf("coverage = %+v, want ok: excluded files are not a failed load", coverage)
				}
				reasons = append(reasons, coverage.Reason)
			}
			if loads != 1 {
				t.Fatalf("packages.Load ran %d times, want the second run served from the fact cache", loads)
			}
			if reasons[0] != reasons[1] {
				t.Errorf("cold reason %q, warm reason %q: the disclosure must survive the fact cache", reasons[0], reasons[1])
			}
			if tc.want == "" && reasons[0] != "" {
				t.Errorf("reason = %q, want none when every Go file is analyzed", reasons[0])
			}
			if !strings.Contains(reasons[0], tc.want) {
				t.Errorf("reason = %q, want it to contain %q", reasons[0], tc.want)
			}
		})
	}
}
