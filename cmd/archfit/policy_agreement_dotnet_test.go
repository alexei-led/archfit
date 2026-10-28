package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const dotnetAgreementCfg = `version: 2
languages:
  dotnet:
    enabled: true
  go:
    enabled: false
  typescript:
    enabled: false
  python:
    enabled: false
  rust:
    enabled: false
analyzers:
  roslyn:
    enabled: true
layers: [domain, application, infrastructure]
modules:
  domain:
    paths: ["src/domain/**"]
    layer: domain
    visible_to: [app, web]
  app:
    paths: ["src/app/**"]
    layer: application
    depends_on: [domain]
  infra:
    paths: ["src/infra/**"]
    public: ["src/infra/Index.cs", "src/infra/api/**"]
    internal: ["src/infra/impl/**"]
    layer: infrastructure
  web:
    paths: ["src/web/**"]
  worker:
    paths: ["src/worker/**"]
rules:
  - id: layer_order
    type: forbidden_layer_direction
    gate: fail
  - id: allowlists
    type: module_dependencies
    gate: fail
  - id: public_only
    type: public_api_only
    gate: fail
  - id: web_not_infra
    type: forbidden_dependency
    gate: fail
    from_module: web
    to_module: infra
  - id: web_not_domain
    type: forbidden_dependency
    gate: fail
    from: src/web/**
    to: src/domain/**
  - id: domain_no_linq
    type: forbidden_dependency
    gate: fail
    from: src/domain/**
    to: System.Linq*
`

const (
	roslynHelperEnv = "ARCHFIT_DOTNET_ROSLYN"
	csDomainName    = "src/domain/Name.cs"
	csInfraIndex    = "src/infra/Index.cs"
	csInfraAPI      = "src/infra/api/Client.cs"
	csInfraImpl     = "src/infra/impl/Pool.cs"
)

// For .NET, target is the file Roslyn resolves the used type to, or the
// assembly of an external type.
var dotnetAgreementImports = []agreementImport{
	{file: "src/domain/Bad.cs", target: csInfraIndex, want: answerDenied},
	{file: "src/app/AppUse.cs", target: csDomainName, want: answerAllowed},
	{file: "src/app/InfraUse.cs", target: csInfraIndex, want: answerDenied},
	{file: "src/web/Db.cs", target: csDomainName, want: answerDenied},
	{file: "src/web/Ok.cs", target: "src/app/AppName.cs", want: answerUnconstrain},
	{file: "src/web/Internal.cs", target: csInfraImpl, want: answerDenied},
	{file: "src/web/Api.cs", target: csInfraAPI, want: answerDenied},
	{file: "src/worker/Job.cs", target: csInfraAPI, want: answerUnconstrain},
	{file: "src/worker/Bad.cs", target: csInfraImpl, want: answerDenied},
	{file: "src/domain/Sorted.cs", target: "System.Linq", want: answerDenied},
}

// csProject is an SDK class library referencing the given projects.
func csProject(refs ...string) string {
	items := make([]string, 0, len(refs))
	for _, ref := range refs {
		items = append(items, `<ProjectReference Include="`+ref+`" />`)
	}
	return `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework></PropertyGroup><ItemGroup>` +
		strings.Join(items, "") + `</ItemGroup></Project>`
}

// csUse is a C# file in namespace ns whose class reads value.
func csUse(ns, class, value string) string {
	return "namespace " + ns + ";\n\npublic static class " + class + "\n{\n    public static string Read() => " + value + ";\n}\n"
}

func writeDotNetAgreementRepo(t *testing.T) string {
	t.Helper()
	const domain, infra, app = "../domain/Domain.csproj", "../infra/Infra.csproj", "../app/App.csproj"
	files := map[string]string{
		defaultConfigPath:          dotnetAgreementCfg,
		"src/domain/Domain.csproj": csProject(infra),
		"src/app/App.csproj":       csProject(domain, infra),
		"src/infra/Infra.csproj":   csProject(),
		"src/web/Web.csproj":       csProject(domain, app, infra),
		"src/worker/Worker.csproj": csProject(infra),
		csDomainName:               csUse("Agree.Domain", "Name", `"domain"`),
		"src/app/AppName.cs":       csUse("Agree.App", "AppName", `"app"`),
		csInfraIndex:               csUse("Agree.Infra", "Index", `"infra"`),
		csInfraAPI:                 csUse("Agree.Infra.Api", "Client", `"api"`),
		csInfraImpl:                csUse("Agree.Infra.Impl", "Pool", `"impl"`),
		"src/domain/Bad.cs":        csUse("Agree.Domain", "Bad", "Agree.Infra.Index.Read()"),
		"src/domain/Sorted.cs":     csUse("Agree.Domain", "Sorted", `System.Linq.Enumerable.First(new[] { "a" })`),
		"src/app/AppUse.cs":        csUse("Agree.App", "AppUse", "Agree.Domain.Name.Read()"),
		"src/app/InfraUse.cs":      csUse("Agree.App", "InfraUse", "Agree.Infra.Index.Read()"),
		"src/web/Db.cs":            csUse("Agree.Web", "Db", "Agree.Domain.Name.Read()"),
		"src/web/Ok.cs":            csUse("Agree.Web", "Ok", "Agree.App.AppName.Read()"),
		"src/web/Internal.cs":      csUse("Agree.Web", "Internal", "Agree.Infra.Impl.Pool.Read()"),
		"src/web/Api.cs":           csUse("Agree.Web", "Api", "Agree.Infra.Api.Client.Read()"),
		"src/worker/Job.cs":        csUse("Agree.Worker", "Job", "Agree.Infra.Api.Client.Read()"),
		"src/worker/Bad.cs":        csUse("Agree.Worker", "Bad", "Agree.Infra.Impl.Pool.Read()"),
	}
	return writeAgreementFiles(t, t.TempDir(), files)
}

// TestErosion_PolicyQueryAgreesWithCheckDotNet runs the policy_query_agreement
// comparison on a .NET fixture, through MSBuild evaluation and the Roslyn
// helper, including a ban on an external assembly.
func TestErosion_PolicyQueryAgreesWithCheckDotNet(t *testing.T) {
	skipShortWithoutRequiredTools(t)
	requireTool(t, "dotnet")
	// The helper is a build product, not an installed analyzer. Locally its
	// absence skips; CI builds it first, so under ARCHFIT_REQUIRE_TOOLS it fails.
	helper := os.Getenv(roslynHelperEnv)
	if helper == "" {
		helper, _ = filepath.Abs(filepath.Join("..", "..", ".bin", "dotnet-roslyn"))
	}
	if _, err := os.Stat(filepath.Join(helper, "archfit-roslyn.dll")); err != nil { //nolint:gosec // helper directory from the test environment
		if os.Getenv(requireToolsEnv) == "1" {
			t.Fatalf("no built helper at %s; run make dotnet-roslyn", helper)
		}
		t.Skipf("no built helper at %s; run make dotnet-roslyn", helper)
	}
	t.Setenv(roslynHelperEnv, helper)
	t.Setenv("DOTNET_CLI_HOME", t.TempDir())
	t.Setenv("DOTNET_NOLOGO", "1")
	t.Setenv("DOTNET_CLI_TELEMETRY_OPTOUT", "1")
	classes := map[string]string{"forbidden_dependency external": "domain_no_linq"}
	for class, rule := range langEdgeClasses {
		classes[class] = rule
	}
	assertAgreement(t, writeDotNetAgreementRepo(t), dotnetAgreementImports, classes)
}
