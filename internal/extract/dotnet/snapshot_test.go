package dotnet_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/extract/dotnet"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// domainOnlyReport reports Domain complete with one file and no edges.
const domainOnlyReport = `{"schema_version":2,"skipped_projects":[],"projects":[
 {"path":"Domain/Domain.csproj","tfm":"net10.0","status":"complete","used_projects":[],
  "files":[{"path":"Domain/Domain.cs","edges":[]}]}]}`

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func TestSnapshot_SubtreeKeepsAncestorBuildInputs(t *testing.T) {
	outer := t.TempDir()
	writeFile(t, outer, "global.json", "{}")
	repo := filepath.Join(outer, "repo")
	for _, f := range []string{"Directory.Build.props", "Directory.Build.targets", "nuget.config", "src/Directory.Packages.props", "README.md", "other/Other.cs", "src/sibling/Sib.cs"} {
		writeFile(t, repo, f, "<Project />")
	}
	root := filepath.Join(repo, "src", "app")
	writeFile(t, root, domainProj, "")
	before := treeState(t, outer)
	installHelper(t)
	fake := newFake(t, root, map[string]reply{domainProj: ok(t, root, "domain.json")})
	var top string
	fake.helper = fakeHelper(func(_ context.Context, snap string) (toolrun.Output, error) {
		top = filepath.Dir(filepath.Dir(snap))
		if !strings.HasSuffix(filepath.ToSlash(snap), "/src/app") {
			t.Errorf("helper root %s is not the scan root's place below the repository root", snap)
		}
		for rel, want := range map[string]bool{
			"Directory.Build.props": true, "Directory.Build.targets": true, "nuget.config": true,
			"src/Directory.Packages.props": true, "src/app/" + domainProj: true,
			"README.md": false, "other": false, "src/sibling": false, "../global.json": false,
		} {
			if got := exists(filepath.Join(top, rel)); got != want {
				t.Errorf("snapshot has %s = %v, want %v", rel, got, want)
			}
		}
		writeFile(t, top, "Directory.Build.props", "<!-- a build may touch the copy -->")
		report := strings.Replace(domainOnlyReport, `"status":"complete"`, `"status":"partial","reason":"`+filepath.ToSlash(top)+`/Directory.Build.props(1,1): error"`, 1)
		return toolrun.Output{Stdout: []byte(report)}, nil
	})
	e := dotnet.New(fake, autoCfg(), nil)
	facts, _, err := e.Extract(context.Background(), scope.Scope{Root: root, GitRoot: repo, SubtreePrefix: "src/app"})
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(nodeIDs(facts), "file:Domain/Domain.cs") {
		t.Errorf("nodes = %v, want root-relative file paths", nodeIDs(facts))
	}
	cov := e.LastTypeGraphCoverage()
	if !strings.Contains(cov.Reason, filepath.ToSlash(repo)+"/Directory.Build.props(1,1)") {
		t.Errorf("roslyn reason %q does not map the ancestor path back to the repository", cov.Reason)
	}
	if !strings.Contains(cov.Reason, "build inputs above the copied tree are not in the snapshot: "+filepath.Join(outer, "global.json")) {
		t.Errorf("roslyn reason %q does not disclose the build input above the repository root", cov.Reason)
	}
	if top == "" || exists(top) {
		t.Errorf("snapshot %q was not removed", top)
	}
	if after := treeState(t, outer); !mapsEqual(after, before) {
		t.Errorf("original tree changed:\nbefore %v\nafter  %v", before, after)
	}
}

func TestSnapshot_TempDirInsideRootIsNotCopied(t *testing.T) {
	root := tree(t, domainProj)
	installHelper(t)
	tmp := filepath.Join(root, "tmp")
	if err := os.Mkdir(tmp, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	fake := newFake(t, root, map[string]reply{domainProj: ok(t, root, "domain.json")})
	ran := false
	fake.helper = fakeHelper(func(_ context.Context, snap string) (toolrun.Output, error) {
		ran = true
		nested, _ := filepath.Glob(filepath.Join(snap, "tmp", "archfit-dotnet-snapshot-*"))
		if len(nested) != 0 {
			t.Errorf("the snapshot copied itself: %v", nested)
		}
		return toolrun.Output{Stdout: []byte(domainOnlyReport)}, nil
	})
	e := dotnet.New(fake, autoCfg(), nil)
	if _, _, err := e.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatalf("helper never ran: %+v", e.LastTypeGraphCoverage())
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Errorf("left in TMPDIR: %v", left)
	}
}

func TestSnapshot_SymlinkThroughAnAliasStaysInTheSnapshot(t *testing.T) {
	root, err := filepath.EvalSymlinks(tree(t, domainProj, "shared-obj/keep.txt"))
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(alias, "shared-obj"), filepath.Join(root, "Domain", "obj")); err != nil {
		t.Fatal(err)
	}
	before := treeState(t, root)
	installHelper(t)
	fake := newFake(t, root, map[string]reply{domainProj: ok(t, root, "domain.json")})
	var snapshot string
	fake.helper = fakeHelper(func(_ context.Context, snap string) (toolrun.Output, error) {
		snapshot = snap
		// The design-time build writes generated files under obj/.
		writeFile(t, filepath.Join(snap, "Domain", "obj"), "Domain.GlobalUsings.g.cs", "// generated")
		return toolrun.Output{Stdout: []byte(domainOnlyReport)}, nil
	})
	if _, _, err := dotnet.New(fake, autoCfg(), nil).Extract(context.Background(), scope.Scope{Root: root}); err != nil {
		t.Fatal(err)
	}
	if snapshot == "" {
		t.Fatal("helper never ran")
	}
	if after := treeState(t, root); !mapsEqual(after, before) {
		t.Errorf("a write through the aliased link reached the original:\nbefore %v\nafter  %v", before, after)
	}
	if _, err := os.Stat(snapshot); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("snapshot %s still exists (err %v)", snapshot, err)
	}
}
