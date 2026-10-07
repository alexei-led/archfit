package acquisition

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const goToolVersion = "go1.26.0"

func goOnlyService() *Service {
	return &Service{Runner: &toolrun.RunnerMock{RunFunc: func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) {
		return toolrun.Output{Stdout: []byte(`{"GOOS":"linux","GOARCH":"amd64","CGO_ENABLED":"0","GOFLAGS":"","GOEXPERIMENT":"","GO111MODULE":"","GOTOOLCHAIN":"auto","GO386":"","GOAMD64":"v1","GOARM":"","GOARM64":"","GOMIPS":"","GOMIPS64":"","GOPPC64":"","GORISCV64":"","GOWASM":""}`)}, nil
	}}}
}

func producerTools(p *evidence.MeasurementProfile) []string {
	out := make([]string, 0, len(p.Producers))
	for _, producer := range p.Producers {
		out = append(out, producer.Tool)
	}
	return out
}

// A release that registers a language must not move the profile of a tree that
// has none of it: the row for that language is absent with no gap and no files.
func TestProfileV2IgnoresNotApplicableLanguages(t *testing.T) {
	s := goOnlyService()
	goRow := evidence.Coverage{Tool: registry.ToolGoPackages, Version: goToolVersion, Status: evidence.StatusOK}
	index := map[string]fileclass.FileClass{"main.go": fileclass.Production}
	without := s.measurementProfile(context.Background(), scope.Scope{Root: "/r"}, []evidence.Coverage{goRow}, nil, index, nil)
	absent := []evidence.Coverage{goRow,
		{Tool: registry.ToolGrimp, Status: evidence.StatusAbsent},
		{Tool: registry.ToolDepCruiser, Status: evidence.StatusAbsent},
		{Tool: registry.ToolCargo, Status: evidence.StatusAbsent},
		{Tool: registry.ToolCargoModules, Status: evidence.StatusAbsent}}
	with := s.measurementProfile(context.Background(), scope.Scope{Root: "/r"}, absent, nil, index, nil)
	if with.SettingsHash != without.SettingsHash {
		t.Errorf("settings hash moved when absent rows of languages with no files were added")
	}
	if got, want := strings.Join(producerTools(with), ","), strings.Join(producerTools(without), ","); got != want {
		t.Errorf("producers = %s, want %s", got, want)
	}
}

func TestProfileV2KeepsAbsentRowsThatMayMatter(t *testing.T) {
	s := goOnlyService()
	grimp := evidence.Coverage{Tool: registry.ToolGrimp, Status: evidence.StatusAbsent}
	tests := []struct {
		name  string
		rows  []evidence.Coverage
		gaps  []evidence.CoverageGap
		files map[string]fileclass.FileClass
		keep  bool
	}{
		{"no files, no gap", []evidence.Coverage{grimp}, nil, nil, false},
		{"stray python file", []evidence.Coverage{grimp}, nil, map[string]fileclass.FileClass{"tools/x.py": fileclass.Production}, true},
		{"coverage gap asks for the tool", []evidence.Coverage{grimp}, []evidence.CoverageGap{{Tool: registry.ToolGrimp}}, nil, true},
		{"switched off over present source", []evidence.Coverage{{Tool: registry.ToolGrimp, Status: evidence.StatusDisabled}}, nil, nil, true},
		{"ran", []evidence.Coverage{{Tool: registry.ToolGrimp, Status: evidence.StatusOK, Version: "grimp 3; python 3.12"}}, nil, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := s.measurementProfile(context.Background(), scope.Scope{Root: "/r"}, tc.rows, tc.gaps, tc.files, nil)
			kept := strings.Contains(strings.Join(producerTools(p), ","), registry.ToolGrimp)
			if kept != tc.keep {
				t.Errorf("grimp kept = %v, want %v", kept, tc.keep)
			}
		})
	}
}

func TestNormalizeToolVersion(t *testing.T) {
	long := strings.Repeat("v", 300)
	tests := []struct {
		name, in string
		check    func(t *testing.T, got string)
	}{
		{"plain", "go1.27.1", func(t *testing.T, got string) { eq(t, got, "go1.27.1") }},
		{"first non-empty line", "\n\ngit version 2.55.0\nextra build info", func(t *testing.T, got string) { eq(t, got, "git version 2.55.0") }},
		{"whitespace and control runs collapse", "tool\t 1.2\x00\x1b[0m  x", func(t *testing.T, got string) { eq(t, got, "tool 1.2[0m x") }},
		{"empty", "  \n ", func(t *testing.T, got string) { eq(t, got, "") }},
		{"long text is bounded and keeps identity", long, func(t *testing.T, got string) {
			if utf8.RuneCountInString(got) > maxToolVersionLen {
				t.Errorf("length %d exceeds %d", utf8.RuneCountInString(got), maxToolVersionLen)
			}
			if other := normalizeToolVersion(long + "x"); other == got {
				t.Error("two different long versions normalized to the same string")
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { tc.check(t, normalizeToolVersion(tc.in)) })
	}
}

func eq(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
