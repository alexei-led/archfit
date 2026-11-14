package evaluation_test

import (
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
)

// dotnetScopeFixture is a .NET-only run over four projects with the dependency
// gaps acquisition pinned to files (Observations.ScopedDependencyEvidence).
func dotnetScopeFixture(msbuild string, gaps map[string]string) (*result.Result, evaluation.StateInput) {
	diag, in := dimensionsFixture()
	primaries := []string{toolGoPackages, toolDepCruiser, "pyimports", toolCargo, toolMSBuild}
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

// TestDotNetIncompleteRoslynEvidenceKeepsDimensionsPartial pins D9: intent and
// the graph dimensions report the same incompleteness the rules do, naming
// roslyn, while the project-level dotnet-msbuild row is ok. With .NET the only
// applicable language, structure has no completed primary and is unmeasured,
// as it is for a partial dotnet-msbuild row or a TypeScript-only partial run.
func TestDotNetIncompleteRoslynEvidenceKeepsDimensionsPartial(t *testing.T) {
	diag, in := dotnetScopeFixture(modevidence.StatusOK, map[string]string{csApplication: gapApplication})
	dims := evaluation.BuildDimensions(diag, in, nil)
	for _, dim := range []state.Dimension{dims.Modularity} {
		if dim.Status != state.Partial {
			t.Errorf("%s status = %q, want partial", dim.Name, dim.Status)
		}
	}
	if dims.Structure.Status != state.Unmeasured {
		t.Errorf("structure status = %q, want unmeasured", dims.Structure.Status)
	}
	var reason string
	for _, unknown := range dims.Structure.Unknown {
		if unknown.Fact == state.FactPrimaryDependencyInventory {
			reason = unknown.Reason
		}
	}
	if !strings.Contains(reason, toolRoslyn+": "+gapApplication+"; fix: "+fixRestore) {
		t.Errorf("structure primary inventory reason = %q, want the roslyn gap named", reason)
	}

	diag, in = dotnetScopeFixture(modevidence.StatusOK, nil)
	dims = evaluation.BuildDimensions(diag, in, nil)
	for _, dim := range []state.Dimension{dims.Intent, dims.Structure, dims.Modularity} {
		if dim.Status != state.Measured {
			t.Errorf("complete roslyn: %s status = %q, want measured", dim.Name, dim.Status)
		}
	}
}

func withModules(in *evaluation.StateInput, modules map[string]policy.ModuleDef) {
	in.Policy.Topology = policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
}
