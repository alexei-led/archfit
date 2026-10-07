package main

import (
	"os"
	"os/exec"
	"testing"
)

// requireToolsEnv makes a missing analyzer a failure instead of a skip. CI
// sets it, so the TypeScript and Python agreement gates cannot go quiet when
// an install step breaks.
const requireToolsEnv = "ARCHFIT_REQUIRE_TOOLS"

// requireTool skips the test when an analyzer binary is absent, or fails it
// when ARCHFIT_REQUIRE_TOOLS=1.
func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		if os.Getenv(requireToolsEnv) == "1" {
			t.Fatalf("%s is not on PATH and %s=1: %v", name, requireToolsEnv, err)
		}
		t.Skipf("%s is not on PATH; the %s fixture needs the real analyzer", name, t.Name())
	}
}

// skipShortWithoutRequiredTools keeps the real-analyzer fixtures out of the
// inner loop (`make test-fast`) unless the caller insists on them.
func skipShortWithoutRequiredTools(t *testing.T) {
	t.Helper()
	if testing.Short() && os.Getenv(requireToolsEnv) != "1" {
		t.Skip("real-analyzer agreement fixture skipped under -short")
	}
}

// Rule IDs shared by the TypeScript and Python fixtures.
const (
	langRuleLayerOrder = "layer_order"
	langRuleAllowlists = "allowlists"
	langRuleWebNoCore  = "web_not_domain"
)

var langEdgeClasses = map[string]string{
	"allowlist":                 langRuleAllowlists,
	"layer order":               langRuleLayerOrder,
	"forbidden_dependency path": langRuleWebNoCore,
}

// The TypeScript fixture is JavaScript on purpose: dependency-cruiser parses
// .ts only when the typescript package sits beside it, and CI installs
// dependency-cruiser alone. The extractor, node spelling and rules are the
// TypeScript ones.
const tsAgreementCfg = `version: 2
languages:
  typescript:
    enabled: true
  go:
    enabled: false
  python:
    enabled: false
  rust:
    enabled: false
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
    layer: infrastructure
  web:
    paths: ["src/web/**"]
rules:
  - id: layer_order
    type: forbidden_layer_direction
    gate: fail
  - id: allowlists
    type: module_dependencies
    gate: fail
  - id: web_not_domain
    type: forbidden_dependency
    gate: fail
    from: src/web/**
    to: src/domain/**
`

const (
	tsDomainIndex = "src/domain/index.js"
	tsInfraIndex  = "src/infra/index.js"
	tsPackageJSON = "package.json"
)

var tsAgreementImports = []agreementImport{
	{file: "src/domain/bad.js", target: tsInfraIndex, want: answerDenied},
	{file: "src/app/app.js", target: tsDomainIndex, want: answerAllowed},
	{file: "src/app/infra.js", target: tsInfraIndex, want: answerDenied},
	{file: "src/web/db.js", target: tsDomainIndex, want: answerDenied},
	{file: "src/web/ok.js", target: "src/app/index.js", want: answerUnconstrain},
}

const pyAgreementCfg = `version: 2
languages:
  python:
    enabled: true
    package: myapp
  go:
    enabled: false
  typescript:
    enabled: false
  rust:
    enabled: false
layers: [domain, application, infrastructure]
modules:
  domain:
    paths: ["myapp.domain", "myapp.domain.**"]
    layer: domain
    visible_to: [app, web]
  app:
    paths: ["myapp.app", "myapp.app.**"]
    layer: application
    depends_on: [domain]
  infra:
    paths: ["myapp.infra", "myapp.infra.**"]
    layer: infrastructure
  web:
    paths: ["myapp.web", "myapp.web.**"]
rules:
  - id: layer_order
    type: forbidden_layer_direction
    gate: fail
  - id: allowlists
    type: module_dependencies
    gate: fail
  - id: web_not_domain
    type: forbidden_dependency
    gate: fail
    from: myapp.web**
    to: myapp.domain**
`

// For Python, file is the importing path and target the dotted module, as
// the extractor spells them.
var pyAgreementImports = []agreementImport{
	{file: "myapp/domain/bad.py", target: "myapp.infra", want: answerDenied},
	{file: "myapp/app/app.py", target: "myapp.domain", want: answerAllowed},
	{file: "myapp/app/infra.py", target: "myapp.infra", want: answerDenied},
	{file: "myapp/web/db.py", target: "myapp.domain", want: answerDenied},
	{file: "myapp/web/ok.py", target: "myapp.app", want: answerUnconstrain},
}

func writeTSAgreementRepo(t *testing.T) string {
	t.Helper()
	files := map[string]string{
		tsPackageJSON:       `{"name":"agree","private":true,"type":"module"}` + "\n",
		defaultConfigPath:   tsAgreementCfg,
		tsDomainIndex:       "export const name = 'domain';\n",
		"src/app/index.js":  "export const name = 'app';\n",
		tsInfraIndex:        "export const name = 'infra';\n",
		"src/domain/bad.js": "import { name } from '../infra/index.js';\n\nexport const bad = name;\n",
		"src/app/app.js":    "import { name } from '../domain/index.js';\n\nexport const ok = name;\n",
		"src/app/infra.js":  "import { name } from '../infra/index.js';\n\nexport const direct = name;\n",
		"src/web/db.js":     "import { name } from '../domain/index.js';\n\nexport const db = name;\n",
		"src/web/ok.js":     "import { name } from '../app/index.js';\n\nexport const ok = name;\n",
	}
	return writeAgreementFiles(t, t.TempDir(), files)
}

func writePyAgreementRepo(t *testing.T) string {
	t.Helper()
	files := map[string]string{
		"pyproject.toml":           "[project]\nname = \"agree\"\nversion = \"0.1.0\"\nrequires-python = \">=3.9\"\n",
		defaultConfigPath:          pyAgreementCfg,
		"myapp/__init__.py":        "",
		"myapp/domain/__init__.py": "NAME = 'domain'\n",
		"myapp/app/__init__.py":    "NAME = 'app'\n",
		"myapp/infra/__init__.py":  "NAME = 'infra'\n",
		"myapp/web/__init__.py":    "",
		"myapp/domain/bad.py":      "import myapp.infra\n",
		"myapp/app/app.py":         "import myapp.domain\n",
		"myapp/app/infra.py":       "import myapp.infra\n",
		"myapp/web/db.py":          "import myapp.domain\n",
		"myapp/web/ok.py":          "import myapp.app\n",
	}
	return writeAgreementFiles(t, t.TempDir(), files)
}

// TestErosion_PolicyQueryAgreesWithCheckTypeScript runs the policy_query_agreement
// comparison end to end on a TypeScript fixture, through dependency-cruiser.
func TestErosion_PolicyQueryAgreesWithCheckTypeScript(t *testing.T) {
	skipShortWithoutRequiredTools(t)
	requireTool(t, "depcruise")
	requireTool(t, "npx")
	assertAgreement(t, writeTSAgreementRepo(t), tsAgreementImports, langEdgeClasses)
}

// TestErosion_PolicyQueryAgreesWithCheckPython runs the same comparison on a
// Python fixture, through grimp (fetched by uv on first use).
func TestErosion_PolicyQueryAgreesWithCheckPython(t *testing.T) {
	skipShortWithoutRequiredTools(t)
	requireTool(t, "uv")
	assertAgreement(t, writePyAgreementRepo(t), pyAgreementImports, langEdgeClasses)
}
