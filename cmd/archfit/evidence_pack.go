package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/initcfg"
)

const repositoryEvidenceHeader = "Repository evidence with stable IDs:"

func architectureEvidenceLines(root string, modules []initcfg.ModuleDef, configPath string, diagnostics []initcfg.EvidenceDiagnostic) []string {
	items := initcfg.BuildArchitectureEvidencePack(initcfg.EvidencePackOptions{
		Root:        root,
		Modules:     modules,
		ConfigPath:  configPath,
		Diagnostics: diagnostics,
	})
	return initcfg.FormatEvidencePack(items)
}

func optionalEvidence(values [][]string) []string {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}

func discoveredEvidenceDiagnostics(cfg initcfg.DiscoveredConfig) []initcfg.EvidenceDiagnostic {
	return []initcfg.EvidenceDiagnostic{{
		Source: "discovery",
		Summary: fmt.Sprintf(
			"languages go=%t python=%t typescript=%t rust=%t; modules=%d; layers=%s; dependency_edges=%d",
			cfg.HasGo, cfg.HasPython, cfg.HasTS, cfg.HasRust, len(cfg.Modules), strings.Join(cfg.Layers, ","), len(cfg.Edges),
		),
	}}
}

func updateEvidenceDiagnostics(report initcfg.UpdateReport) []initcfg.EvidenceDiagnostic {
	return []initcfg.EvidenceDiagnostic{{
		Source: "update",
		Summary: fmt.Sprintf(
			"added=%d suggested=%d removed=%d path_drift=%d unclassified=%d structurally_in_sync=%t",
			len(report.Added), len(report.Suggested), len(report.Removed), len(report.PathDrift), len(report.Unclassified), report.StructuralInSync,
		),
	}}
}

func enrichEvidenceDiagnostics(source string, count int) []initcfg.EvidenceDiagnostic {
	return []initcfg.EvidenceDiagnostic{{
		Source:  source,
		Summary: fmt.Sprintf("candidate_count=%d", count),
	}}
}

func configModulesForEvidence(cfg config.Config) []initcfg.ModuleDef {
	mods := make([]initcfg.ModuleDef, 0, len(cfg.Modules))
	for name, mod := range cfg.Modules {
		mods = append(mods, initcfg.ModuleDef{
			Name:     name,
			Paths:    mod.Paths,
			Public:   mod.Public,
			Internal: mod.Internal,
			Layer:    mod.Layer,
		})
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].Name < mods[j].Name })
	return mods
}

func scanRootForEvidence(configDir, root string) string {
	if root == "" {
		return configDir
	}
	if filepath.IsAbs(root) {
		return root
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	return abs
}
