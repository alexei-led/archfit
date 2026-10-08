package initcfg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// goListPkg mirrors the subset of `go list -json` output that we need.
type goListPkg struct {
	ImportPath string
	Dir        string
	Imports    []string
	Module     *struct {
		Path string
	}
}

// goMemberPackages is one Go member's `go list` result: the member directory
// relative to the discovery root ("." for the root), its module path, and its
// packages.
type goMemberPackages struct {
	rel     string
	modPath string
	pkgs    []goListPkg
}

// discoverGo runs `go list -e -json ./...` in every Go member directory the
// extractor would load and groups the packages into candidate modules. Members
// come from the extractor's own member discovery (Presence.GoMembers), so a
// go.work monorepo with no root go.mod is enumerated member by member instead
// of being skipped. Returns modules, inter-module edges, the first member's
// module path, and any error.
func discoverGo(ctx context.Context, root string, runner toolrun.Runner, members []string, goWorkOff bool) ([]ModuleDef, []ModuleEdge, string, error) {
	var env []string
	if goWorkOff {
		// The extractor ignores a go.work that names no member under root; the
		// toolchain must be told the same or `go list` refuses the module.
		env = []string{"GOWORK=off"}
	}
	listed := make([]goMemberPackages, 0, len(members))
	for _, dir := range members {
		rel, err := filepath.Rel(root, dir)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		m, err := goListMember(ctx, runner, dir, env)
		if err != nil {
			return nil, nil, "", err
		}
		m.rel = filepath.ToSlash(rel)
		listed = append(listed, m)
	}
	if len(listed) == 0 {
		return nil, nil, "", nil
	}

	// segments groups package paths (relative to root) by their 2-segment key.
	segments := make(map[string][]string)
	// pkgImports maps each package's root-relative path to the root-relative
	// paths of the first-party packages it imports. Used to derive module edges.
	pkgImports := make(map[string][]string)
	for _, m := range listed {
		for _, pkg := range m.pkgs {
			rel := memberRelPath(m.rel, stripPrefix(pkg.ImportPath, m.modPath))
			key := groupKey(rel)
			segments[key] = append(segments[key], rel)
			var intraImports []string
			for _, imp := range pkg.Imports {
				if impRel, ok := firstPartyImport(imp, listed); ok {
					intraImports = append(intraImports, impRel)
				}
			}
			if len(intraImports) > 0 {
				pkgImports[rel] = intraImports
			}
		}
	}

	mods := buildGoModules(segments)
	edges := buildGoEdges(segments, pkgImports)
	return mods, edges, listed[0].modPath, nil
}

// goListMember runs `go list -e -json ./...` in one member directory and
// returns its packages and module path. -e keeps a package that fails to load
// (an unresolvable import, a syntax error) in the listing instead of failing
// the run: onboarding proposes modules from directory structure, and one broken
// package in one go.work member must not abort init or update for the tree.
func goListMember(ctx context.Context, runner toolrun.Runner, dir string, env []string) (goMemberPackages, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    "go",
		Args:    []string{"list", "-e", "-json", "./..."},
		Env:     env,
		WorkDir: dir,
	})
	if err != nil {
		return goMemberPackages{}, fmt.Errorf("initcfg: go list: %w", err)
	}
	if out.ExitCode != 0 {
		return goMemberPackages{}, fmt.Errorf("initcfg: go list exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	var m goMemberPackages
	dec := json.NewDecoder(bytes.NewReader(out.Stdout))
	for {
		var pkg goListPkg
		if err := dec.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			return goMemberPackages{}, fmt.Errorf("initcfg: parse go list output: %w", err)
		}
		if pkg.Module != nil && pkg.Module.Path != "" && m.modPath == "" {
			m.modPath = pkg.Module.Path
		}
		m.pkgs = append(m.pkgs, pkg)
	}
	return m, nil
}

// memberRelPath joins a member's root-relative directory with a package path
// relative to that member. The root member's own root package is ".".
func memberRelPath(memberRel, pkgRel string) string {
	switch {
	case memberRel == "." && pkgRel == "":
		return "."
	case memberRel == ".":
		return pkgRel
	case pkgRel == "":
		return memberRel
	default:
		return memberRel + "/" + pkgRel
	}
}

// firstPartyImport resolves an import path to the root-relative path of a
// package in one of the listed members, choosing the longest matching module
// path so a nested module wins over its parent.
func firstPartyImport(imp string, members []goMemberPackages) (string, bool) {
	best := -1
	for i, m := range members {
		if m.modPath == "" || (imp != m.modPath && !strings.HasPrefix(imp, m.modPath+"/")) {
			continue
		}
		if best < 0 || len(m.modPath) > len(members[best].modPath) {
			best = i
		}
	}
	if best < 0 {
		return "", false
	}
	return memberRelPath(members[best].rel, stripPrefix(imp, members[best].modPath)), true
}

// buildGoEdges derives module-level dependency edges from the package-level
// import graph. An edge (fromMod → toMod) is emitted when a package in fromMod
// imports a package in toMod and the two modules are distinct.
// The result is sorted and deduplicated for determinism.
func buildGoEdges(segments map[string][]string, pkgImports map[string][]string) []ModuleEdge {
	// Build a map from package relative path → module key.
	pkgToKey := make(map[string]string, len(pkgImports))
	for key, pkgs := range segments {
		for _, p := range pkgs {
			pkgToKey[p] = key
		}
	}

	seen := make(map[string]struct{})
	var edges []ModuleEdge
	for fromPkg, imports := range pkgImports {
		fromKey := pkgToKey[fromPkg]
		if fromKey == "" || fromKey == "." {
			continue
		}
		fromMod := moduleNameFromKey(fromKey)
		for _, toPkg := range imports {
			toKey := pkgToKey[toPkg]
			if toKey == "" || toKey == "." || toKey == fromKey {
				continue
			}
			toMod := moduleNameFromKey(toKey)
			edgeKey := fromMod + "\x00" + toMod
			if _, dup := seen[edgeKey]; dup {
				continue
			}
			seen[edgeKey] = struct{}{}
			edges = append(edges, ModuleEdge{From: fromMod, To: toMod})
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	return edges
}

// stripPrefix removes the module path prefix from an import path.
// e.g. "github.com/foo/bar/pkg/a" with modPath "github.com/foo/bar" → "pkg/a".
func stripPrefix(importPath, modPath string) string {
	if modPath == "" {
		return importPath
	}
	trimmed := strings.TrimPrefix(importPath, modPath)
	return strings.TrimPrefix(trimmed, "/")
}

// groupKey returns the first 2 path segments of a relative package path.
// e.g. "internal/extract/golang" → "internal/extract"
// e.g. "cmd/archfit" → "cmd/archfit"
// e.g. "." → "."
func groupKey(rel string) string {
	if rel == "." || rel == "" {
		return "."
	}
	parts := strings.SplitN(rel, "/", 3)
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}

// buildGoModules converts the segments map into sorted ModuleDef slice.
//
// It assigns no layer. Directory names do not prove an architectural layer,
// and a guess that puts domain, application and adapters in one layer writes a
// direction rule that can never fire; Render asks the owner to declare layers
// instead.
func buildGoModules(segments map[string][]string) []ModuleDef {
	// Sort keys for determinism.
	keys := make([]string, 0, len(segments))
	for k := range segments {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var mods []ModuleDef
	for _, key := range keys {
		if key == "." {
			// Skip the root package as a standalone module entry.
			continue
		}
		pkgs := append([]string(nil), segments[key]...)
		sort.Strings(pkgs)

		// No public: entry. Under bc_score.v7 a public: target is the integration
		// contract, so an entry claims a published surface; discovery cannot know
		// which packages are one, and guessing the package itself turned every
		// cross-module call into contract coupling. The owner declares surfaces.
		mods = append(mods, ModuleDef{
			Name: moduleNameFromKey(key),
			// Doublestar glob (classify/extractor node paths use "/"-separated
			// package and file paths). "key/**" matches the package node "key" and
			// its files. NOT the go-list "key/..." form, which doublestar does not
			// match.
			Paths:   []string{key + "/**"},
			Sources: pkgs,
		})
	}
	return mods
}

// moduleNameFromKey produces a short name from a 2-segment path key.
// "internal/extract" → "extract", "cmd/archfit" → "cmd_archfit"
func moduleNameFromKey(key string) string {
	parts := strings.Split(key, "/")
	if len(parts) == 2 && parts[0] == "internal" {
		return parts[1]
	}
	return strings.Join(parts, "_")
}
