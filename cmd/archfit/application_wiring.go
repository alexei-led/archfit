package main

import (
	"context"
	"slices"

	"github.com/alexei-led/archfit/v4/internal/application"
	"github.com/alexei-led/archfit/v4/internal/config"
	"github.com/alexei-led/archfit/v4/internal/evidence/acquisition"
	"github.com/alexei-led/archfit/v4/internal/model/evidence"
)

// newEvidenceService builds the concrete acquisition stage for one tree.
func newEvidenceService(configPath, root string, cfg config.Config, deps *appDeps) *acquisition.Service {
	return &acquisition.Service{
		ConfigPath: configPath, Root: root,
		Options: cfg.RunOptions(), Policy: cfg.PolicySnapshot(),
		Runner: deps.Runner,
		Stderr: deps.stderr(), Progress: deps.progress, WarnLabel: deps.warnLabel, Refresh: deps.refresh,
	}
}

// newAnalysisStages composes one analysis run: the policy preparer, the
// acquisition stage. Every field is a constructed value — no branching decision lives
// here, only the choice of concrete implementation.
func newAnalysisStages(configPath, root string, cfg config.Config, deps *appDeps) application.StageExecutor {
	return application.StageExecutor{
		Preparer: config.Preparer{Config: cfg, Stderr: deps.stderr()},
		Evidence: newEvidenceService(configPath, root, cfg, deps),
		Stderr:   deps.stderr(),
		Progress: deps.progress,
	}
}

func reportScope(in config.ReportInput) evidence.ReportScope {
	digest := in.Policy.Digest
	l := in.Config.Languages
	return evidence.ReportScope{
		Abstraction: "module", SourceRoot: ".",
		PolicyRef: evidence.ReportRef{Path: defaultConfigPath, SHA256: &digest, Status: "captured"},
		Exclude:   append([]string{}, in.Config.RunOptions().Exclusions...), Tests: append([]string{}, in.Policy.Tests...), Generated: append([]string{}, in.Policy.Generated...),
		Languages: []evidence.ReportLanguage{{Name: "go", Mode: reportLanguageMode(string(l.Go.Enabled))}, {Name: "typescript", Mode: reportLanguageMode(string(l.TypeScript.Enabled))}, {Name: "python", Mode: reportLanguageMode(string(l.Python.Enabled))}, {Name: "rust", Mode: reportLanguageMode(string(l.Rust.Enabled))}, {Name: "dotnet", Mode: reportLanguageMode(string(l.DotNet.Enabled))}},
	}
}

func reportSnapshotInputs(in config.ReportInput) []string {
	paths := []string{}
	coverage := in.Config.SuppliedCoverageOptions()
	if !coverage.Enabled {
		return paths
	}
	for _, source := range coverage.Sources {
		paths = append(paths, source.Path)
		sidecar := source.SidecarPath
		if sidecar == "" {
			sidecar = source.Path + ".sidecar.json"
		}
		paths = append(paths, sidecar)
	}
	return slices.Compact(paths)
}

func reportRequest(ctx context.Context, co checkout, deps *appDeps, in config.ReportInput, snapshots acquisition.ReportSnapshotSource) application.Request {
	file := co.policyPath("")
	return application.Request{ConfigSource: file, BundleDir: co.root, Formats: []string{formatJSON}, Identity: reportIdentityInputs(ctx, deps.Runner, in.Policy.Digest, co.root), ReportContext: &evidence.ReportContext{Scope: reportScope(in), Components: in.Components, Before: snapshots.Capture(ctx)}, Snapshots: snapshots}
}
