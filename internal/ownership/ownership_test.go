package ownership_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/ownership"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// buildModuleMap is a test helper that constructs a config.ModuleMap from a
// name→paths mapping using the exported ForClassify projection.
func buildModuleMap(t *testing.T, mods map[string][]string) config.ModuleMap {
	t.Helper()
	modules := make(map[string]config.ModuleDef, len(mods))
	for name, paths := range mods {
		modules[name] = config.ModuleDef{Paths: paths}
	}
	cfg := config.Config{
		Version: 1,
		Modules: modules,
	}
	return cfg.ForClassify().ModuleMap
}

const (
	modCmd = "cmd"
	modPkg = "pkg"
)

// -----------------------------------------------------------------------
// CODEOWNERS tests
// -----------------------------------------------------------------------

func TestResolve_CodeownersPrecedence(t *testing.T) {
	// File matched by two rules — last rule wins.
	root := t.TempDir()
	writeFile(t, root, ".github/CODEOWNERS", "*.go @first-owner\ninternal/ @last-owner\n")
	writeFile(t, root, "internal/foo.go", "")

	mm := buildModuleMap(t, map[string][]string{
		"core": {"internal/**"},
	})

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, nilRunner())
	if want := "@last-owner"; got["core"] != want {
		t.Errorf("CODEOWNERS last-match-wins: got %q, want %q", got["core"], want)
	}
}

func TestResolve_CodeownersLastMatchWins_ExplicitOrder(t *testing.T) {
	// Explicit ordering: rule for "internal/service/" appears after "internal/"
	// which itself appears after the catch-all. Last matching rule must win.
	root := t.TempDir()
	codeowners := "* @catch-all\ninternal/ @internal-owner\ninternal/service/ @service-owner\n"
	writeFile(t, root, ".github/CODEOWNERS", codeowners)
	writeFile(t, root, "internal/service/handler.go", "")

	mm := buildModuleMap(t, map[string][]string{
		"service": {"internal/service/**"},
	})

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, nilRunner())
	if want := "@service-owner"; got["service"] != want {
		t.Errorf("last rule wins: got %q, want %q", got["service"], want)
	}
}

func TestResolve_CodeownersMissingFile_NoGitFallback(t *testing.T) {
	// CODEOWNERS exists but doesn't match the file — no owner, no git fallback.
	root := t.TempDir()
	writeFile(t, root, "CODEOWNERS", "docs/ @docs-team\n")
	writeFile(t, root, "internal/foo.go", "")

	mm := buildModuleMap(t, map[string][]string{
		"core": {"internal/**"},
	})

	// The runner should never be called because CODEOWNERS exists.
	called := false
	runner := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			called = true
			return toolrun.Output{}, nil
		},
	}

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, runner)
	if called {
		t.Error("git runner was called even though CODEOWNERS exists — must not fall back per-file")
	}
	if _, ok := got["core"]; ok {
		t.Errorf("expected no owner for unmatched module, got %q", got["core"])
	}
}

func TestResolve_CodeownersRootLocation(t *testing.T) {
	// CODEOWNERS at repo root (not .github/) is also picked up.
	root := t.TempDir()
	writeFile(t, root, "CODEOWNERS", "src/ @src-owner\n")
	writeFile(t, root, "src/main.go", "")

	mm := buildModuleMap(t, map[string][]string{
		"app": {"src/**"},
	})

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, nilRunner())
	if want := "@src-owner"; got["app"] != want {
		t.Errorf("CODEOWNERS at root: got %q, want %q", got["app"], want)
	}
}

func TestResolve_CodeownersDocsLocation(t *testing.T) {
	// CODEOWNERS under docs/ is also discovered.
	root := t.TempDir()
	writeFile(t, root, "docs/CODEOWNERS", "lib/ @lib-owner\n")
	writeFile(t, root, "lib/util.go", "")

	mm := buildModuleMap(t, map[string][]string{
		"lib": {"lib/**"},
	})

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, nilRunner())
	if want := "@lib-owner"; got["lib"] != want {
		t.Errorf("CODEOWNERS in docs/: got %q, want %q", got["lib"], want)
	}
}

func TestResolve_CodeownersDirNoTrailingSlash(t *testing.T) {
	// GitHub/gitignore semantics: an anchored path pattern WITHOUT a trailing
	// slash owns that directory and everything beneath it — not just an exact
	// file of that name. (Real-world CODEOWNERS, e.g. omni, write dir patterns
	// this way far more often than with a trailing slash.)
	root := t.TempDir()
	writeFile(t, root, ".github/CODEOWNERS", "/svc/api @api-team\n")
	writeFile(t, root, "svc/api/handler/v1.go", "")

	mm := buildModuleMap(t, map[string][]string{
		"api": {"svc/api/**"},
	})

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, nilRunner())
	if want := "@api-team"; got["api"] != want {
		t.Errorf("dir pattern without trailing slash: got %q, want %q", got["api"], want)
	}
}

func TestResolve_CodeownersWildcardDir(t *testing.T) {
	// Wildcard directory pattern (ruff-style "/crates/ty*"): the "*" segment
	// matches a directory name, and contents beneath it are owned too.
	root := t.TempDir()
	writeFile(t, root, ".github/CODEOWNERS", "/crates/ty* @ty-team\n/crates/ruff_db/ @db-team\n")
	writeFile(t, root, "crates/ty_project/src/lib.rs", "")
	writeFile(t, root, "crates/ruff_db/src/lib.rs", "")

	mm := buildModuleMap(t, map[string][]string{
		"ty":      {"crates/ty_project/**"},
		"ruff_db": {"crates/ruff_db/**"},
	})

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, nilRunner())
	if want := "@ty-team"; got["ty"] != want {
		t.Errorf("wildcard dir pattern: got %q, want %q", got["ty"], want)
	}
	if want := "@db-team"; got["ruff_db"] != want {
		t.Errorf("trailing-slash dir pattern: got %q, want %q", got["ruff_db"], want)
	}
}

func TestResolve_CodeownersSubtree(t *testing.T) {
	// F2 regression: --root is a monorepo subtree. CODEOWNERS lives at the git
	// root and its patterns are gitRoot-relative; module globs are subtree-
	// relative. The owner must still resolve.
	gitRoot := t.TempDir()
	writeFile(t, gitRoot, ".github/CODEOWNERS", "/services/sched/ @sched-team\n")
	writeFile(t, gitRoot, "services/sched/run/job.go", "")
	scanRoot := filepath.Join(gitRoot, "services", "sched")

	mm := buildModuleMap(t, map[string][]string{
		"run": {"run/**"}, // subtree-relative glob
	})

	got, _ := ownership.Resolve(context.Background(), scanRoot, gitRoot, "services/sched", mm, nilRunner())
	if want := "@sched-team"; got["run"] != want {
		t.Errorf("subtree CODEOWNERS: got %q, want %q", got["run"], want)
	}
}

// -----------------------------------------------------------------------
// git-author fallback tests
// -----------------------------------------------------------------------

func TestResolve_GitAuthorFallback(t *testing.T) {
	// No CODEOWNERS anywhere → use git-author output.
	root := t.TempDir()
	writeFile(t, root, modCmd+"/main.go", "")

	mm := buildModuleMap(t, map[string][]string{
		modCmd: {modCmd + "/**"},
	})

	gitLog := "alice@example.com\ncmd/main.go\n\nbob@example.com\ncmd/main.go\ncmd/main.go\n"
	runner := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(gitLog), ExitCode: 0}, nil
		},
	}

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, runner)
	// bob@example.com has 2 touches, alice has 1 → dominant is bob.
	if want := "bob@example.com"; got[modCmd] != want {
		t.Errorf("git-author dominant: got %q, want %q", got[modCmd], want)
	}
}

func TestResolve_GitAuthorFallback_Tie_AlphaFirst(t *testing.T) {
	// Equal touches: alphabetically-first author wins.
	root := t.TempDir()
	writeFile(t, root, modPkg+"/a.go", "")

	mm := buildModuleMap(t, map[string][]string{
		modPkg: {modPkg + "/**"},
	})

	gitLog := "zebra@example.com\npkg/a.go\n\nalpha@example.com\npkg/a.go\n"
	runner := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(gitLog), ExitCode: 0}, nil
		},
	}

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, runner)
	if want := "alpha@example.com"; got[modPkg] != want {
		t.Errorf("tie → alpha-first: got %q, want %q", got[modPkg], want)
	}
}

func TestResolve_GitAuthorFallback_GitFailure_EmptyMap(t *testing.T) {
	// git failure → empty map, no error.
	root := t.TempDir()

	mm := buildModuleMap(t, map[string][]string{
		modPkg: {modPkg + "/**"},
	})

	runner := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 128, Stderr: []byte("fatal: not a git repo")}, nil
		},
	}

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, runner)
	if len(got) != 0 {
		t.Errorf("git failure: expected empty map, got %v", got)
	}
}

func TestResolve_GitAuthorFallback_Subtree(t *testing.T) {
	// F2 regression: no CODEOWNERS, --root is a subtree. git log emits
	// gitRoot-relative paths; they must be stripped to the subtree-relative form
	// before module mapping. Files outside the subtree are ignored.
	gitRoot := t.TempDir()
	scanRoot := filepath.Join(gitRoot, "services", "sched")
	writeFile(t, gitRoot, "services/sched/run/job.go", "")

	mm := buildModuleMap(t, map[string][]string{
		"run": {"run/**"}, // subtree-relative glob
	})

	// git emits gitRoot-relative paths even when scoped to the subtree; an
	// out-of-subtree path must not leak into a module.
	gitLog := "carol@x.com\nservices/sched/run/job.go\nservices/sched/run/job.go\n\n" +
		"dave@x.com\nother/area/x.go\n"
	runner := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(gitLog), ExitCode: 0}, nil
		},
	}

	got, _ := ownership.Resolve(context.Background(), scanRoot, gitRoot, "services/sched", mm, runner)
	if want := "carol@x.com"; got["run"] != want {
		t.Errorf("subtree git-author: got %q, want %q", got["run"], want)
	}
}

// -----------------------------------------------------------------------
// Neither source present
// -----------------------------------------------------------------------

func TestResolve_NeitherSource_EmptyMap(t *testing.T) {
	// No CODEOWNERS, git returns empty output → empty map.
	root := t.TempDir()

	mm := buildModuleMap(t, map[string][]string{
		modPkg: {modPkg + "/**"},
	})

	runner := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(""), ExitCode: 0}, nil
		},
	}

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, runner)
	if len(got) != 0 {
		t.Errorf("no source: expected empty map, got %v", got)
	}
}

func TestResolve_NoModuleMap_EmptyMap(t *testing.T) {
	// CODEOWNERS present but no modules configured — all files unmapped.
	root := t.TempDir()
	writeFile(t, root, ".github/CODEOWNERS", "* @owner\n")
	writeFile(t, root, "main.go", "")

	mm := buildModuleMap(t, map[string][]string{}) // empty module map

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, nilRunner())
	if len(got) != 0 {
		t.Errorf("no modules: expected empty map, got %v", got)
	}
}

// -----------------------------------------------------------------------
// CODEOWNERS pattern matching edge cases
// -----------------------------------------------------------------------

func TestResolve_CodeownersCatchAll(t *testing.T) {
	// "* @owner" catches every file.
	root := t.TempDir()
	writeFile(t, root, ".github/CODEOWNERS", "* @everyone\n")
	writeFile(t, root, modPkg+"/foo.go", "")
	writeFile(t, root, modCmd+"/main.go", "")

	mm := buildModuleMap(t, map[string][]string{
		modPkg: {modPkg + "/**"},
		modCmd: {modCmd + "/**"},
	})

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, nilRunner())
	for _, mod := range []string{modPkg, modCmd} {
		if got[mod] != "@everyone" {
			t.Errorf("catch-all: %s got %q, want @everyone", mod, got[mod])
		}
	}
}

func TestResolve_CodeownersComment_Skipped(t *testing.T) {
	// Comment lines must not be parsed as rules.
	root := t.TempDir()
	writeFile(t, root, "CODEOWNERS", "# This is a comment\npkg/ @real-owner\n")
	writeFile(t, root, modPkg+"/a.go", "")

	mm := buildModuleMap(t, map[string][]string{
		modPkg: {modPkg + "/**"},
	})

	got, _ := ownership.Resolve(context.Background(), root, root, "", mm, nilRunner())
	if want := "@real-owner"; got[modPkg] != want {
		t.Errorf("comment skipped: got %q, want %q", got[modPkg], want)
	}
}

func TestResolve_ReportsSource(t *testing.T) {
	mm := buildModuleMap(t, map[string][]string{"app": {"src/**"}})

	t.Run("codeowners", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, ".github/CODEOWNERS", "src/ @team\n")
		writeFile(t, root, "src/a.go", "")
		if _, src := ownership.Resolve(context.Background(), root, root, "", mm, nilRunner()); src != ownership.SourceCodeowners {
			t.Errorf("got %q, want codeowners", src)
		}
	})

	t.Run("none when codeowners matches nothing", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, ".github/CODEOWNERS", "docs/ @team\n")
		writeFile(t, root, "src/a.go", "")
		if _, src := ownership.Resolve(context.Background(), root, root, "", mm, nilRunner()); src != ownership.SourceNone {
			t.Errorf("got %q, want none", src)
		}
	})

	t.Run("git fallback", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "src/a.go", "")
		runner := &toolrun.RunnerMock{
			RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
				return toolrun.Output{Stdout: []byte("a@x.com\nsrc/a.go\n"), ExitCode: 0}, nil
			},
		}
		if _, src := ownership.Resolve(context.Background(), root, root, "", mm, runner); src != ownership.SourceGit {
			t.Errorf("got %q, want git", src)
		}
	})
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

// writeFile creates a file (and parent dirs) with the given content.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

// nilRunner returns a RunnerMock that panics if called — used to assert the
// git path is never taken when CODEOWNERS is present.
func nilRunner() toolrun.Runner {
	return &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			panic("git runner must not be called when CODEOWNERS exists")
		},
	}
}
