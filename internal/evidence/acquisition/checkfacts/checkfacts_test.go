package checkfacts_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition/checkfacts"
	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	fileGoMod  = "go.mod"
	fileMainGo = "main.go"
	filePkgA   = "pkg/a/a.go"
	filePkgB   = "pkg/b/b.go"
	settingOn  = "true"
	snapshot   = "git read-tree"
)

func requireTool(t *testing.T, tool string) {
	t.Helper()
	if _, err := exec.LookPath(tool); err != nil {
		t.Skip(tool + " not installed")
	}
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)...) //nolint:gosec // fixed binary, test args
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// repo commits files as the base and returns the repository root and the
// base commit.
func repo(t *testing.T, files map[string]string) (root, base string) {
	t.Helper()
	root = t.TempDir()
	gitIn(t, root, "init", "-q")
	writeFiles(t, root, files)
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-qm", "base")
	return root, gitIn(t, root, "rev-parse", "HEAD")
}

func goConfig() checkfacts.Config {
	return checkfacts.Config{Languages: registry.Configs{"go": {Src: ".", Mode: evidenceports.ModeAuto}}, Literals: true}
}

var goFiles = map[string]string{
	".gitignore": "dist/\n",
	fileGoMod:    "module example.com/m\n\ngo 1.22\n",
	fileMainGo:   "package main\n\nimport \"example.com/m/pkg/a\"\n\nvar _ = a.X\n",
	filePkgA:     "package a\n\nconst X = \"a long literal value\"\n",
}

// recorder counts the commands a run starts, by tool and first argument.
type recorder struct {
	toolrun.Runner
	mu    sync.Mutex
	calls []string
}

func (r *recorder) Run(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	r.mu.Lock()
	r.calls = append(r.calls, cmd.Name+" "+strings.Join(cmd.Args, " "))
	r.mu.Unlock()
	return r.Runner.Run(ctx, cmd)
}

// snapshots counts the tree snapshots the run wrote.
func (r *recorder) snapshots() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.calls {
		if strings.Contains(c, snapshot) {
			n++
		}
	}
	return n
}

// TestHeadIsTheWorkingTreeAsACommit: the head holds edits and untracked files
// and not ignored ones; the base holds the commit; the repository's index and
// status do not change.
func TestHeadIsTheWorkingTreeAsACommit(t *testing.T) {
	requireTool(t, "git")
	t.Parallel()
	root, base := repo(t, goFiles)
	writeFiles(t, root, map[string]string{
		filePkgB:        "package b\n\nimport \"example.com/m/pkg/a\"\n\nvar _ = a.X\n",
		"dist/out/x.go": "package out\n\nimport \"example.com/m/pkg/a\"\n",
	})
	status := gitIn(t, root, "status", "--porcelain")
	a := &checkfacts.Acquirer{Runner: toolrun.New(), Config: goConfig()}

	head, err := a.Head(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{fileMainGo, filePkgA, filePkgB}; !slices.Equal(head.Sources, want) {
		t.Errorf("head sources = %q, want %q (untracked in, ignored out)", head.Sources, want)
	}
	if !slices.Contains(head.Imports, evidence.ImportFact{File: filePkgB, Line: 3, Target: "pkg/a"}) {
		t.Errorf("head imports %+v lack the untracked file's import", head.Imports)
	}
	baseFacts, err := a.Base(context.Background(), root, base)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{fileMainGo, filePkgA}; !slices.Equal(baseFacts.Sources, want) {
		t.Errorf("base sources = %q, want %q", baseFacts.Sources, want)
	}
	if got := gitIn(t, root, "status", "--porcelain"); got != status {
		t.Errorf("status changed:\n%s\nwas\n%s", got, status)
	}
}

// TestFactsComeFromTheCache: a tree read once is never read again: no
// snapshot and no extractor run for the base or an unchanged head, and equal
// base and head trees are read once. An edit is a new tree. Without an engine
// version nothing is cached.
func TestFactsComeFromTheCache(t *testing.T) {
	requireTool(t, "git")
	t.Parallel()
	root, base := repo(t, goFiles)
	ctx := context.Background()
	rec := &recorder{Runner: toolrun.New()}
	store := factcache.NewStore(t.TempDir())
	newAcquirer := func() *checkfacts.Acquirer {
		return &checkfacts.Acquirer{Runner: rec, Store: store, Version: "v-test", Config: goConfig()}
	}

	first, err := newAcquirer().Base(ctx, root, base)
	if err != nil || rec.snapshots() != 1 {
		t.Fatalf("first base: %d snapshots, %v; want 1", rec.snapshots(), err)
	}
	head, err := newAcquirer().Head(ctx, root)
	if err != nil || rec.snapshots() != 1 || !reflect.DeepEqual(head, first) {
		t.Errorf("head of the same tree: %d snapshots, %v; want the base's cached facts", rec.snapshots(), err)
	}
	again, err := newAcquirer().Base(ctx, root, base)
	if err != nil || rec.snapshots() != 1 || !reflect.DeepEqual(again, first) {
		t.Errorf("second base: %d snapshots, %v; want the cached facts", rec.snapshots(), err)
	}

	writeFiles(t, root, map[string]string{filePkgA: "package a\n\nconst X = \"another long literal\"\n"})
	edited, err := newAcquirer().Head(ctx, root)
	if err != nil || rec.snapshots() != 2 || edited.Literals[0].Value != "another long literal" {
		t.Errorf("edited head: %d snapshots, %+v, %v; want a fresh read", rec.snapshots(), edited.Literals, err)
	}

	uncached := &checkfacts.Acquirer{Runner: rec, Store: store, Config: goConfig()}
	if _, err := uncached.Base(ctx, root, base); err != nil || rec.snapshots() != 3 {
		t.Errorf("no version: %d snapshots, %v; want a read", rec.snapshots(), err)
	}
}

// TestBaseThatCannotBeReadIsUnknown: one unknown item of cause base that
// names the fetch, not an error.
func TestBaseThatCannotBeReadIsUnknown(t *testing.T) {
	requireTool(t, "git")
	t.Parallel()
	root, _ := repo(t, goFiles)
	a := &checkfacts.Acquirer{Runner: toolrun.New(), Config: goConfig()}
	const missing = "0123456789012345678901234567890123456789"
	got, err := a.Base(context.Background(), root, missing)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Unknown) != 1 || got.Unknown[0].Cause != evidence.CauseBase || got.Unknown[0].Specifier != missing ||
		got.Unknown[0].Fix != "run git fetch, then archfit check again" || len(got.Sources) != 0 {
		t.Errorf("facts = %+v, want one base item", got)
	}
}

// TestOnlyGitAndAstGrepAreNeeded: with nothing but git and ast-grep on PATH,
// Go, TypeScript and Python facts are complete, and the run starts no other
// tool: no go, node, depcruise, python, uv, grimp, SCIP indexer or jscpd.
func TestOnlyGitAndAstGrepAreNeeded(t *testing.T) {
	requireTool(t, "git")
	requireTool(t, registry.ToolAstGrep)
	bin := t.TempDir()
	for _, tool := range []string{"git", registry.ToolAstGrep} {
		p, _ := exec.LookPath(tool)
		if err := os.Symlink(p, filepath.Join(bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	root, _ := repo(t, map[string]string{
		fileGoMod:             "module example.com/m\n\ngo 1.22\n",
		fileMainGo:            "package main\n\nimport \"example.com/m/pkg/a\"\n\nvar _ = a.X\n",
		filePkgA:              "package a\n\nconst X = 1\n",
		"web/a.ts":            "import { b } from './b';\n",
		"web/b.ts":            "export const b = 1;\n",
		"src/app/__init__.py": "",
		"src/app/x.py":        "from app import y\n",
		"src/app/y.py":        "",
	})
	t.Setenv("PATH", bin)
	rec := &recorder{Runner: toolrun.New()}
	langs := registry.Configs{}
	for _, d := range registry.All() {
		langs[d.ID] = evidenceports.ExtractConfig{Src: ".", Mode: evidenceports.ModeAuto}
	}
	a := &checkfacts.Acquirer{Runner: rec, Config: checkfacts.Config{Languages: langs, Literals: true}}
	got, err := a.Head(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	want := []evidence.ImportFact{
		{File: fileMainGo, Line: 3, Target: "pkg/a"},
		{File: "src/app/x.py", Line: 1, Target: "src/app/y.py"},
		{File: "web/a.ts", Line: 1, Target: "web/b.ts"},
	}
	if !reflect.DeepEqual(got.Imports, want) || len(got.Unknown) != 0 {
		t.Errorf("imports = %+v, unknown = %+v; want %+v and none", got.Imports, got.Unknown, want)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, c := range rec.calls {
		if tool, _, _ := strings.Cut(c, " "); tool != "git" && tool != registry.ToolAstGrep {
			t.Errorf("ran %q, want git and ast-grep only", c)
		}
	}
}

// TestConfigFromReadsTheLanguagesAndNoAnalyzer: the languages block and
// exclude reach the extractors; the analyzers keys change nothing, so the
// facts are the same with every optional analyzer on or off; duplication off
// turns the literals off.
func TestConfigFromReadsTheLanguagesAndNoAnalyzer(t *testing.T) {
	t.Parallel()
	settings := map[string]string{
		"languages.go.enabled":            "auto",
		"languages.go.modules.include[0]": "svc/*",
		"languages.go.modules.exclude[0]": "svc/old",
		"languages.typescript.enabled":    "false",
		"languages.python.enabled":        settingOn,
		"languages.python.package":        "app",
		"languages.rust.manifest":         "crates/Cargo.toml",
		"languages.rust.features[0]":      "a",
		"languages.rust.features[1]":      "b",
		"languages.rust.include_dev_deps": settingOn,
		"languages.dotnet.projects[0]":    "App.slnx",
	}
	p := resolved.Policy{Exclude: []string{"gen/**"}, Settings: settings, Builtin: resolved.Builtin{Duplication: resolved.ModeAsk}}
	got := checkfacts.ConfigFrom(p)
	want := checkfacts.Config{
		Exclude:  []string{"gen/**"},
		Literals: true,
		Languages: registry.Configs{
			"go":         {Src: ".", Mode: evidenceports.ModeAuto, GoModuleInclude: []string{"svc/*"}, GoModuleExclude: []string{"svc/old"}},
			"typescript": {Src: ".", Mode: evidenceports.ModeOff},
			"python":     {Src: ".", Mode: evidenceports.ModeOn, PyPackage: "app"},
			"rust":       {Src: ".", Mode: evidenceports.ModeAuto, CargoManifest: "crates/Cargo.toml", CargoFeatures: []string{"a", "b"}, IncludeDevDeps: true},
			"dotnet":     {Src: ".", Mode: evidenceports.ModeAuto, DotNetProjects: []string{"App.slnx"}, TypeGraphDisabled: true},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ConfigFrom =\n%+v\nwant\n%+v", got, want)
	}

	on, off := map[string]string{}, map[string]string{}
	for k, v := range settings {
		on[k], off[k] = v, v
	}
	for _, key := range []string{"analyzers.syntax.enabled", "analyzers.cargo_modules.enabled", "analyzers.roslyn.enabled"} {
		on[key], off[key] = settingOn, "false"
	}
	if a, b := checkfacts.ConfigFrom(resolved.Policy{Settings: on}), checkfacts.ConfigFrom(resolved.Policy{Settings: off}); !reflect.DeepEqual(a, b) {
		t.Errorf("analyzers on and off give different configs:\n%+v\n%+v", a, b)
	}
	if got := checkfacts.ConfigFrom(resolved.Policy{Builtin: resolved.Builtin{Duplication: resolved.ModeOff}}); got.Literals {
		t.Error("duplication off still reads literals")
	}
}

// cargoStub answers cargo over a real runner: --version with version, and
// metadata for the root crate of the tree it runs in, or, when failing, an
// error that names the manifest by its absolute path, as cargo does.
type cargoStub struct {
	toolrun.Runner
	mu      sync.Mutex
	version string
	failing bool
}

func (c *cargoStub) set(version string, failing bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.version, c.failing = version, failing
}

func (c *cargoStub) Detect(ctx context.Context, tool string) (toolrun.ToolInfo, bool) {
	if tool == registry.ToolCargo {
		return toolrun.ToolInfo{Name: tool}, true
	}
	return c.Runner.Detect(ctx, tool)
}

func (c *cargoStub) Run(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	if cmd.Name != registry.ToolCargo {
		return c.Runner.Run(ctx, cmd)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if slices.Contains(cmd.Args, "--version") {
		return toolrun.Output{Stdout: []byte(c.version + "\n")}, nil
	}
	dir, err := filepath.EvalSymlinks(cmd.WorkDir)
	if err != nil {
		return toolrun.Output{}, err
	}
	manifest := filepath.Join(dir, "Cargo.toml")
	if c.failing {
		return toolrun.Output{ExitCode: 101, Stderr: []byte("error: failed to parse manifest at `" + manifest + "`")}, nil
	}
	id := "path+file://" + dir + "#app"
	metadata := `{"packages":[{"id":"` + id + `","name":"app","manifest_path":"` + manifest + `","dependencies":[],"targets":[{"name":"app","kind":["bin"]}]}],"workspace_members":["` + id + `"],"workspace_root":"` + dir + `"}`
	return toolrun.Output{Stdout: []byte(metadata)}, nil
}

// TestCacheFollowsTheTreesTools: facts read by cargo are served only while
// cargo has the version that read them; facts with a tool that failed are not
// cached, so the next run reads again; a failure's reason names no snapshot
// directory.
func TestCacheFollowsTheTreesTools(t *testing.T) {
	requireTool(t, "git")
	t.Parallel()
	root, _ := repo(t, map[string]string{"Cargo.toml": "[package]\nname = \"app\"\n", "src/main.rs": "fn main() {}\n"})
	ctx := context.Background()
	cargo := &cargoStub{Runner: toolrun.New(), version: "cargo 1.0"}
	rec := &recorder{Runner: cargo}
	store := factcache.NewStore(t.TempDir())
	cfg := checkfacts.Config{Languages: registry.Configs{"rust": {Src: ".", Mode: evidenceports.ModeAuto}}}
	head := func() evidence.CheckFacts {
		t.Helper()
		f, err := (&checkfacts.Acquirer{Runner: rec, Store: store, Version: "v-test", Config: cfg}).Head(ctx, root)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}

	if f := head(); !slices.Equal(f.Sources, []string{"src/main.rs"}) || len(f.Unknown) != 0 {
		t.Fatalf("facts = %+v, want src/main.rs and no unknown", f)
	}
	head()
	if got := rec.snapshots(); got != 1 {
		t.Errorf("same cargo: %d snapshots, want 1 (cached)", got)
	}
	cargo.set("cargo 2.0", false)
	head()
	if got := rec.snapshots(); got != 2 {
		t.Errorf("new cargo: %d snapshots, want 2 (read again)", got)
	}

	cargo.set("cargo 3.0", true)
	failed := head()
	if len(failed.Unknown) != 1 || failed.Unknown[0].Cause != evidence.CauseTool {
		t.Fatalf("unknown = %+v, want one tool item", failed.Unknown)
	}
	if reason := failed.Unknown[0].Reason; strings.Contains(reason, "archfit-check-tree") || !strings.Contains(reason, "./Cargo.toml") {
		t.Errorf("reason %q names the snapshot directory, want ./Cargo.toml", reason)
	}
	head()
	if got := rec.snapshots(); got != 4 {
		t.Errorf("after a failed tool: %d snapshots, want 4 (a failure is never cached)", got)
	}
	cargo.set("cargo 3.0", false)
	if f := head(); len(f.Unknown) != 0 {
		t.Errorf("after the fix: unknown = %+v, want none", f.Unknown)
	}
}

// failingCheckout fails git checkout-index with the message git gives, which
// names the snapshot directory.
type failingCheckout struct{ toolrun.Runner }

func (f failingCheckout) Run(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	for _, arg := range cmd.Args {
		if prefix, ok := strings.CutPrefix(arg, "--prefix="); ok {
			return toolrun.Output{ExitCode: 128, Stderr: []byte("fatal: unable to create file " + prefix + "main.go")}, nil
		}
	}
	return f.Runner.Run(ctx, cmd)
}

// TestBaseSnapshotFailureNamesNoDirectory: a base whose files cannot be
// written is one base item whose reason is the same on every run.
func TestBaseSnapshotFailureNamesNoDirectory(t *testing.T) {
	requireTool(t, "git")
	t.Parallel()
	root, base := repo(t, goFiles)
	a := &checkfacts.Acquirer{Runner: failingCheckout{toolrun.New()}, Config: goConfig()}
	reasons := make([]string, 0, 2)
	for range 2 {
		got, err := a.Base(context.Background(), root, base)
		if err != nil || len(got.Unknown) != 1 || got.Unknown[0].Cause != evidence.CauseBase {
			t.Fatalf("facts = %+v, %v; want one base item", got, err)
		}
		reasons = append(reasons, got.Unknown[0].Reason)
	}
	if strings.Contains(reasons[0], "archfit-check-tree") || reasons[0] != reasons[1] {
		t.Errorf("reasons %q name the snapshot directory or differ between runs", reasons)
	}
}
