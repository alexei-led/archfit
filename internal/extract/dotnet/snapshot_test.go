package dotnet_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/extract/dotnet"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// subtreeDir is the scan root's directory below the repository root in the
// subtree cases.
const subtreeDir = "src"

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

// linksEscaping names every link under dir that resolves outside it: the
// snapshot must never carry one, because the design-time build writes through
// links.
func linksEscaping(t *testing.T, dir string) []string {
	t.Helper()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		link, err := os.Readlink(p)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(p), link)
		}
		link = filepath.Clean(link)
		if resolved, err := filepath.EvalSymlinks(link); err == nil {
			link = resolved
		}
		if rel, err := filepath.Rel(dir, link); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			out = append(out, p+" -> "+link)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestSnapshot_ExternalSymlinkIsDroppedAndDisclosed: a link whose target lies
// outside the scan root must not be reproduced in the snapshot. MSBuild then
// creates a real directory inside it, and the run names the dropped link, so
// the project does not read as silently complete.
func TestSnapshot_ExternalSymlinkIsDroppedAndDisclosed(t *testing.T) {
	assertDropped := func(t *testing.T, e *dotnet.Extractor, link string) {
		t.Helper()
		cov := e.LastTypeGraphCoverage()
		if cov.Status != evidence.StatusPartial || cov.UnresolvedInputsMissing == 0 {
			t.Errorf("roslyn coverage = %+v, want partial with missing inputs", cov)
		}
		if !strings.Contains(cov.Reason, "not reproduced in the snapshot: "+link) {
			t.Errorf("roslyn reason %q does not disclose the dropped link %q", cov.Reason, link)
		}
		if p := typeProject(t, e.LastTypeGraph(), domainProj); p.Status != dotnet.TypeGraphPartial {
			t.Errorf("project = %+v, want partial: the snapshot does not have the link's target", p)
		}
	}

	t.Run("project obj link to an absolute outside directory", func(t *testing.T) {
		root := tree(t, domainProj)
		outside := t.TempDir()
		writeFile(t, outside, "existing.txt", "keep")
		if err := os.Symlink(outside, filepath.Join(root, "Domain", "obj")); err != nil {
			t.Fatal(err)
		}
		beforeRoot, beforeOutside := treeState(t, root), treeState(t, outside)
		installHelper(t)
		fake := newFake(t, root, map[string]reply{domainProj: ok(t, root, "domain.json")})
		fake.helper = fakeHelper(func(_ context.Context, snap string) (toolrun.Output, error) {
			if escaping := linksEscaping(t, snap); len(escaping) != 0 {
				t.Errorf("snapshot links escape it: %v", escaping)
			}
			if _, err := os.Lstat(filepath.Join(snap, "Domain", "obj")); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("snapshot still has the outside link (err %v)", err)
			}
			// MSBuild creates obj/ as a real directory and writes generated files there.
			writeFile(t, filepath.Join(snap, "Domain"), "obj/Domain.GlobalUsings.g.cs", "// generated")
			return toolrun.Output{Stdout: []byte(domainOnlyReport)}, nil
		})
		e := dotnet.New(fake, autoCfg(), nil)
		if _, _, err := e.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
			t.Fatal(err)
		}
		assertDropped(t, e, "Domain/obj")
		if after := treeState(t, outside); !mapsEqual(after, beforeOutside) {
			t.Errorf("a write through the link reached the outside directory:\nbefore %v\nafter  %v", beforeOutside, after)
		}
		if after := treeState(t, root); !mapsEqual(after, beforeRoot) {
			t.Errorf("the original root changed:\nbefore %v\nafter  %v", beforeRoot, after)
		}
	})

	t.Run("subtree relative link above the scan root", func(t *testing.T) {
		repo := t.TempDir()
		root := filepath.Join(repo, "src")
		writeFile(t, root, domainProj, "")
		writeFile(t, repo, "artifacts/obj/existing.txt", "keep")
		if err := os.Symlink(filepath.Join("..", "..", "artifacts", "obj"), filepath.Join(root, "Domain", "obj")); err != nil {
			t.Fatal(err)
		}
		before := treeState(t, repo)
		installHelper(t)
		fake := newFake(t, root, map[string]reply{domainProj: ok(t, root, "domain.json")})
		fake.helper = fakeHelper(func(_ context.Context, snap string) (toolrun.Output, error) {
			snapshotDir := filepath.Dir(snap)
			if escaping := linksEscaping(t, snapshotDir); len(escaping) != 0 {
				t.Errorf("snapshot links escape it: %v", escaping)
			}
			if _, err := os.Lstat(filepath.Join(snap, "Domain", "obj")); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("snapshot still has the outside link (err %v)", err)
			}
			writeFile(t, filepath.Join(snap, "Domain"), "obj/Domain.GlobalUsings.g.cs", "// generated")
			return toolrun.Output{Stdout: []byte(domainOnlyReport)}, nil
		})
		e := dotnet.New(fake, autoCfg(), nil)
		if _, _, err := e.Extract(context.Background(), scope.Scope{Root: root, GitRoot: repo, SubtreePrefix: subtreeDir}); err != nil {
			t.Fatal(err)
		}
		assertDropped(t, e, "Domain/obj")
		if after := treeState(t, repo); !mapsEqual(after, before) {
			t.Errorf("the original tree changed:\nbefore %v\nafter  %v", before, after)
		}
	})
}

// restoreProject is one project of a fake restore graph.
type restoreProject struct {
	version string   // the central version of the one package it references
	refs    []string // referenced projects
}

// restoreGraphJSON renders a NuGet restore graph of projects under root. env
// stands for the fields that depend on the machine, never on the inputs.
func restoreGraphJSON(root, env string, projects map[string]restoreProject) string {
	abs := func(p string) string { return filepath.ToSlash(filepath.Join(root, p)) }
	entries := make([]string, 0, len(projects))
	for path, p := range projects {
		refs := make([]string, 0, len(p.refs))
		for _, r := range p.refs {
			refs = append(refs, fmt.Sprintf("%q:{\"projectPath\":%q}", abs(r), abs(r)))
		}
		entries = append(entries, fmt.Sprintf(`%q:{"frameworks":{"net10.0":{"dependencies":{"Pkg":{"target":"Package","version":"[1.0.0, )","versionCentrallyManaged":true}},`+
			`"centralPackageVersions":{"Pkg":%q},"runtimeIdentifierGraphPath":%q}},`+
			`"restore":{"projectPath":%q,"packagesPath":%q,"sources":{"https://api.nuget.org/v3/index.json":{}},`+
			`"centralPackageVersionsManagementEnabled":true,"frameworks":{"net10.0":{"projectReferences":{%s}}}}}`,
			abs(path), p.version, env+"/sdk/PortableRuntimeIdentifierGraph.json", abs(path), env+"/packages", strings.Join(refs, ",")))
	}
	return `{"format":1,"projects":{` + strings.Join(entries, ",") + `}}`
}

// closure returns project and every project it references in graph.
func closure(graph map[string]restoreProject, project string) map[string]restoreProject {
	out := map[string]restoreProject{}
	var walk func(string)
	walk = func(p string) {
		if _, seen := out[p]; seen {
			return
		}
		out[p] = graph[p]
		for _, r := range graph[p].refs {
			walk(r)
		}
	}
	walk(project)
	return out
}

// answerRestoreGraph answers `msbuild -t:GenerateRestoreGraphFile` with graph's
// closure of the target, written where the call asks, and passes every other
// call to next.
func answerRestoreGraph(t *testing.T, next func(context.Context, toolrun.ToolCmd) (toolrun.Output, error), graph map[string]restoreProject) func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) {
	return func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		if !slices.Contains(cmd.Args, "-t:GenerateRestoreGraphFile") {
			return next(ctx, cmd)
		}
		target, err := filepath.Rel(cmd.WorkDir, cmd.Args[1])
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range cmd.Args {
			if out, ok := strings.CutPrefix(a, "-p:RestoreGraphOutputPath="); ok {
				writeFile(t, filepath.Dir(out), filepath.Base(out), restoreGraphJSON(cmd.WorkDir, "/base-machine", closure(graph, filepath.ToSlash(target))))
			}
		}
		return toolrun.Output{}, nil
	}
}

// TestSnapshot_CheckoutBorrowsWorkingTreeRestoreOutputs: a checkout (the --base
// worktree) holds no gitignored restore outputs, so the snapshot takes them
// from the working tree, project by project, at the path MSBuild evaluates in
// the checkout. A project whose restore inputs differ from the ones the working
// tree restored with is partial and named, and so is one whose restore outputs
// record a project path that cannot be matched to the project's. Restore
// outputs produced under another root but naming the same project are
// reconciled with the checkout's. Neither tree is written.
func TestSnapshot_CheckoutBorrowsWorkingTreeRestoreOutputs(t *testing.T) {
	const (
		restored   = "Restored/Restored.csproj"     // restored in the working tree, same inputs
		artifacts  = "Artifacts/Artifacts.csproj"   // UseArtifactsOutput; its central version was bumped
		consumer   = "Consumer/Consumer.csproj"     // same own inputs, references Artifacts
		sourced    = "Sourced/Sourced.csproj"       // a NuGet.config only the working tree has
		propped    = "Propped/Propped.csproj"       // a Directory.Build.props that differs
		moved      = "Moved/Moved.csproj"           // restored at another root, same inputs
		foreign    = "Foreign/Foreign.csproj"       // restore outputs record a path that is not its own
		unrestored = "Unrestored/Unrestored.csproj" // never restored in the working tree
		baseOnly   = "BaseOnly/BaseOnly.csproj"     // gone from the working tree
		committed  = "Committed/Committed.csproj"   // the checkout tracks its own restore outputs
		outside    = "Outside/Outside.csproj"       // restore dir outside the root
		pkgVersion = "1.0.0"
	)
	projects := []string{restored, artifacts, consumer, sourced, propped, moved, foreign, unrestored, baseOnly, committed, outside}
	restoreDirs := map[string]string{
		restored: "Restored/obj", artifacts: "artifacts/obj/Artifacts", consumer: "Consumer/obj", sourced: "Sourced/obj", propped: "Propped/obj",
		moved: "Moved/obj", foreign: "Foreign/obj",
		unrestored: "Unrestored/obj", baseOnly: "BaseOnly/obj", committed: "Committed/obj",
	}
	baseGraph := map[string]restoreProject{
		restored: {version: pkgVersion}, artifacts: {version: pkgVersion}, consumer: {version: pkgVersion, refs: []string{artifacts}},
		sourced: {version: pkgVersion}, propped: {version: pkgVersion}, moved: {version: pkgVersion}, foreign: {version: pkgVersion},
	}
	headGraph := maps.Clone(baseGraph)
	headGraph[artifacts] = restoreProject{version: "2.0.0"}

	work := tree(t, restored, artifacts, consumer, sourced, propped, moved, foreign, unrestored, committed, outside)
	writeFile(t, work, "global.json", `{"sdk":{"version":"10.0.100"}}`)
	writeFile(t, work, "Sourced/NuGet.config", "<configuration />")
	writeFile(t, work, "Propped/Directory.Build.props", "<Project />")
	for _, p := range []string{restored, artifacts, consumer, sourced, propped} {
		dir := restoreDirs[p]
		writeFile(t, work, dir+"/project.assets.json", "work assets")
		writeFile(t, work, dir+"/"+filepath.Base(p)+".nuget.dgspec.json", restoreGraphJSON(work, "/work-machine", closure(headGraph, p)))
	}
	// Restore outputs of a tree copied from elsewhere still name the original root.
	for _, p := range []string{moved, foreign} {
		writeFile(t, work, restoreDirs[p]+"/project.assets.json", "work assets")
	}
	writeFile(t, work, restoreDirs[moved]+"/Moved.csproj.nuget.dgspec.json",
		restoreGraphJSON("/another-tree", "/work-machine", closure(headGraph, moved)))
	writeFile(t, work, restoreDirs[foreign]+"/Foreign.csproj.nuget.dgspec.json",
		restoreGraphJSON("/another-tree", "/work-machine", map[string]restoreProject{"Elsewhere/Foreign.csproj": {version: pkgVersion}}))
	writeFile(t, work, "Restored/obj/Restored.csproj.nuget.g.props", "<Project />")
	writeFile(t, work, "Restored/obj/Debug/net10.0/Restored.dll", "build output")
	writeFile(t, work, "BaseOnly/obj/project.assets.json", "stale assets of a removed project")
	writeFile(t, work, "Committed/obj/project.assets.json", "work assets")

	checkout := tree(t, projects...)
	writeFile(t, checkout, "global.json", `{"sdk":{"version":"10.0.100"}}`)
	writeFile(t, checkout, "Committed/obj/project.assets.json", "checkout assets")
	writeFile(t, checkout, "Propped/Directory.Build.props", `<Project><ItemGroup><PackageReference Include="Other" /></ItemGroup></Project>`)

	entries := make([]string, 0, len(projects))
	replies := map[string]reply{}
	for _, p := range projects {
		ext := "/elsewhere/obj/"
		if dir, ok := restoreDirs[p]; ok {
			ext = filepath.ToSlash(filepath.Join(checkout, dir)) + "/"
		}
		replies[p] = reply{stdout: `{"Properties":{"TargetFramework":"net10.0","MSBuildProjectExtensionsPath":"` + ext + `"},"Items":{}}`}
		entries = append(entries, `{"path":"`+p+`","tfm":"net10.0","status":"complete","used_projects":[],"files":[]}`)
	}
	report := `{"schema_version":2,"skipped_projects":[],"projects":[` + strings.Join(entries, ",") + `]}`

	tests := []struct {
		name           string
		dependencyRoot string
		want           map[string]string // snapshot file -> content; "" means absent
		wantReason     string
		wantPartial    []string
	}{
		{
			name:           "checkout with a working tree",
			dependencyRoot: work,
			want: map[string]string{
				"Restored/obj/project.assets.json":            "work assets",
				"Restored/obj/Restored.csproj.nuget.g.props":  "<Project />",
				"Restored/obj/Debug/net10.0/Restored.dll":     "",
				"artifacts/obj/Artifacts/project.assets.json": "work assets",
				"Unrestored/obj/project.assets.json":          "",
				"BaseOnly/obj/project.assets.json":            "",
				"Committed/obj/project.assets.json":           "checkout assets",
			},
			wantReason: "no restore outputs for 2 projects in the checkout or at the same path in the working tree; " +
				"run dotnet restore in the working tree: BaseOnly/BaseOnly.csproj, Unrestored/Unrestored.csproj; " +
				"restore outputs were produced for another path, so they are partial; " +
				"run dotnet restore in the working tree: Foreign/Foreign.csproj; " +
				"restore inputs (package references, central versions, sources, global.json, NuGet.config, Directory.Build.props/.targets, " +
				"Directory.Packages.props) differ from the working tree's for 4 projects, so they are partial: " +
				"Artifacts/Artifacts.csproj, Consumer/Consumer.csproj, Propped/Propped.csproj, Sourced/Sourced.csproj; " +
				"restore outputs reused from the working tree for 7 projects",
			wantPartial: []string{artifacts, consumer, foreign, propped, sourced},
		},
		{
			name: "working tree scan",
			want: map[string]string{
				"Restored/obj/project.assets.json":            "",
				"artifacts/obj/Artifacts/project.assets.json": "",
				"Committed/obj/project.assets.json":           "checkout assets",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workBefore, checkoutBefore := treeState(t, work), treeState(t, checkout)
			installHelper(t)
			fake := newFake(t, checkout, replies)
			fake.RunFunc = answerRestoreGraph(t, fake.RunFunc, baseGraph)
			fake.helper = fakeHelper(func(_ context.Context, snap string) (toolrun.Output, error) {
				for rel, want := range tt.want {
					data, err := os.ReadFile(filepath.Join(snap, filepath.FromSlash(rel))) //nolint:gosec // test snapshot path
					if got := string(data); (err == nil) != (want != "") || got != want {
						t.Errorf("snapshot %s = %q (err %v), want %q", rel, got, err, want)
					}
				}
				return toolrun.Output{Stdout: []byte(report)}, nil
			})
			e := dotnet.New(fake, autoCfg(), nil)
			if _, _, err := e.Extract(context.Background(), scope.Scope{Root: checkout, DependencyRoot: tt.dependencyRoot}); err != nil {
				t.Fatalf("Extract: %v", err)
			}
			if len(fake.helperCalls) != 2 {
				t.Fatalf("helper calls = %d, want --version and one run", len(fake.helperCalls))
			}
			cov := e.LastTypeGraphCoverage()
			if tt.wantReason == "" && cov.Reason != "" || !strings.HasPrefix(cov.Reason, tt.wantReason) {
				t.Errorf("roslyn reason = %q, want it to start with %q", cov.Reason, tt.wantReason)
			}
			var partial []string
			for _, p := range e.LastTypeGraph().Projects {
				if p.Status == dotnet.TypeGraphPartial {
					partial = append(partial, p.Path)
				}
			}
			slices.Sort(partial)
			if !slices.Equal(partial, tt.wantPartial) {
				t.Errorf("partial projects = %v, want %v", partial, tt.wantPartial)
			}
			if wantStatus := map[bool]string{true: "partial", false: "ok"}[len(tt.wantPartial) > 0]; cov.Status != wantStatus {
				t.Errorf("roslyn status = %q, want %q", cov.Status, wantStatus)
			}
			if after := treeState(t, work); !mapsEqual(after, workBefore) {
				t.Errorf("the working tree changed:\nbefore %v\nafter  %v", workBefore, after)
			}
			if after := treeState(t, checkout); !mapsEqual(after, checkoutBefore) {
				t.Errorf("the checkout changed:\nbefore %v\nafter  %v", checkoutBefore, after)
			}
		})
	}

	// A checkout whose project directory is a link outside the scan root has no
	// project the snapshot can load, so it must not borrow restore outputs for
	// it either: the working tree's assets would describe a tree the helper
	// never sees, and copying them must not write through the link.
	t.Run("symlinked project directory outside the snapshot", func(t *testing.T) {
		outside := t.TempDir()
		writeFile(t, outside, "Proj/Domain.csproj", "")
		work := tree(t, domainProj)
		writeFile(t, work, "Domain/obj/project.assets.json", "work assets")
		checkout := t.TempDir()
		if err := os.Symlink(filepath.Join(outside, "Proj"), filepath.Join(checkout, "Domain")); err != nil {
			t.Fatal(err)
		}
		beforeOutside, beforeCheckout := treeState(t, outside), treeState(t, checkout)
		installHelper(t)
		fake := newFake(t, checkout, map[string]reply{domainProj: {
			stdout: `{"Properties":{"TargetFramework":"net10.0","MSBuildProjectExtensionsPath":"` +
				filepath.ToSlash(filepath.Join(checkout, "Domain", "obj")) + `"},"Items":{}}`,
		}})
		fake.helper = fakeHelper(func(_ context.Context, snap string) (toolrun.Output, error) {
			if _, err := os.Lstat(filepath.Join(snap, "Domain")); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("the snapshot has a Domain entry (err %v): the project link was reproduced or written through", err)
			}
			return toolrun.Output{Stdout: []byte(domainOnlyReport)}, nil
		})
		e := dotnet.New(fake, autoCfg(), []string{domainProj})
		if _, _, err := e.Extract(context.Background(), scope.Scope{Root: checkout, DependencyRoot: work}); err != nil {
			t.Fatalf("Extract: %v", err)
		}
		cov := e.LastTypeGraphCoverage()
		if !strings.Contains(cov.Reason, "no restore outputs for 1 project") || !strings.Contains(cov.Reason, domainProj) {
			t.Errorf("roslyn reason %q does not name the project left without restore outputs", cov.Reason)
		}
		if after := treeState(t, outside); !mapsEqual(after, beforeOutside) {
			t.Errorf("a write through the project link reached the outside directory:\nbefore %v\nafter  %v", beforeOutside, after)
		}
		if after := treeState(t, checkout); !mapsEqual(after, beforeCheckout) {
			t.Errorf("the checkout changed:\nbefore %v\nafter  %v", beforeCheckout, after)
		}
	})
}
