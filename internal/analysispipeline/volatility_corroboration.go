package pipeline

import (
	"context"
	"strings"

	historygit "github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	volatilityCorroborationTopN = 5
	volatilityCorroborationSrc  = "git_history"
	volatilityCorroborationNote = "Supporting evidence only. Git history can reflect both essential and accidental volatility and never changes scoring or gate verdicts."
)

// BuildVolatilityCorroboration summarizes git-history touches as report-only evidence.
func BuildVolatilityCorroboration(ctx context.Context, gitRoot, subtreePrefix string, p policy.PolicySnapshot, runner toolrun.Runner) *evidence.VolatilityCorroboration {
	if gitRoot == "" || len(p.Topology.Modules) == 0 {
		return nil
	}
	mm := p.Topology.ModuleMap
	touches := historygit.TouchCounts(ctx, gitRoot, subtreePrefix, mm.ModuleFor, runner)
	if touches.Status == historygit.ModuleTouchStatusUnavailable {
		return nil
	}
	out := &evidence.VolatilityCorroboration{
		Source:         volatilityCorroborationSrc,
		Status:         string(touches.Status),
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

func declaredVolatilityLabel(def module.ModuleDef) string {
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
