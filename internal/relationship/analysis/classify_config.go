package analysis

import (
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship/classify"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

// AugmentConfig returns cfg with the synthetic-module augmentation and
// ModuleMap rebuild Analyze applies before label freshness, classification,
// advisories, and diagnostics.
func AugmentConfig(g *graph.Graph, cfg classify.Config) classify.Config {
	// Register auto-discovered module-graph nodes (Rust "<crate>::<mod>") as modules so
	// classify can resolve their distance/volatility; otherwise their edges are
	// distance-unknown and coupling_balance/encapsulation never see them. No-op for
	// Go/TS/Python (their nodes are already configured; the "::" gate excludes them).
	cfg.Modules = classify.AugmentModulesFromGraph(g, cfg.Modules)
	// Register Go workspace members (≥2-member gate) as synthetic modules so
	// cross-member edges classify with a real Distance for coupling_balance. No-op for
	// single-module repos and archfit's own self-scan (1 surviving member after exclusion).
	cfg.Modules = classify.AugmentGoWorkspaceModules(g, cfg.Modules)
	// Bind crate-level Rust nodes (bare `package:<crate>` names) to the module whose
	// path glob covers the crate's directory, so multi-crate workspaces configured with
	// "crates/<crate>/**" globs measure coupling instead of classifying every cross-crate
	// edge as external. No-op for bare-name configs (tokio/yazi) and single-crate repos.
	cfg.Modules = classify.AugmentCargoCrateNodes(g, cfg.Modules)

	// Rebuild the ModuleMap from the augmented Modules slice so that all secondary
	// consumers see auto-registered members. The Augment* calls above mutate
	// cfg.Modules but NOT cfg.ModuleMap, which was built at config-view construction
	// time.
	cfg.ModuleMap = policy.BuildModuleMap(cfg.Modules)
	return cfg
}

// Classify augments module boundaries and runs relationship classification for
// review-only flows. Staged analysis uses Analyze, which owns the same steps
// plus label freshness, advisories, and report evidence.
func Classify(g *graph.Graph, cfg classify.Config) (classify.Config, coupling.Index) {
	cfg = AugmentConfig(g, cfg)
	return cfg, classify.Run(g, cfg)
}

// StaticExternalDistanceCandidates turns unresolved external dependency targets
// into declared-external review candidates.
func StaticExternalDistanceCandidates(g *graph.Graph, idx coupling.Index, mm policy.ModuleMap) []evidence.DistanceConfigCandidate {
	return buildStaticDistanceCandidates(g, idx, mm)
}
