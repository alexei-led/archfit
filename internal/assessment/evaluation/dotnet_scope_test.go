package evaluation_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	toolMSBuild    = "dotnet-msbuild"
	toolGoPackages = "go/packages"
	toolRoslyn     = "roslyn"

	csDomainEntity = "src/Domain/Entities/TodoItem.cs"
	csDomainCommon = "src/Domain/Common/BaseEntity.cs"
	csApplication  = "src/Application/Handler.cs"
	csInfra        = "src/Infrastructure/Repository.cs"
	csAppHost      = "src/AppHost/Program.cs"

	globDomain = "src/Domain/**"
	globApp    = "src/Application/**"
	globInfra  = "src/Infrastructure/**"
	globHost   = "src/AppHost/**"

	modDomain = "Domain"
	modApp    = "Application"
	modInfra  = "Infrastructure"
	modHost   = "AppHost"

	gapApplication = "src/Application/Application.csproj partial"
	gapAppHost     = "src/AppHost/AppHost.csproj failed MSBuild evaluation"
	fixRestore     = "restore the projects with NuGet access (`dotnet restore`); the roslyn row names each project's cause"
	fixSDK         = "MSB4236: SDK 'Aspire.AppHost.Sdk' not resolvable offline; run with NuGet access and a warm cache"
	reasonRoslyn   = "roslyn evidence is incomplete for the rule scope: "
	reasonAppPart  = reasonRoslyn + gapApplication + "; fix: " + fixRestore
	reasonHostFail = reasonRoslyn + gapAppHost + "; fix: " + fixSDK

	typeFileCycle  = "cycle"
	typeAllowlists = "module_dependencies"
)

// dotnetScopeFixture is a .NET-only run over four projects with the dependency
// gaps acquisition pinned to files (Observations.ScopedDependencyEvidence).
func dotnetScopeFixture(msbuild string, gaps map[string]string) (*result.Result, evaluation.StateInput) {
	diag, in := dimensionsFixture()
	primaries := []string{toolGoPackages, toolDepCruiser, "grimp", toolCargo, toolMSBuild}
	diag.PrimaryExtractorTools = primaries
	diag.ToolCoverage = []modevidence.Coverage{{Tool: toolDeployDiscovery, Status: modevidence.StatusOK}}
	for _, tool := range primaries[:4] {
		diag.ToolCoverage = append(diag.ToolCoverage, modevidence.Coverage{Tool: tool, Status: modevidence.StatusAbsent})
	}
	roslyn := modevidence.StatusOK
	if len(gaps) > 0 {
		roslyn = modevidence.StatusPartial
	}
	diag.ToolCoverage = append(diag.ToolCoverage,
		modevidence.Coverage{Tool: toolMSBuild, Status: msbuild},
		modevidence.Coverage{Tool: toolRoslyn, Status: roslyn},
	)
	files := []string{csDomainEntity, csDomainCommon, csApplication, csInfra, csAppHost}
	in.Facts.FileLOC = map[string]int{}
	in.Facts.FileClassIndex = map[string]fileclass.FileClass{}
	for _, file := range files {
		in.Facts.FileLOC[file] = 10
		in.Facts.FileClassIndex[file] = fileclass.Production
	}
	in.Facts.ScopedDependencyEvidence = map[string]evaluation.ScopedDependencyEvidence{
		"dotnet": {Tool: toolRoslyn, Incomplete: gaps, Fixes: map[string]string{gapApplication: fixRestore, gapAppHost: fixSDK}},
	}
	withModules(&in, map[string]policy.ModuleDef{
		modDomain: {Paths: []string{globDomain}, DependsOn: []string{}},
		modApp:    {Paths: []string{globApp}, DependsOn: []string{modDomain}},
		modInfra:  {Paths: []string{globInfra}},
		modHost:   {Paths: []string{globHost}},
	})
	return diag, in
}

// TestDotNetRulesUnevaluatedOnlyWhereRoslynEvidenceIsIncomplete pins D1 and
// D6: a required rule whose dependency source reaches a file of a project the
// helper did not load complete is unevaluated, naming roslyn, the project and
// the fix; a rule scoped clear of those files is still evaluated, even while
// the coverage rows are partial. Without pinned gaps the partial primary row
// holds every .NET rule, as before.
func TestDotNetRulesUnevaluatedOnlyWhereRoslynEvidenceIsIncomplete(t *testing.T) {
	appPartial := map[string]string{csApplication: gapApplication}
	hostFailed := map[string]string{csAppHost: gapAppHost}
	tests := []struct {
		name       string
		msbuild    string
		gaps       map[string]string
		unpinned   bool
		modules    func(map[string]policy.ModuleDef)
		rule       policy.RuleDef
		wantReason string
	}{
		{name: "glob rule clear of the partial project", gaps: appPartial,
			rule: policy.RuleDef{Type: ruleForbidden, From: globDomain, To: globInfra}},
		{name: "glob rule from the partial project", gaps: appPartial,
			rule:       policy.RuleDef{Type: ruleForbidden, From: globApp, To: globInfra},
			wantReason: reasonAppPart},
		{name: "glob rule from every file", gaps: appPartial,
			rule:       policy.RuleDef{Type: ruleForbidden, To: globInfra},
			wantReason: reasonAppPart},
		{name: "from_module clear of the partial project", gaps: appPartial,
			rule: policy.RuleDef{Type: ruleForbidden, FromModule: modDomain, ToModule: modInfra}},
		{name: "from_module on the partial project", gaps: appPartial,
			rule:       policy.RuleDef{Type: ruleForbidden, FromModule: modApp, ToModule: modInfra},
			wantReason: reasonAppPart},
		{name: "layer direction reads every module", gaps: appPartial,
			rule:       policy.RuleDef{Type: typeLayerDir},
			wantReason: reasonAppPart},
		{name: "module cycle reads every module", gaps: appPartial,
			rule:       policy.RuleDef{Type: typeModCycle},
			wantReason: reasonAppPart},
		{name: "file cycle reads every file", gaps: appPartial,
			rule:       policy.RuleDef{Type: typeFileCycle},
			wantReason: reasonAppPart},
		{name: "depends_on of complete modules only", gaps: map[string]string{csInfra: gapApplication},
			rule: policy.RuleDef{Type: typeAllowlists}},
		{name: "depends_on of the partial project", gaps: appPartial,
			rule:       policy.RuleDef{Type: typeAllowlists},
			wantReason: reasonAppPart},
		{name: "visible_to reads every importer", gaps: map[string]string{csInfra: gapApplication},
			modules: func(m map[string]policy.ModuleDef) {
				def := m[modDomain]
				def.VisibleTo = []string{modApp}
				m[modDomain] = def
			},
			rule:       policy.RuleDef{Type: typeAllowlists},
			wantReason: reasonAppPart},
		{name: "failed evaluation of another project", msbuild: modevidence.StatusPartial, gaps: hostFailed,
			rule: policy.RuleDef{Type: ruleForbidden, From: globDomain, To: globInfra}},
		{name: "failed evaluation in the rule scope", msbuild: modevidence.StatusPartial, gaps: hostFailed,
			rule:       policy.RuleDef{Type: ruleForbidden, From: globHost, To: globInfra},
			wantReason: reasonHostFail},
		{name: "unpinned partial evaluation holds every rule", msbuild: modevidence.StatusPartial, unpinned: true,
			rule:       policy.RuleDef{Type: ruleForbidden, From: globDomain, To: globInfra},
			wantReason: "dotnet-msbuild evidence is partial"},
		{name: "restored run evaluates everything",
			rule: policy.RuleDef{Type: typeLayerDir}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msbuild := tt.msbuild
			if msbuild == "" {
				msbuild = modevidence.StatusOK
			}
			diag, in := dotnetScopeFixture(msbuild, tt.gaps)
			if tt.unpinned {
				in.Facts.ScopedDependencyEvidence = nil
			}
			if tt.modules != nil {
				modules := maps.Clone(in.Policy.Topology.Modules)
				tt.modules(modules)
				withModules(&in, modules)
			}
			withRule(&in, tt.rule)
			want := map[string]string{}
			if tt.wantReason != "" {
				want[ruleIDScoped] = tt.wantReason
			}
			if got := unevaluatedReasons(diag, in); !maps.Equal(got, want) {
				t.Fatalf("unevaluated_required_rules = %v, want %v", got, want)
			}
		})
	}
}

func TestDotNetScopedGapsListAtMostFive(t *testing.T) {
	gaps := map[string]string{}
	for _, project := range []string{"A", "B", "C", "D", "E", "F", "G"} {
		file := "src/Application/" + project + ".cs"
		gaps[file] = "src/Application/" + project + "/" + project + ".csproj partial"
	}
	diag, in := dotnetScopeFixture(modevidence.StatusOK, gaps)
	for file := range gaps {
		in.Facts.FileLOC[file] = 1
	}
	withRule(&in, policy.RuleDef{Type: ruleForbidden, From: globApp, To: globInfra})
	reason := unevaluatedReasons(diag, in)[ruleIDScoped]
	if !strings.HasSuffix(reason, "E/E.csproj partial, and 2 more") {
		t.Fatalf("reason = %q, want five gaps and the rest counted", reason)
	}
	if strings.Count(reason, ".csproj partial") != 5 {
		t.Errorf("reason = %q names %d gaps, want 5", reason, strings.Count(reason, ".csproj partial"))
	}
}

func TestDotNetScopedGapsIgnoreOutOfScopeFiles(t *testing.T) {
	diag, in := dotnetScopeFixture(modevidence.StatusOK, map[string]string{csApplication: gapApplication})
	in.Facts.OutOfScopeFiles = map[string]struct{}{csApplication: {}}
	withRule(&in, policy.RuleDef{Type: typeLayerDir})
	if got := unevaluatedReasons(diag, in); len(got) != 0 {
		t.Errorf("unevaluated_required_rules = %v, want none: the incomplete file is declared out of scope", got)
	}
	if dims := evaluation.BuildDimensions(diag, in, nil); dims.Structure.Status != state.Measured {
		t.Errorf("structure = %q, want measured", dims.Structure.Status)
	}
}
