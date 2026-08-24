// Package acquire coordinates repository evidence adapters behind one facade.
package acquire

import (
	"context"
	"time"

	"github.com/alexei-led/archfit/internal/extract/astgrep"
	"github.com/alexei-led/archfit/internal/extract/clones"
	"github.com/alexei-led/archfit/internal/extract/deployunit"
	"github.com/alexei-led/archfit/internal/extract/dynimports"
	"github.com/alexei-led/archfit/internal/extract/loc"
	"github.com/alexei-led/archfit/internal/extract/manifest"
	"github.com/alexei-led/archfit/internal/extract/registry"
	runtimedetect "github.com/alexei-led/archfit/internal/extract/runtime"
	"github.com/alexei-led/archfit/internal/extract/scip"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/syntax"
	"github.com/alexei-led/archfit/internal/toolrun"
	"github.com/alexei-led/archfit/internal/view"
)

const (
	toolAstGrepSyntax = "ast-grep/syntax"
	toolDeployUnit    = "deploy-unit"
	reasonScipOff     = "opt-in: analyzers.scip.enabled"
	reasonSyntaxOff   = "opt-in: analyzers.syntax.enabled"
)

var cloneTestGenGlobs = []string{
	"**/*_test.go", "**/*_test.ts", "**/*_test.py", "**/mock_*.go", "**/*_mock.go",
	"**/*_moq.go", "**/*.pb.go", "**/*_gen.go", "**/mocks/**", "**/__mocks__/**",
}

// Options contains projected adapter configuration for one acquisition run.
type Options struct {
	Exclusions    []string
	FileClass     syntax.FileClassConfig
	ModuleMap     module.Map
	ClonesEnabled bool
	CloneTimeout  time.Duration
	SCIPEnabled   bool
	SCIPTimeout   time.Duration
	Syntax        view.SyntaxConfig
	GoExtract     view.ExtractConfig
}

// Result contains neutral evidence and adapter ports for one pipeline run.
type Result struct {
	FileLOC             map[string]int
	FileClassIndex      map[string]fileclass.FileClass
	DynamicImports      []evidence.DynamicImportSite
	DeprecatedDeps      []evidence.DeprecatedDep
	RuntimeAsyncSites   []evidence.RuntimeAsyncSite
	RuntimeConfidence   string
	DeployUnitsByModule map[string]string
	DuplicationClusters []clone.Cluster
	ExtraCoverage       []evidence.Coverage
	Resolver            ports.SymbolResolver
	Syntax              ports.SyntaxProvider
	Patterns            ports.PatternProvider
	LOCError            error
	CloneError          error
}

// Collect runs auxiliary evidence adapters without leaking their concrete types to the CLI.
func Collect(ctx context.Context, root string, opts Options, runner toolrun.Runner, facts *factcache.Store) Result {
	var out Result
	var locCoverage evidence.Coverage
	out.FileLOC, out.FileClassIndex, locCoverage, out.LOCError = loc.RunWithConfig(root, opts.FileClass)
	out.ExtraCoverage = append(out.ExtraCoverage, locCoverage)

	out.DynamicImports = dynimports.Detect(root)
	out.DeprecatedDeps = manifest.Scan(root)

	runtimeResult := runtimedetect.Detect(ctx, root, runner)
	for _, site := range runtimeResult.Signals {
		out.RuntimeAsyncSites = append(out.RuntimeAsyncSites, evidence.RuntimeAsyncSite{
			File: site.File, Line: site.Line, Library: site.Library,
			IntegrationKind: string(site.IntegrationKind), Language: site.Language,
		})
	}
	out.RuntimeConfidence = runtimeResult.Confidence

	deployUnitsByPath := deployunit.Detect(ctx, root, opts.ModuleMap, runner)
	out.DeployUnitsByModule = deployunit.KeyByModule(deployUnitsByPath, opts.ModuleMap)
	out.ExtraCoverage = append(out.ExtraCoverage, evidence.Coverage{
		Tool: toolDeployUnit, FilesSeen: len(deployUnitsByPath), Status: evidence.StatusOK,
	})

	cloneExclusions := append(append([]string(nil), opts.Exclusions...), cloneTestGenGlobs...)
	var cloneCoverage evidence.Coverage
	out.DuplicationClusters, cloneCoverage, out.CloneError = clones.Run(
		ctx, runner, root, opts.ClonesEnabled, opts.CloneTimeout, cloneExclusions, facts,
	)
	out.ExtraCoverage = append(out.ExtraCoverage, cloneCoverage)

	out.Resolver = ports.NopSymbolResolver{}
	if opts.SCIPEnabled {
		adapter := scip.New(runner, opts.SCIPTimeout)
		adapter.Cache = facts
		adapter.GoWorkOff = registry.GoWorkOff(root, opts.GoExtract)
		out.Resolver = adapter
	} else {
		out.ExtraCoverage = append(out.ExtraCoverage, evidence.Coverage{
			Tool: "scip", Status: evidence.StatusDisabled, Reason: reasonScipOff,
		})
	}

	out.Syntax = ports.NopSyntaxProvider{}
	if opts.Syntax.Enabled {
		adapter := astgrep.New(runner)
		adapter.Cache = facts
		out.Syntax = adapter
	} else {
		out.ExtraCoverage = append(out.ExtraCoverage, evidence.Coverage{
			Tool: toolAstGrepSyntax, Status: evidence.StatusDisabled, Reason: reasonSyntaxOff,
		})
	}

	patternAdapter := astgrep.New(runner)
	patternAdapter.Cache = facts
	out.Patterns = patternAdapter
	return out
}
