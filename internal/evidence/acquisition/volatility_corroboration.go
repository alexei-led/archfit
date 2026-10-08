package acquisition

import (
	"context"
	"strings"

	historygit "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// Declared-volatility vocabulary used when labelling git-history corroboration.
const (
	volatilityLow        = "low"
	volatilityMedium     = "medium"
	volatilityHigh       = "high"
	volatilityFrozen     = "frozen"
	volatilityLegacy     = "legacy"
	volatilityUndeclared = "undeclared"

	subdomainCore       = "core"
	subdomainSupporting = "supporting"
	subdomainGeneric    = "generic"

	volatilityCorroborationTopN = 5
	volatilityCorroborationSrc  = "git_history"
	volatilityCorroborationNote = "Supporting evidence only. Git history can reflect both essential and accidental volatility and never changes scoring or gate verdicts."
)

// buildVolatilityCorroboration summarizes git-history touches as report-only evidence.
func buildVolatilityCorroboration(ctx context.Context, gitRoot, subtreePrefix string, p policy.PolicySnapshot, runner toolrun.Runner, roots ...graph.CrateRoot) *evidence.VolatilityCorroboration {
	if gitRoot == "" || len(p.Topology.Modules) == 0 {
		return nil
	}
	mm := p.Topology.ModuleMap
	missingCrateIdentity := false
	needsCrateIdentity := historyUsesCrateSelectors(p.Topology)
	moduleFor := func(file string) (string, bool) {
		if module, ok := mm.ModuleFor(file); ok {
			return module, true
		}
		language, _, _ := mm.RuleSelectorForFile(file, roots...)
		if language == graph.LangRust && len(roots) == 0 {
			missingCrateIdentity = missingCrateIdentity || needsCrateIdentity
			return "", false
		}
		return mm.ModuleForFile(file, roots...)
	}
	touches := historygit.TouchCounts(ctx, gitRoot, subtreePrefix, moduleFor, runner)
	status := string(touches.Status)
	if missingCrateIdentity && touches.Status == historygit.ModuleTouchStatusOK {
		status = evidence.StatusPartial
	}
	out := &evidence.VolatilityCorroboration{
		Source:         volatilityCorroborationSrc,
		Status:         status,
		CommitWindow:   touches.CommitWindow,
		FullHistory:    touches.FullHistory,
		CommitsScanned: touches.CommitsScanned,
		ModulesTouched: len(touches.TouchedByModule),
		Caveat:         volatilityCorroborationNote,
	}
	for i, mod := range touches.RankedModules() {
		if i == volatilityCorroborationTopN {
			break
		}
		out.TopTouched = append(out.TopTouched, evidence.VolatilityTouch{
			Module:             mod,
			TouchCommits:       touches.TouchedByModule[mod],
			DeclaredVolatility: declaredVolatilityLabel(p.Topology.Modules[mod]),
		})
	}
	return out
}

func historyUsesCrateSelectors(topology policy.TopologyView) bool {
	for _, module := range topology.Modules {
		for _, pattern := range module.Paths {
			if _, rust := topology.ModuleMap.SelectorLanguages(pattern)[graph.LangRust]; rust {
				return true
			}
		}
	}
	return false
}

func declaredVolatilityLabel(def policy.ModuleDef) string {
	switch strings.ToLower(def.Volatility) {
	case volatilityHigh:
		return volatilityHigh
	case volatilityMedium:
		return volatilityMedium
	case volatilityLow:
		return volatilityLow
	case volatilityFrozen, volatilityLegacy:
		return volatilityFrozen
	}
	switch strings.ToLower(def.Subdomain) {
	case subdomainCore:
		return volatilityHigh
	case subdomainSupporting, subdomainGeneric:
		return volatilityLow
	default:
		return volatilityUndeclared
	}
}
