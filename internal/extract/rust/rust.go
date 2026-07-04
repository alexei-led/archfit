package rust

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	toolCargo    = "cargo"
	langRust     = "rust"
	statusOK     = "ok"
	statusAbsent = "absent"
	manifestFile = "Cargo.toml"
	runTimeout   = 5 * time.Minute
)

// cargo dependency kinds reported by `cargo metadata`.
const (
	depKindDev   = "dev"
	depKindBuild = "build"
)

// Extractor is the Rust dependency extractor using `cargo metadata`.
// It satisfies the ports.Extractor interface structurally.
type Extractor struct {
	runner             toolrun.Runner
	cfg                config.ExtractConfig
	lastModuleGraphCov diagnostic.Coverage // cargo-modules coverage from most recent Extract call
	lastCrateRoots     []graph.CrateRoot   // crate roots from most recent Extract call
	// Cache is the extractor fact cache; nil disables caching (--no-cache).
	Cache *factcache.Store
}

// rustManifestNames are the fact-affecting manifests hashed into the cargo
// metadata fact-cache key: `cargo metadata --no-deps` derives everything from
// them, so source files stay out of that key (a .rs edit must NOT re-run it).
var rustManifestNames = []string{"Cargo.toml", "Cargo.lock"}

// New returns an Extractor configured with the given runner and config. The
// module-graph coverage is seeded to a well-formed absent record so a caller that
// reads LastModuleGraphCoverage before Extract never sees a zero-value Coverage.
func New(runner toolrun.Runner, cfg config.ExtractConfig) *Extractor {
	return &Extractor{
		runner:             runner,
		cfg:                cfg,
		lastModuleGraphCov: diagnostic.Coverage{Tool: toolCargoModules, Status: statusAbsent},
	}
}

// Name returns the language identifier for this extractor.
func (e *Extractor) Name() string {
	return langRust
}

// Extract detects cargo, runs `cargo metadata` against the project root, parses
// the JSON output, and returns a graph.Facts + diagnostic.Coverage.
//
// If mode is off, Extract returns empty Facts and an "absent" Coverage immediately.
// If mode is auto and cargo is absent or the project has no Cargo.toml, Extract
// returns empty Facts and an "absent" Coverage record — never an error.
// If mode is on and the marker or binary is missing, Extract returns an error.
func (e *Extractor) Extract(ctx context.Context, s scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
	// Default the module-graph coverage to a valid absent record so EVERY early
	// return (disabled, not-applicable, cargo errors) leaves the pipeline a
	// well-formed row instead of a zero-value Coverage{} (empty Tool/Status). The
	// cfg.ModuleGraph branch below overwrites it when the graph actually runs.
	e.lastModuleGraphCov = diagnostic.Coverage{Tool: toolCargoModules, Status: statusAbsent}
	if e.cfg.Mode == config.ModeOff {
		return graph.Facts{}, absentCoverage(""), nil
	}

	// Applicability marker: a configured rust_manifest (resolved against s.Root
	// exactly like cargo's --manifest-path) when set, else the root Cargo.toml.
	// This lets a sub-crate manifest drive analysis without a root manifest. Any
	// stat error (absent, unreadable) means not-applicable, matching the py
	// extractor's presence test — don't fall through to cargo on a non-ENOENT error.
	marker := filepath.Join(s.Root, manifestFile)
	if e.cfg.CargoManifest != "" {
		marker = e.cfg.CargoManifest
		if !filepath.IsAbs(marker) {
			marker = filepath.Join(s.Root, marker)
		}
	}
	if _, err := os.Stat(marker); err != nil {
		// An explicitly configured rust_manifest that is missing is a
		// misconfiguration → error in on mode. But a missing DEFAULT root
		// Cargo.toml just means "not a Rust project at this root": degrade to an
		// n/a coverage record (warn-loud), never exit 3, even when rust is enabled
		// — a multi-language repo or a manifest-less subtree must not fail the run (E3).
		if e.cfg.Mode == config.ModeOn && e.cfg.CargoManifest != "" {
			return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/rust: configured rust_manifest %s not found", marker)
		}
		return graph.Facts{}, absentCoverage(""), nil
	}

	// Detect cargo.
	if _, ok := e.runner.Detect(ctx, toolCargo); !ok {
		if e.cfg.Mode == config.ModeOn {
			return graph.Facts{}, diagnostic.Coverage{}, errors.New("extract/rust: cargo not found; install Rust via https://rustup.rs")
		}
		return graph.Facts{}, absentCoverage(""), nil
	}

	version := e.detectVersion(ctx)

	// --no-deps limits the package set to first-party workspace members; each
	// member still lists its direct dependencies, which we resolve to package:/
	// external: targets below.
	args := []string{"metadata", "--format-version", "1", "--no-deps"}
	if len(e.cfg.CargoFeatures) > 0 {
		args = append(args, "--features", strings.Join(e.cfg.CargoFeatures, ","))
	}
	if e.cfg.CargoManifest != "" {
		args = append(args, "--manifest-path", e.cfg.CargoManifest)
	}

	out, err := e.metadataRunner(s, version).Run(ctx, toolrun.ToolCmd{
		Name:    toolCargo,
		Args:    args,
		WorkDir: s.Root,
		Timeout: runTimeout,
	})
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/rust: run cargo metadata: %w", err)
	}
	if out.ExitCode != 0 {
		// cargo metadata exited non-zero — e.g. a malformed manifest or an
		// unresolvable dependency graph. This is a coverage gap, not a run-level
		// failure (the "warn-loud, don't block" contract); only an explicitly
		// required analyzer (ModeOn) hard-errors.
		reason := fmt.Sprintf("cargo metadata exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
		if e.cfg.Mode == config.ModeOn {
			return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/rust: %s", reason)
		}
		return graph.Facts{}, diagnostic.Coverage{Tool: toolCargo, Version: version, Status: statusPartial, Reason: reason}, nil
	}

	facts, members, cov, err := e.parseAndNormalize(out.Stdout, version)
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/rust: parse output: %w", err)
	}

	// Carry crate roots (repo-relative src dir + crate name) so the filesystem-free
	// core ring can resolve .rs files to module keys ("<crate>::<mod>") for the
	// size/cohesion metrics — the crate name is not derivable from a path alone.
	facts.CrateRoots = crateRoots(s.Root, members)
	e.lastCrateRoots = facts.CrateRoots

	// Opt-in intra-crate module graph via cargo-modules (analyzers.cargo_modules.enabled: true).
	// When enabled, module-level nodes and edges are merged into facts alongside the
	// crate-level nodes from parseAndNormalize, giving metrics (cycle, blast_radius,
	// cohesion) module granularity and preventing the degenerate-graph guard from
	// tripping on single-crate repos. The module-graph coverage row is stored in
	// lastModuleGraphCov so the pipeline can append it to ExtraCoverage (same pattern
	// as complexity/clones, which also surface coverage outside Extract).
	if e.cfg.ModuleGraph {
		modNodes, modEdges, modCov := e.runModuleGraph(ctx, e.moduleGraphRunner(ctx, s, version), members)
		facts.Nodes = append(facts.Nodes, modNodes...)
		facts.Edges = append(facts.Edges, modEdges...)
		e.lastModuleGraphCov = modCov
	} else {
		e.lastModuleGraphCov = diagnostic.Coverage{Tool: toolCargoModules, Status: statusAbsent}
	}

	return facts, cov, nil
}

// LastModuleGraphCoverage returns the cargo-modules coverage record from the most
// recent Extract call. The pipeline appends this to change.ExtraCoverage so it
// appears in the diagnostic's ToolCoverage block alongside the cargo row.
// Returns absent coverage when ModuleGraph is disabled or Extract has not been called.
func (e *Extractor) LastModuleGraphCoverage() diagnostic.Coverage {
	return e.lastModuleGraphCov
}

// LastCrateRoots returns the crate roots (repo-relative crate dir + crate
// name) from the most recent Extract call. The pipeline uses this to resolve
// "<crate>::<mod>" module keys (cargo-modules' intra-crate convention) to a
// real directory for agent_tasks files[]. Nil when Extract has not been
// called or found no workspace members.
func (e *Extractor) LastCrateRoots() []graph.CrateRoot {
	return e.lastCrateRoots
}

// metadataRunner wraps the runner in a fact-cache decorator for the cargo
// metadata invocation (fact-cache.md D5 seam 1). Returns the plain runner
// when the cache is off or key material cannot be derived — never fails the
// run. Key inputs: cargo version, the ExtractConfig view, the scan root (the
// metadata JSON embeds absolute manifest paths, so entries are per-checkout),
// and the content hash of every Cargo.toml/Cargo.lock under s.Root.
func (e *Extractor) metadataRunner(s scope.Scope, version string) toolrun.Runner {
	return e.cachedRunner(s, langRust, version, nil)
}

// moduleGraphRunner wraps the runner in a fact-cache decorator for the
// per-crate cargo-modules invocations. cargo-modules reads the source tree,
// so the key adds every .rs file to the manifest set, plus the cargo-modules
// version (probed here — best-effort, "" when the binary is absent; Detect in
// runModuleGraph still reports the tool absent before any Run happens).
func (e *Extractor) moduleGraphRunner(ctx context.Context, s scope.Scope, cargoVersion string) toolrun.Runner {
	if e.Cache == nil {
		return e.runner
	}
	ver := cargoVersion + "\x00" + e.toolVersion(ctx, toolCargoModules)
	return e.cachedRunner(s, "cargo_modules", ver, []string{".rs"})
}

// cachedRunner builds the fact-cache decorator shared by the two cargo seams:
// the input scope is the manifest set (rustManifestNames) plus any extra
// extensions the analyzer reads (.rs for cargo-modules, none for metadata).
func (e *Extractor) cachedRunner(s scope.Scope, analyzer, version string, exts []string) toolrun.Runner {
	if e.Cache == nil {
		return e.runner
	}
	cfgHash, err := factcache.HashJSON(struct {
		Cfg  config.ExtractConfig
		Root string
	}{e.cfg, s.Root})
	if err != nil {
		return e.runner
	}
	exclude := append([]string{"**/target/**"}, e.cfg.Exclusions...)
	files := factcache.ListInputs(s.Root, factcache.MatchExts(exts, rustManifestNames), exclude)
	treeHash, err := factcache.HashTree(s.Root, files)
	if err != nil {
		return e.runner
	}
	return &factcache.Runner{
		Inner:     e.runner,
		Store:     e.Cache,
		Analyzer:  analyzer,
		Key:       factcache.Key(analyzer, version, cfgHash, treeHash),
		Cacheable: func(out toolrun.Output) bool { return out.ExitCode == 0 },
	}
}

// toolVersion runs `<tool> --version` and returns the trimmed version string.
// Returns empty string on any failure (non-fatal).
func (e *Extractor) toolVersion(ctx context.Context, tool string) string {
	out, err := e.runner.Run(ctx, toolrun.ToolCmd{
		Name:    tool,
		Args:    []string{"--version"},
		Timeout: 30 * time.Second,
	})
	if err != nil || out.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(out.Stdout))
}

// detectVersion runs `cargo --version` and returns the trimmed version string.
// Returns empty string on any failure (non-fatal).
func (e *Extractor) detectVersion(ctx context.Context) string {
	out, err := e.runner.Run(ctx, toolrun.ToolCmd{
		Name:    toolCargo,
		Args:    []string{"--version"},
		Timeout: 30 * time.Second,
	})
	if err != nil || out.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(out.Stdout))
}

// ---------------------------------------------------------------------------
// JSON parsing types for `cargo metadata --format-version 1`.
// ---------------------------------------------------------------------------

type cargoMetadata struct {
	Packages         []cargoPackage `json:"packages"`
	WorkspaceMembers []string       `json:"workspace_members"`
	WorkspaceRoot    string         `json:"workspace_root"`
}

type cargoPackage struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	ManifestPath string            `json:"manifest_path"`
	Source       *string           `json:"source"`
	Dependencies []cargoDependency `json:"dependencies"`
	Targets      []cargoTarget     `json:"targets"`
}

type cargoTarget struct {
	Name string   `json:"name"`
	Kind []string `json:"kind"`
}

type cargoDependency struct {
	Name   string  `json:"name"`
	Source *string `json:"source"`
	// Kind is null for a normal dependency, "dev", or "build".
	Kind *string `json:"kind"`
}

// included reports whether this dependency should become a graph edge. Normal
// and build dependencies always count; dev-dependencies count only when
// includeDev is set.
func (d cargoDependency) included(includeDev bool) bool {
	if d.Kind == nil {
		return true // normal dependency
	}
	switch *d.Kind {
	case depKindDev:
		return includeDev
	case depKindBuild:
		return true
	default:
		return true
	}
}

// ---------------------------------------------------------------------------
// Parse + normalise.
// ---------------------------------------------------------------------------

// parseAndNormalize parses `cargo metadata` JSON and builds graph.Facts.
//
// Workspace members are the first-party set: each becomes a package:<crate>
// node. A dependency whose crate name matches a member resolves to a package:
// node (intra-workspace edge); any other dependency becomes an external: node.
// Every kept dependency yields a depends_on edge located at Cargo.toml.
// The resolved members slice is returned so ModuleGraph can reuse it without
// re-running cargo metadata.
func (e *Extractor) parseAndNormalize(data []byte, version string) (graph.Facts, []cargoPackage, diagnostic.Coverage, error) {
	var meta cargoMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return graph.Facts{}, nil, diagnostic.Coverage{}, fmt.Errorf("unmarshal: %w", err)
	}

	// First-party set: packages whose ID is listed in workspace_members. With
	// --no-deps cargo already restricts packages to members, so if the ID format
	// drifts and nothing matches we fall back to treating every package as a
	// member (still correct under --no-deps).
	memberIDs := make(map[string]struct{}, len(meta.WorkspaceMembers))
	for _, id := range meta.WorkspaceMembers {
		memberIDs[id] = struct{}{}
	}
	var members []cargoPackage
	memberNames := make(map[string]struct{})
	for _, p := range meta.Packages {
		if _, ok := memberIDs[p.ID]; ok {
			members = append(members, p)
			memberNames[p.Name] = struct{}{}
		}
	}
	idFallback := false
	if len(members) == 0 {
		// workspace_members listed IDs but none matched a package ID — a cargo
		// metadata format drift (e.g. the cargo 1.96 `path+file://…#name@ver` change).
		// Under --no-deps every package is a member, so promoting them all is correct,
		// but record it so the regression is visible, not silent.
		idFallback = true
		for _, p := range meta.Packages {
			members = append(members, p)
			memberNames[p.Name] = struct{}{}
		}
	}

	var nodes []graph.Node
	var edges []graph.Edge
	seenNodes := make(map[string]struct{})
	seenEdges := make(map[string]struct{})

	emitNode := func(n graph.Node) {
		id := n.ID()
		if _, ok := seenNodes[id]; !ok {
			seenNodes[id] = struct{}{}
			nodes = append(nodes, n)
		}
	}

	for _, m := range members {
		fromNode := graph.Node{Kind: graph.NodeKindPackage, Path: m.Name, Language: graph.LangRust}
		emitNode(fromNode)

		for _, dep := range m.Dependencies {
			if !dep.included(e.cfg.IncludeDevDeps) {
				continue
			}

			kind := graph.NodeKindExternal
			if _, ok := memberNames[dep.Name]; ok {
				kind = graph.NodeKindPackage
			}
			toNode := graph.Node{Kind: kind, Path: dep.Name, Language: graph.LangRust}
			emitNode(toNode)

			edgeKey := fromNode.ID() + "\x00" + toNode.ID()
			if _, ok := seenEdges[edgeKey]; ok {
				continue
			}
			seenEdges[edgeKey] = struct{}{}

			edges = append(edges, graph.Edge{
				From:       fromNode.ID(),
				To:         toNode.ID(),
				Kind:       graph.EdgeKindDependsOn,
				Language:   langRust,
				Confidence: "high",
				Locations:  []graph.Location{{File: manifestFile}},
			})
		}
	}

	cov := diagnostic.Coverage{
		Tool:            toolCargo,
		Version:         version,
		FilesSeen:       len(members),
		FilesApplicable: len(members),
		Status:          statusOK,
	}
	if idFallback {
		cov.Reason = "workspace_members IDs matched no package (cargo metadata format drift?); treated all packages as members"
	}
	facts := graph.Facts{
		Nodes:    nodes,
		Edges:    edges,
		Language: langRust,
	}
	return facts, members, cov, nil
}

// crateRoots maps each workspace member to its repo-relative source dir and crate
// name so the core ring can resolve a .rs path to its module key. cargo reports an
// absolute manifest_path; members whose dir is outside the analysed root, or that
// cannot be made relative to it, are skipped (best-effort — never fails extraction).
// A member at the root itself yields Dir "".
func crateRoots(root string, members []cargoPackage) []graph.CrateRoot {
	// EvalSymlinks resolves case variants and symlinks so rootAbs matches the
	// canonical ManifestPath reported by cargo (macOS case-insensitive FS).
	// Falls back to Abs if the path does not exist yet (e.g. tests with fake paths).
	rootAbs, err := filepath.EvalSymlinks(root)
	if err != nil {
		rootAbs, err = filepath.Abs(root)
		if err != nil {
			return nil
		}
	}
	out := make([]graph.CrateRoot, 0, len(members))
	for _, m := range members {
		rel, err := filepath.Rel(rootAbs, filepath.Dir(m.ManifestPath))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue // outside the analysed root
		}
		if rel == "." {
			rel = ""
		}
		out = append(out, graph.CrateRoot{Dir: filepath.ToSlash(rel), Name: m.Name})
	}
	return out
}

// absentCoverage returns a Coverage record indicating cargo was not found.
func absentCoverage(version string) diagnostic.Coverage {
	return diagnostic.Coverage{
		Tool:    toolCargo,
		Version: version,
		Status:  statusAbsent,
	}
}
