package dotnet

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	targetRestoreGraph = "-t:GenerateRestoreGraphFile"
	propRestoreGraph   = "-p:RestoreGraphOutputPath="
	// dgspecSuffix names the restore graph restore writes next to
	// project.assets.json: <project file>.nuget.dgspec.json.
	dgspecSuffix = ".nuget.dgspec.json"

	reasonRestoreInputsDiffer = "restore inputs differ from the working tree's, and its reused restore outputs describe other packages"
	reasonRestoreOutputsMoved = "restore outputs were produced for another path; re-run dotnet restore in this tree"
)

// restoreViewKeys are the restore-section fields that decide which packages
// restore resolves. Paths and environment-bound fields (packagesPath,
// configFilePaths, audit settings, SDK analysis level) are left out: they
// differ between the machine that restored and this run without changing the
// packages. Project references are not compared either: the helper reads them
// from the checkout's project file, and they only lead to the referenced
// projects' entries, which are compared in turn.
var restoreViewKeys = []string{"sources", "centralPackageVersionsManagementEnabled", "CentralPackageTransitivePinningEnabled", "projectStyle"}

// packageInputFiles are compared byte for byte from a project's directory up
// to the root, lowercased. The restore graph records what they resolve to, not
// all they decide: global.json selects the SDK (its implicit packages and
// analysis level), NuGet.config carries settings beyond the source list, and
// the Directory.* files can add references and properties restore reads.
var packageInputFiles = map[string]struct{}{
	"global.json": {}, "nuget.config": {},
	"directory.packages.props": {}, "directory.build.props": {}, "directory.build.targets": {},
}

// restoreEntry is one project of a restore graph: what is compared, and the
// projects it references.
type restoreEntry struct {
	view map[string]any
	refs []string
}

// changedRestoreInputs returns the borrowed projects whose restore inputs in
// the checkout differ from those the working tree was restored with, or cannot
// be compared, split by cause: changed are the projects whose package inputs
// genuinely differ; moved are the projects whose restore outputs record a
// project path that cannot be reconciled with their path in the working tree.
// Restore inputs are the project's and every referenced project's package
// references, central package versions and sources, as NuGet's restore graph
// records them, plus the packageInputFiles. The checkout's graph is generated
// in the snapshot (`msbuild -t:GenerateRestoreGraphFile`, no restore); the
// working tree's is the one restore wrote. Only cancellation is an error: any
// other failure to build the checkout's graph leaves every borrowed project
// uncompared, so changed.
func (e *Extractor) changedRestoreInputs(ctx context.Context, snapRoot, depRoot string, targets []string, borrowed []ProjectEvaluation, mapper snapshotMapper) (changed, moved []string, err error) {
	base, err := e.restoreGraph(ctx, snapRoot, targets, mapper)
	if err != nil && ctx.Err() != nil {
		return nil, nil, err
	}
	depMapper := newSnapshotMapper(snapshot{dir: depRoot, root: depRoot}, "")
	for _, p := range borrowed {
		same, reconciled := sameRestoreGraph(p, base, depRoot, depMapper)
		switch {
		case !reconciled:
			moved = append(moved, p.Path)
		case !same || !samePackageInputFiles(snapRoot, depRoot, p.Path):
			changed = append(changed, p.Path)
		}
	}
	return changed, moved, nil
}

// restoreGraph generates the restore graph of every target in the snapshot,
// keyed by root-relative project path.
func (e *Extractor) restoreGraph(ctx context.Context, snapRoot string, targets []string, mapper snapshotMapper) (map[string]restoreEntry, error) {
	f, err := os.CreateTemp("", "archfit-restore-graph-*.json")
	if err != nil {
		return nil, err
	}
	out := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(out) }()
	graph := map[string]restoreEntry{}
	for _, target := range targets {
		res, err := e.runner.Run(ctx, toolrun.ToolCmd{
			Name:    toolDotnet,
			Args:    []string{"msbuild", filepath.Join(snapRoot, filepath.FromSlash(target)), targetRestoreGraph, propRestoreGraph + out, "-nologo"},
			Env:     msbuildEnv,
			WorkDir: snapRoot,
			Timeout: evalTimeout,
		})
		if err != nil {
			return nil, err
		}
		if res.ExitCode != 0 {
			return nil, fmt.Errorf("restore graph of %s: dotnet msbuild exited %d", target, res.ExitCode)
		}
		data, err := os.ReadFile(out) //nolint:gosec // the temp file this function created
		if err != nil {
			return nil, err
		}
		part, err := parseRestoreGraph(data, mapper)
		if err != nil {
			return nil, err
		}
		maps.Copy(graph, part)
	}
	return graph, nil
}

// parseRestoreGraph decodes a restore graph with its paths made root-relative
// by mapper, keeping the compared view of each project.
func parseRestoreGraph(data []byte, mapper snapshotMapper) (map[string]restoreEntry, error) {
	var doc struct {
		Projects map[string]struct {
			Frameworks map[string]map[string]any `json:"frameworks"`
			Restore    map[string]any            `json:"restore"`
		} `json:"projects"`
	}
	if err := json.Unmarshal([]byte(mapper.text(string(data))), &doc); err != nil {
		return nil, fmt.Errorf("parse restore graph: %w", err)
	}
	out := make(map[string]restoreEntry, len(doc.Projects))
	for project, p := range doc.Projects {
		for _, fw := range p.Frameworks {
			delete(fw, "runtimeIdentifierGraphPath") // the SDK's install path
		}
		restore := map[string]any{}
		for _, k := range restoreViewKeys {
			restore[k] = p.Restore[k]
		}
		var refs []string
		fws, _ := p.Restore["frameworks"].(map[string]any)
		for _, fw := range fws {
			fwMap, _ := fw.(map[string]any)
			projectRefs, _ := fwMap["projectReferences"].(map[string]any)
			for ref := range projectRefs {
				refs = append(refs, filepath.ToSlash(ref))
			}
		}
		out[filepath.ToSlash(project)] = restoreEntry{view: map[string]any{"frameworks": p.Frameworks, "restore": restore}, refs: refs}
	}
	return out, nil
}

// sameRestoreGraph compares p and every project it references, as the
// checkout's graph has them, with the working tree's graph for p. reconciled
// is false when the working tree's restore outputs record a project path that
// is not p's, so they were produced under another root and cannot be trusted.
func sameRestoreGraph(p ProjectEvaluation, base map[string]restoreEntry, depRoot string, mapper snapshotMapper) (same, reconciled bool) {
	file := filepath.Join(depRoot, filepath.FromSlash(p.RestoreDir), path.Base(p.Path)+dgspecSuffix)
	data, err := os.ReadFile(file) //nolint:gosec // a restore output in the working tree
	if err != nil {
		return false, true
	}
	mapper, reconciled = dgspecMapper(data, p.Path, mapper)
	if !reconciled {
		return false, false
	}
	head, err := parseRestoreGraph(data, mapper)
	if err != nil {
		return false, true
	}
	seen := map[string]bool{}
	var walk func(project string) bool
	walk = func(project string) bool {
		if seen[project] {
			return true
		}
		seen[project] = true
		b, inBase := base[project]
		h, inHead := head[project]
		if !inBase || !inHead || !reflect.DeepEqual(b.view, h.view) {
			return false
		}
		for _, ref := range b.refs {
			if !walk(ref) {
				return false
			}
		}
		return true
	}
	return walk(p.Path), true
}

// dgspecMapper returns the mapper that makes a working tree dgspec's paths
// root-relative, and whether the dgspec belongs to project. NuGet records the
// absolute path the project was restored at, and a copied tree (cp -a, a CI
// cache) keeps the original. The top-level restore entry names that project;
// the suffix it shares with project's path is the project's path past the
// recorded root, so what precedes it is that root.
func dgspecMapper(data []byte, project string, mapper snapshotMapper) (snapshotMapper, bool) {
	var doc struct {
		Restore  map[string]json.RawMessage `json:"restore"`
		Projects map[string]json.RawMessage `json:"projects"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return mapper, true // parseRestoreGraph reports the malformed graph as an input mismatch
	}
	// The top-level restore entry names the project this graph was generated
	// for; the projects map is the fallback for a graph that omits it.
	for _, keys := range [][]string{slices.Sorted(maps.Keys(doc.Restore)), slices.Sorted(maps.Keys(doc.Projects))} {
		for _, key := range keys {
			root, ok := strings.CutSuffix(filepath.ToSlash(key), "/"+filepath.ToSlash(project))
			if ok && root != "" {
				return mapper.addPrefix(root, ""), true
			}
		}
	}
	return mapper, false
}

// samePackageInputFiles reports whether the package input files from the
// project's directory up to the root match in both trees.
func samePackageInputFiles(snapRoot, depRoot, project string) bool {
	for dir := path.Dir(project); ; dir = path.Dir(dir) {
		local := filepath.FromSlash(dir)
		if !maps.Equal(packageInputs(filepath.Join(snapRoot, local)), packageInputs(filepath.Join(depRoot, local))) {
			return false
		}
		if dir == "." {
			return true
		}
	}
}

// packageInputs maps the lowercased name of each package input file in dir to
// its content.
func packageInputs(dir string) map[string]string {
	entries, _ := os.ReadDir(dir)
	out := map[string]string{}
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if _, ok := packageInputFiles[name]; !ok || !e.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // walking the scan root by design
		if err != nil {
			data = []byte("unreadable: " + err.Error())
		}
		out[name] = string(data)
	}
	return out
}
