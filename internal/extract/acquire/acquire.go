// Package acquire coordinates repository evidence adapters behind one facade.
package acquire

import (
	"context"
	"time"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
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
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/syntax"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	toolAstGrepSyntax = "ast-grep/syntax"
	toolDeployUnit    = "deploy-unit"
)

// ReasonSCIPDisabled is the published Reason on the StatusDisabled coverage row
// this package emits when the opt-in SCIP pass is off. Exported so tests assert
// the shipped text rather than a copy of it.
const ReasonSCIPDisabled = "opt-in: analyzers.scip.enabled"

// ReasonSyntaxDisabled is the published Reason on the StatusDisabled coverage
// row this package emits when the opt-in syntax pass is off.
const ReasonSyntaxDisabled = "opt-in: analyzers.syntax.enabled"

var cloneTestGenGlobs = []string{
	"**/*_test.go", "**/*_test.ts", "**/*_test.py", "**/mock_*.go", "**/*_mock.go",
	"**/*_moq.go", "**/*.pb.go", "**/*_gen.go", "**/mocks/**", "**/__mocks__/**",
}

// Options contains projected adapter configuration for one acquisition run.
type Options struct {
	Exclusions    []string
	FileClass     syntax.FileClassConfig
	ModuleMap     policy.ModuleMap
	ClonesEnabled bool
	CloneTimeout  time.Duration
	SCIPEnabled   bool
	SCIPTimeout   time.Duration
	Syntax        evidenceports.SyntaxConfig
	GoExtract     evidenceports.ExtractConfig
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
	DuplicationClusters     []clone.Cluster
	ExtraCoverage           []evidence.Coverage
	Resolver                evidenceports.SymbolResolver
	Syntax                  evidenceports.SyntaxProvider
	Patterns                evidenceports.PatternProvider
	LOCError                error
	CloneError              error
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

	deployUnitsByPath := deployunit.DetectCorroborated(ctx, root, opts.ModuleMap, runner)
	out.CorroboratedDeployUnits = deployunit.KeyCorroboratedByModule(deployUnitsByPath, opts.ModuleMap)
	out.DeployUnitsByModule = make(map[string]string, len(out.CorroboratedDeployUnits))
	for module, fact := range out.CorroboratedDeployUnits {
		out.DeployUnitsByModule[module] = fact.Unit
	}
	out.ExtraCoverage = append(out.ExtraCoverage, evidence.Coverage{
		Tool: toolDeployUnit, FilesSeen: len(deployUnitsByPath), Status: evidence.StatusOK,
	})

	cloneExclusions := append(append([]string(nil), opts.Exclusions...), cloneTestGenGlobs...)
	var cloneCoverage evidence.Coverage
	out.DuplicationClusters, cloneCoverage, out.CloneError = clones.Run(
		ctx, runner, root, opts.ClonesEnabled, opts.CloneTimeout, cloneExclusions, facts,
	)
	out.ExtraCoverage = append(out.ExtraCoverage, cloneCoverage)

	out.Resolver = evidenceports.NopSymbolResolver{}
	if opts.SCIPEnabled {
		adapter := scip.New(runner, opts.SCIPTimeout)
		adapter.Cache = facts
		adapter.GoWorkOff = registry.GoWorkOff(root, opts.GoExtract)
		out.Resolver = adapter
	} else {
		out.ExtraCoverage = append(out.ExtraCoverage, evidence.Coverage{
			Tool: "scip", Status: evidence.StatusDisabled, Reason: ReasonSCIPDisabled,
		})
	}

	out.Syntax = evidenceports.NopSyntaxProvider{}
	if opts.Syntax.Enabled {
		adapter := astgrep.New(runner)
		adapter.Cache = facts
		out.Syntax = adapter
	} else {
		out.ExtraCoverage = append(out.ExtraCoverage, evidence.Coverage{
			Tool: toolAstGrepSyntax, Status: evidence.StatusDisabled, Reason: ReasonSyntaxDisabled,
		})
	}

	patternAdapter := astgrep.New(runner)
	patternAdapter.Cache = facts
	out.Patterns = patternAdapter
	return out
}
