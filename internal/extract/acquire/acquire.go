// Package acquire coordinates repository evidence adapters behind one facade.
package acquire

import (
	"context"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/astgrep"
	"github.com/alexei-led/archfit/v3/internal/extract/deployunit"
	"github.com/alexei-led/archfit/v3/internal/extract/dynimports"
	"github.com/alexei-led/archfit/v3/internal/extract/loc"
	"github.com/alexei-led/archfit/v3/internal/extract/manifest"
	runtimedetect "github.com/alexei-led/archfit/v3/internal/extract/runtime"
	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/syntax"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	toolAstGrepSyntax = "ast-grep/syntax"
)

// ReasonSyntaxDisabled is the published Reason on the StatusDisabled coverage
// row this package emits when the opt-in syntax pass is off.
const ReasonSyntaxDisabled = "off by default because the pass is slow: set analyzers.syntax.enabled: true for roles, routes and exported surface"

// Options contains projected adapter configuration for one acquisition run.
type Options struct {
	Exclusions []string
	FileClass  syntax.FileClassConfig
	ModuleMap  policy.ModuleMap
	Syntax     evidenceports.SyntaxConfig
	GoExtract  evidenceports.ExtractConfig
}

// Result contains neutral evidence and adapter ports for one pipeline run.
type Result struct {
	FileLOC                 map[string]int
	FileClassIndex          map[string]fileclass.FileClass
	DynamicImports          []evidence.DynamicImportSite
	DeprecatedDeps          []evidence.DeprecatedDep
	RuntimeAsyncSites       []evidence.RuntimeAsyncSite
	RuntimeConfidence       string
	DeployUnitsByModule     map[string]string
	CorroboratedDeployUnits map[string]evidence.CorroboratedDeployUnit
	ExtraCoverage           []evidence.Coverage
	Resolver                evidenceports.SymbolResolver
	Syntax                  evidenceports.SyntaxProvider
	LOCError                error
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

	deployResult := deployunit.Discover(ctx, root, opts.ModuleMap, runner, opts.GoExtract)
	deployUnitsByPath := deployResult.Units
	out.CorroboratedDeployUnits = deployunit.KeyCorroboratedByModule(deployUnitsByPath, opts.ModuleMap)
	out.DeployUnitsByModule = make(map[string]string, len(out.CorroboratedDeployUnits))
	for module, fact := range out.CorroboratedDeployUnits {
		out.DeployUnitsByModule[module] = fact.Unit
	}
	out.ExtraCoverage = append(out.ExtraCoverage, deployResult.Coverage)

	out.Resolver = evidenceports.NopSymbolResolver{}
	out.Syntax = evidenceports.NopSyntaxProvider{}
	if opts.Syntax.Enabled {
		adapter := astgrep.New(runner)
		adapter.Cache = facts
		adapter.Exclusions = opts.Exclusions
		out.Syntax = adapter
	} else {
		out.ExtraCoverage = append(out.ExtraCoverage, evidence.Coverage{
			Tool: toolAstGrepSyntax, Status: evidence.StatusDisabled, Reason: ReasonSyntaxDisabled,
		})
	}

	return out
}
