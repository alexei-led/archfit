package main

import (
	"context"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"

	"github.com/alexei-led/archfit/internal/config"
	historygit "github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	volatilityCorroborationTopN = 5
	volatilityCorroborationSrc  = "git_history"
	volatilityCorroborationNote = "Supporting evidence only. Git history can reflect both essential and accidental volatility and never changes scoring or gate verdicts."
)

func buildVolatilityCorroboration(ctx context.Context, gitRoot, subtreePrefix string, cfg config.Config, runner toolrun.Runner) *evidence.VolatilityCorroboration {
	if gitRoot == "" || len(cfg.Modules) == 0 {
		return nil
	}
	mm := cfg.ModuleMapView()
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
			DeclaredVolatility: declaredVolatilityLabel(cfg.Modules[mod]),
		})
	}
	return out
}

func declaredVolatilityLabel(def config.ModuleDef) string {
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
