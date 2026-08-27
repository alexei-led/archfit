// Package deployunit detects deploy units in a repository by scanning for
// Go main packages, TypeScript package.json workspaces, Python pyproject.toml,
// Dockerfiles, and Kubernetes Deployment/StatefulSet manifests.
//
// It is an adapter package: os.ReadFile and filepath.Walk are permitted here.
// os/exec is forbidden — all subprocess calls go through toolrun.Runner.
package deployunit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	goListTimeout = 30 * time.Second
	nodeModules   = "node_modules"
	vendorDir     = "vendor"
)

// Detect scans root for deploy units and maps each discovered path to a unit
// name using mm to resolve module names. Runner is used only for `go list`.
// File I/O (Dockerfile, k8s manifests, package.json, pyproject.toml) is done
// directly — only os/exec is forbidden in adapters.
//
// Priority order (highest first): Go main pkg, TS workspace, Python pyproject,
// Dockerfile, k8s manifest. Only the first match per relative path is recorded.
func Detect(ctx context.Context, root string, mm policy.ModuleMap, runner toolrun.Runner) map[string]string {
	r := &detector{root: root, mm: mm, runner: runner}
	return r.detect(ctx)
}

// KeyByModule converts the path-keyed map returned by Detect (repo-relative
// path → deploy-unit name) into a module-name-keyed map suitable for
// config.FillMissingDeployUnits, which looks units up by module name (matching
// config.Config.Modules keys). Without this translation the two cannot be wired
// directly: auto-detected units are silently dropped unless a module's map key
// happens to equal the detected path.
//
// Each detected path is resolved to its owning module via mm.ModuleForFile. When
// ModuleForFile finds no match, the path is kept only if it is itself a module key
// (mm.Has) — that exact-key case was fillable under the old path-keyed wiring
// (e.g. a module keyed "cmd/tool" whose glob "cmd/tool/*.go" does not match the
// bare directory), so this keeps KeyByModule a strict superset and never drops
// what the old code filled. Paths matching neither are dropped (no module to
// attach the unit to). When several detected paths resolve to the same module,
// the alphabetically-first path wins (deterministic). A deploy-unit directory
// containing several modules fills only the module ModuleForFile selects for that
// path; filling every nested module is a separate enhancement (deploy-unit
// membership), not done here.
func KeyByModule(detected map[string]string, mm policy.ModuleMap) map[string]string {
	out := make(map[string]string, len(detected))
	paths := make([]string, 0, len(detected))
	for p := range detected {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		mod, ok := mm.ModuleForFile(p)
		if !ok {
			if !mm.Has(p) {
				continue // no glob match and not an exact module key — nothing to fill
			}
			mod = p // exact module-key fallback (old path-keyed behavior)
		}
		if _, exists := out[mod]; exists {
			continue // first detected path wins (deterministic)
		}
		out[mod] = detected[p]
	}
	return out
}

type detector struct {
	root   string
	mm     policy.ModuleMap
	runner toolrun.Runner
}

func (d *detector) detect(ctx context.Context) map[string]string {
	result := make(map[string]string)

	// Each source appends into result; first write wins per key.
	d.detectGoMain(ctx, result)
	d.detectTSWorkspaces(result)
	d.detectPyProject(result)
	d.detectDockerfiles(result)
	d.detectK8sManifests(result)

	return result
}

// relPath converts an absolute path to a repo-relative path.
// Returns "" when path is not under root.
func (d *detector) relPath(abs string) string {
	rel, err := filepath.Rel(d.root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return rel
}

// set records unit for relDir only if not already present (first write wins).
func set(result map[string]string, relDir, unitName string) {
	if relDir == "" || unitName == "" {
		return
	}
	if _, exists := result[relDir]; !exists {
		result[relDir] = unitName
	}
}

// dirName returns the last path element of a relative directory.
func dirName(relDir string) string {
	return filepath.Base(relDir)
}

// skipDir reports whether this directory should be skipped during Walk.
func skipDir(name string) bool {
	return name != "." && (strings.HasPrefix(name, ".") || name == nodeModules || name == vendorDir)
}

// ---------------------------------------------------------------------------
// 1. Go main packages via go list.
// ---------------------------------------------------------------------------

func (d *detector) detectGoMain(ctx context.Context, result map[string]string) {
	if _, ok := d.runner.Detect(ctx, "go"); !ok {
		return
	}

	out, err := d.runner.Run(ctx, toolrun.ToolCmd{
		Name:    "go",
		Args:    []string{"list", "-f", "{{if eq .Name \"main\"}}{{.Dir}}{{end}}", "./..."},
		WorkDir: d.root,
		Timeout: goListTimeout,
	})
	if err != nil || out.ExitCode != 0 {
		return
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out.Stdout)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rel := d.relPath(line)
		if rel == "" {
			continue
		}
		// Prefer module name from ModuleMap; fall back to directory base name.
		name := dirName(rel)
		if modName, ok := d.mm.ModuleForFile(rel); ok {
			// A main.go nested somewhere inside a module's tree (a migration/
			// dev-tool CLI helper, e.g. "promql/promqltest/cmd/migrate") is not
			// itself a deploy-unit boundary — only a main.go at the owning
			// module's own root is. Otherwise the whole module gets tagged as
			// its own deploy unit because of one incidental nested binary,
			// producing a false cross_deploy_unit distance against every other
			// module (confirmed on prometheus: promql/promqltest tagged solely
			// because of promql/promqltest/cmd/migrate/main.go, 3 directories
			// deep, while 13 sibling promtool imports into the same tree
			// correctly resolved cross_module_same_owner).
			if !d.mm.IsModuleRoot(rel) {
				continue
			}
			name = modName
		}
		set(result, rel, name)
	}
}

// ---------------------------------------------------------------------------
// 2. TypeScript package.json workspaces.
// ---------------------------------------------------------------------------

type packageJSON struct {
	Name       string          `json:"name"`
	Main       string          `json:"main"`
	Bin        any             `json:"bin"`        // string or object
	Workspaces json.RawMessage `json:"workspaces"` // []string or {"packages": []string}
}

// workspacePatterns extracts the glob patterns from a package.json "workspaces"
// field, accepting both common shapes: the array form `["a/*","b"]` (npm/Yarn
// classic) and the object form `{"packages": ["a/*"]}` (Yarn workspaces config).
// Returns nil for an absent or unrecognized shape — never errors, so a stray
// shape cannot abort root bin/main detection.
func workspacePatterns(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	var obj struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Packages
	}
	return nil
}

func (d *detector) detectTSWorkspaces(result map[string]string) {
	data, err := os.ReadFile(filepath.Join(d.root, "package.json"))
	if err != nil {
		return
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return
	}

	// Root package.json itself is a deploy unit if it has bin or main.
	if pkg.Bin != nil || pkg.Main != "" {
		name := pkg.Name
		if name == "" {
			name = dirName(".")
		}
		set(result, ".", name)
	}

	// Each workspace path is a potential deploy unit.
	for _, ws := range workspacePatterns(pkg.Workspaces) {
		// Workspaces may be globs; resolve them.
		matches, err := filepath.Glob(filepath.Join(d.root, ws))
		if err != nil {
			continue
		}
		for _, match := range matches {
			fi, err := os.Stat(match)
			if err != nil || !fi.IsDir() {
				continue
			}
			// Check if there's a package.json inside.
			wsData, err := os.ReadFile(filepath.Join(match, "package.json")) //nolint:gosec // match is workspace dir resolved from package.json glob
			if err != nil {
				continue
			}
			var wsPkg packageJSON
			if err := json.Unmarshal(wsData, &wsPkg); err != nil {
				continue
			}
			rel := d.relPath(match)
			name := wsPkg.Name
			if name == "" {
				name = dirName(rel)
			}
			set(result, rel, name)
		}
	}
}

// ---------------------------------------------------------------------------
// 3. Python pyproject.toml (TOML line-scanner; no external dependency).
// ---------------------------------------------------------------------------

func (d *detector) detectPyProject(result map[string]string) {
	_ = filepath.Walk(d.root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			if skipDir(fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if fi.Name() != "pyproject.toml" {
			return nil
		}

		data, err := os.ReadFile(path) //nolint:gosec // path comes from filepath.Walk under the repo root
		if err != nil {
			return nil
		}

		name := parsePyprojectName(data)
		rel := d.relPath(filepath.Dir(path))
		if name == "" {
			name = dirName(rel)
		}
		set(result, rel, name)
		return nil
	})
}

// parsePyprojectName extracts [project].name from pyproject.toml content.
// pyproject.toml is TOML; we use a simple line scanner rather than pulling in
// a TOML dependency just for one field.
func parsePyprojectName(data []byte) string {
	inProject := false
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "[project]" {
			inProject = true
			continue
		}
		// Any other [section] header ends the [project] block.
		if strings.HasPrefix(line, "[") {
			inProject = false
			continue
		}
		if !inProject {
			continue
		}
		// Match: name = "value" or name = 'value'
		before, after, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(before) != "name" {
			continue
		}
		val := strings.TrimSpace(after)
		val = strings.Trim(val, `"'`)
		return val
	}
	return ""
}

// ---------------------------------------------------------------------------
// 4. Dockerfiles.
// ---------------------------------------------------------------------------

func (d *detector) detectDockerfiles(result map[string]string) {
	_ = filepath.Walk(d.root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			if skipDir(fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		name := fi.Name()
		// Match Dockerfile or Dockerfile.* (e.g. Dockerfile.dev).
		if name == "Dockerfile" || strings.HasPrefix(name, "Dockerfile.") {
			rel := d.relPath(filepath.Dir(path))
			unitName := dirName(rel)
			if rel == "." {
				unitName = filepath.Base(d.root)
			}
			set(result, rel, unitName)
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// 5. Kubernetes Deployment / StatefulSet manifests (line-scanner; no yaml dep).
// ---------------------------------------------------------------------------

func (d *detector) detectK8sManifests(result map[string]string) {
	_ = filepath.Walk(d.root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			if skipDir(fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		data, err := os.ReadFile(path) //nolint:gosec // path comes from filepath.Walk under the repo root
		if err != nil {
			return nil
		}

		// A file may contain multiple YAML documents separated by ---.
		for _, doc := range splitYAMLDocs(data) {
			kind, metaName := parseK8sDoc(doc)
			if kind != "Deployment" && kind != "StatefulSet" {
				continue
			}
			rel := d.relPath(filepath.Dir(path))
			name := metaName
			if name == "" {
				name = dirName(rel)
				if rel == "." {
					name = filepath.Base(d.root)
				}
			}
			set(result, rel, name)
			break // one match per file is enough
		}
		return nil
	})
}

// parseK8sDoc extracts kind and metadata.name from a single YAML document
// using a simple line scanner. No external YAML library is used.
func parseK8sDoc(data []byte) (kind, metaName string) {
	inMetadata := false
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Top-level key: reset metadata section tracking.
		if len(rawLine) > 0 && rawLine[0] != ' ' && rawLine[0] != '\t' {
			inMetadata = false
			before, after, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			key := strings.TrimSpace(before)
			val := strings.TrimSpace(after)
			switch key {
			case "kind":
				kind = val
			case "metadata":
				inMetadata = true
			}
			continue
		}
		// Indented line inside metadata block.
		if inMetadata {
			before, after, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			if strings.TrimSpace(before) == "name" {
				metaName = strings.TrimSpace(after)
			}
		}
	}
	return kind, metaName
}

// splitYAMLDocs splits a YAML file that may contain multiple documents (---).
func splitYAMLDocs(data []byte) [][]byte {
	var docs [][]byte
	for _, part := range strings.Split(string(data), "\n---") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			docs = append(docs, []byte(trimmed))
		}
	}
	return docs
}
