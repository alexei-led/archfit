package resolved_test

import (
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

const (
	chaosGlob   = "pkg/chaos/**"
	chaosDir    = "pkg/chaos"
	chaosFile   = "pkg/chaos/chaos.go"
	testGlob    = "**/*_test.go"
	modUI       = "ui"
	lastDayLeft = "2026-11-19 (0 days)"
)

func TestCovers(t *testing.T) {
	var p resolved.Policy
	tests := []struct {
		glob, path string
		want       bool
	}{
		{chaosGlob, "pkg/chaos/netem/netem.go", true},
		{chaosGlob, chaosDir, true},
		{chaosDir, chaosDir, true},
		{chaosDir, chaosFile, false},
		{"pkg/*", chaosFile, false},
		{"pkg/*.go", "pkg/main.go", true},
		{chaosGlob, "pkg/chaosx/x.go", false},
		{"pkg/[", "pkg/[", false},
	}
	for _, tt := range tests {
		if got := p.Covers(tt.glob, tt.path); got != tt.want {
			t.Errorf("Covers(%q, %q) = %v, want %v", tt.glob, tt.path, got, tt.want)
		}
	}
}

func TestIsPublic(t *testing.T) {
	// base: pkg_chaos has public [pkg/chaos]; pkg_util names no public.
	p := base()
	chaos := p.Modules[modChaos]
	chaos.Public = append(chaos.Public, "pkg/chaos/cliflags")
	p.Modules[modChaos] = chaos
	p.Modules[modUI] = resolved.Module{Paths: []string{"src/ui/**"}, Public: []string{"src/ui/api/**"}}
	p.Modules["files"] = resolved.Module{Paths: []string{"pkg/foo/*.go"}}
	p.Modules["filepub"] = resolved.Module{Paths: []string{"pkg/bar/*.go"}, Public: []string{"pkg/bar/api.go"}}
	p = p.WithDefaults()
	tests := []struct {
		name, module, target string
		isFile               bool
		want                 bool
	}{
		{"a public package", modChaos, chaosDir, false, true},
		{"a listed subpackage", modChaos, "pkg/chaos/cliflags", false, true},
		{"a subpackage past the public part", modChaos, "pkg/chaos/netem", false, false},
		{"a file of a public package", modChaos, chaosFile, true, true},
		{"a file of a subpackage", modChaos, "pkg/chaos/netem/netem.go", true, false},
		{"a file under a public tree", modUI, "src/ui/api/client/index.ts", true, true},
		{"a file past a public tree", modUI, "src/ui/state/store.ts", true, false},
		{"no public: the whole module", modUtil, "pkg/util/deep/x", false, true},
		{"no public, file-glob paths: the package", "files", "pkg/foo", false, true},
		{"no public, file-glob paths: a file", "files", "pkg/foo/foo.go", true, true},
		{"a public file", "filepub", "pkg/bar/api.go", true, true},
		{"a file next to a public file", "filepub", "pkg/bar/impl.go", true, false},
		{"not a module", "missing", chaosDir, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.IsPublic(tt.module, tt.target, tt.isFile); got != tt.want {
				t.Errorf("IsPublic(%q, %q, %v) = %v, want %v", tt.module, tt.target, tt.isFile, got, tt.want)
			}
		})
	}
}

func TestIsTestAndIsGenerated(t *testing.T) {
	p := resolved.Policy{Tests: []string{testGlob, "e2e/**"}, Generated: []string{"**/*.pb.go"}}
	tests := []struct {
		file                    string
		wantTest, wantGenerated bool
	}{
		{"pkg/chaos/chaos_test.go", true, false},
		{"e2e/run.go", true, false},
		{"api/v1/api.pb.go", false, true},
		{chaosFile, false, false},
		{"pkg/chaos/testdata/x.go", false, false},
	}
	for _, tt := range tests {
		if got := p.IsTest(tt.file); got != tt.wantTest {
			t.Errorf("IsTest(%q) = %v, want %v", tt.file, got, tt.wantTest)
		}
		if got := p.IsGenerated(tt.file); got != tt.wantGenerated {
			t.Errorf("IsGenerated(%q) = %v, want %v", tt.file, got, tt.wantGenerated)
		}
	}
	var none resolved.Policy
	if none.IsTest("x_test.go") || none.IsGenerated("x.pb.go") {
		t.Error("a policy without tests or generated takes no file out")
	}
}

func TestFormatExpires(t *testing.T) {
	tests := []struct {
		name           string
		expires, today time.Time
		want           string
	}{
		{"days left", day(19), day(10), "2026-11-19 (9 days)"},
		{"one day left", day(19), day(18), "2026-11-19 (1 day)"},
		{"the last day", day(19), day(19), lastDayLeft},
		{"the last day, late in the day", day(19), day(19).Add(23 * time.Hour), lastDayLeft},
		{"today read in UTC", day(19), time.Date(2026, 11, 20, 0, 30, 0, 0, time.FixedZone("CET", 3600)), lastDayLeft},
		{"the day after", day(19), day(20), "2026-11-19 (expired)"},
		{"long past", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), day(20), "2026-01-01 (expired)"},
		{"a date past the time.Duration range", time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC), day(11), "9999-12-31 (2912128 days)"},
		{"no date", time.Time{}, day(19), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolved.FormatExpires(tt.expires, tt.today); got != tt.want {
				t.Errorf("FormatExpires(%v, %v) = %q, want %q", tt.expires, tt.today, got, tt.want)
			}
		})
	}
}
