package acquisition

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	ports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	goToolVersion = "go1.26.0"
	mainGoFile    = "main.go"
)

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
	index := map[string]fileclass.FileClass{mainGoFile: fileclass.Production}
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

// The settings hash must not read configuration of a language that is not in
// the tree: not its extractor config, and not its syntax flag.
func TestProfileV2SettingsIgnoreNotApplicableLanguageConfig(t *testing.T) {
	configured := func(pyPackage string, syntaxLangs ...string) *Service {
		s := goOnlyService()
		s.Options.Extractors = registry.Configs{"go": {Src: "."}, graph.LangPython: {PyPackage: pyPackage}}
		s.Options.Syntax = ports.SyntaxConfig{Enabled: true, Languages: syntaxLangs}
		return s
	}
	goRow := evidence.Coverage{Tool: registry.ToolGoPackages, Version: goToolVersion, Status: evidence.StatusOK}
	grimp := evidence.Coverage{Tool: registry.ToolGrimp, Status: evidence.StatusAbsent}
	hash := func(s *Service, files map[string]fileclass.FileClass) string {
		return s.measurementProfile(context.Background(), scope.Scope{Root: "/r"}, []evidence.Coverage{goRow, grimp}, nil, files, nil).SettingsHash
	}
	goFiles := map[string]fileclass.FileClass{mainGoFile: fileclass.Production}
	pyFiles := map[string]fileclass.FileClass{mainGoFile: fileclass.Production, "pkg/a.py": fileclass.Production}

	base := hash(configured("one", "go", graph.LangPython), goFiles)
	if hash(configured("two", "go", graph.LangPython), goFiles) != base {
		t.Error("python extractor config moved the hash on a tree with no python")
	}
	if hash(configured("one", "go"), goFiles) != base {
		t.Error("switching python out of the syntax languages moved the hash on a tree with no python")
	}
	applicable := hash(configured("one", "go", graph.LangPython), pyFiles)
	if applicable == base {
		t.Error("a python file in the tree did not add the python slice")
	}
	if hash(configured("two", "go", graph.LangPython), pyFiles) == applicable {
		t.Error("python extractor config did not move the hash once python is in the tree")
	}
}

// The Roslyn row joins the .NET slice: absent on a tree with no C# file it
// moves neither the producers nor the settings hash; a C# file keeps it.
func TestProfileV2RoslynRowFollowsCSharpSource(t *testing.T) {
	s := goOnlyService()
	goRow := evidence.Coverage{Tool: registry.ToolGoPackages, Version: goToolVersion, Status: evidence.StatusOK}
	roslyn := evidence.Coverage{Tool: registry.ToolRoslyn, Status: evidence.StatusAbsent}
	profile := func(rows []evidence.Coverage, files map[string]fileclass.FileClass) *evidence.MeasurementProfile {
		return s.measurementProfile(context.Background(), scope.Scope{Root: "/r"}, rows, nil, files, nil)
	}
	goFiles := map[string]fileclass.FileClass{mainGoFile: fileclass.Production}
	without := profile([]evidence.Coverage{goRow}, goFiles)
	with := profile([]evidence.Coverage{goRow, roslyn}, goFiles)
	if with.SettingsHash != without.SettingsHash {
		t.Error("settings hash moved when an absent roslyn row was added to a tree with no C# file")
	}
	if got, want := strings.Join(producerTools(with), ","), strings.Join(producerTools(without), ","); got != want {
		t.Errorf("producers = %s, want %s", got, want)
	}
	csFiles := map[string]fileclass.FileClass{mainGoFile: fileclass.Production, "src/Core/User.cs": fileclass.Production}
	if kept := profile([]evidence.Coverage{goRow, roslyn}, csFiles); !strings.Contains(strings.Join(producerTools(kept), ","), registry.ToolRoslyn) {
		t.Errorf("absent roslyn over C# source left the profile: %v", producerTools(kept))
	}
}

// Registering .NET must not move the measurement profile of a tree with no C#:
// the extractor's own probe finds no project, so neither .NET row raises a gap,
// and both absent rows leave the producers and the settings hash unchanged.
func TestProfileV2UnchangedByTheDotNetRowsOnATreeWithoutCSharp(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, mainGoFile), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := func(root string) bool { return registry.ProjectPresent(graph.LangDotNet, root, ports.ExtractConfig{}) }
	opts := CoverageOptions{ProjectPresent: map[string]func(string) bool{registry.ToolDotNetMSBuild: probe, registry.ToolRoslyn: probe}}
	goRow := evidence.Coverage{Tool: registry.ToolGoPackages, Version: goToolVersion, Status: evidence.StatusOK}
	dotnetRows := []evidence.Coverage{goRow,
		{Tool: registry.ToolDotNetMSBuild, Status: evidence.StatusAbsent},
		{Tool: registry.ToolRoslyn, Status: evidence.StatusAbsent}}
	gaps := buildCoverageGaps(dotnetRows, opts, root)
	if len(gaps) != 0 {
		t.Fatalf("gaps = %+v, want none on a tree with no .NET project", gaps)
	}
	s := goOnlyService()
	files := map[string]fileclass.FileClass{mainGoFile: fileclass.Production}
	before := s.measurementProfile(context.Background(), scope.Scope{Root: root}, []evidence.Coverage{goRow}, nil, files, nil)
	after := s.measurementProfile(context.Background(), scope.Scope{Root: root}, dotnetRows, gaps, files, nil)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("profile moved when the .NET rows landed:\nbefore %+v\nafter  %+v", before, after)
	}
}

// Over a .NET project the absent roslyn row is a gap that names the helper
// build, and an explicit gate demands it even where the probe finds nothing.
func TestCoverageGapForAbsentRoslynFollowsTheDotNetProbe(t *testing.T) {
	withProject := t.TempDir()
	if err := os.WriteFile(filepath.Join(withProject, "App.csproj"), []byte("<Project Sdk=\"Microsoft.NET.Sdk\" />\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := func(root string) bool { return registry.ProjectPresent(graph.LangDotNet, root, ports.ExtractConfig{}) }
	rows := []evidence.Coverage{{Tool: registry.ToolRoslyn, Status: evidence.StatusAbsent}}
	for _, tc := range []struct {
		name, root, gate string
		wantGap          bool
	}{
		{"a .NET project without the helper", withProject, "", true},
		{"no .NET project", t.TempDir(), "", false},
		{"no .NET project, gate demands it", t.TempDir(), gateWarn, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := CoverageOptions{Gates: map[string]string{"roslyn": tc.gate}, ProjectPresent: map[string]func(string) bool{registry.ToolRoslyn: probe}}
			gaps := buildCoverageGaps(rows, opts, tc.root)
			if got := len(gaps) == 1 && strings.Contains(gaps[0].InstallCmd, "make dotnet-roslyn"); got != tc.wantGap {
				t.Errorf("roslyn gap = %+v, want gap %v", gaps, tc.wantGap)
			}
		})
	}
}
