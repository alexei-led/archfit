package golang

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/scope"
)

const evidenceSourceFile = "main.go"

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
			_, coverage, err := ex.Extract(context.Background(), scope.Scope{Root: root, Mode: scope.ModeFull})
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
		"go.mod":           "module example.com/cachefailure\n\ngo 1.26\n",
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
	_, coverage, err := ext.Extract(context.Background(), scope.Scope{Root: root, Mode: scope.ModeFull})
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Status != statusPartial || coverage.Reason == "" {
		t.Fatalf("unavailable cache hid failed source loading: %+v", coverage)
	}
}
