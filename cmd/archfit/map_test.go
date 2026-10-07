package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	mapGoldenDir = "testdata/map"
	hexCfg       = `version: 2
layers: [domain, application, adapter, entrypoint]
coupling:
  min_severity: critical
modules:
  api:
    paths: ["internal/api/**"]
    layer: entrypoint
  stripe:
    paths: ["internal/stripe/**"]
    layer: adapter
  orders:
    paths: ["internal/orders/**"]
    layer: application
  billing:
    paths: ["internal/billing/**"]
    layer: domain
  audit:
    paths: ["internal/audit/**"]
rules:
  - id: layers-point-inward
    type: forbidden_layer_direction
    gate: fail
  - id: api-not-into-billing
    type: forbidden_dependency
    from: internal/api/**
    to: internal/billing
    gate: fail
`
)

// hexagonalRepo is a small hexagonal repository: api (entrypoint) -> stripe
// (adapter) -> orders (application) -> billing (domain), plus an unlayered
// audit module. api -> billing breaks a rule the baseline accepts; billing ->
// stripe, added after the baseline, points against the layer order.
func hexagonalRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	goFile := func(pkg string, imports ...string) string {
		var b strings.Builder
		b.WriteString("package " + pkg + "\n\n")
		for i, imp := range imports {
			b.WriteString("import x" + string(rune('a'+i)) + " \"example.com/hex/internal/" + imp + "\"\n")
		}
		b.WriteString("\nfunc " + strings.ToUpper(pkg[:1]) + pkg[1:] + "() {\n")
		for i, imp := range imports {
			b.WriteString("\tx" + string(rune('a'+i)) + "." + strings.ToUpper(imp[:1]) + imp[1:] + "()\n")
		}
		b.WriteString("}\n")
		return b.String()
	}
	for name, content := range map[string]string{
		markerGoMod:                   "module example.com/hex\n\ngo 1.21\n",
		"internal/api/api.go":         goFile("api", "stripe", "billing"),
		"internal/stripe/stripe.go":   goFile("stripe", "orders"),
		"internal/orders/orders.go":   goFile("orders", "billing", "audit"),
		"internal/billing/billing.go": goFile("billing"),
		"internal/audit/audit.go":     goFile("audit"),
		defaultConfigPath:             hexCfg,
	} {
		writeFixtureFile(t, dir, name, content)
	}
	gitInitFixtureRepo(t, dir)
	var buf bytes.Buffer
	if code := Run([]string{cmdBaseline, "-c", filepath.Join(dir, defaultConfigPath), flagRefresh}, &buf); code != 0 {
		t.Fatalf("baseline exit = %d\n%s", code, buf.String())
	}
	writeFixtureFile(t, dir, "internal/billing/billing.go", goFile("billing", "stripe"))
	return dir
}

func runMap(t *testing.T, dir string, args ...string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	code := Run(append([]string{"map", "-c", filepath.Join(dir, defaultConfigPath), "-q"}, args...), &buf)
	return code, buf.String()
}

func assertMapGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(mapGoldenDir, name)
	if os.Getenv("ARCHFIT_UPDATE_MAP") != "" {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) // #nosec G304 — fixed golden path
	if err != nil {
		t.Fatalf("golden %s is missing; regenerate deliberately with ARCHFIT_UPDATE_MAP=1: %v", path, err)
	}
	if got != string(want) {
		t.Errorf("map output differs from %s (regenerate with ARCHFIT_UPDATE_MAP=1 and inspect the diff)\n--- got ---\n%s--- want ---\n%s", path, got, want)
	}
}

// TestRun_Map_Hexagonal pins the architecture map on a hexagonal repository:
// layers outermost first, a violation drawn thick, accepted debt dotted (never
// allowed), permitted seams allowed, an unlayered module last, and --focus
// counting what it omits.
func TestRun_Map_Hexagonal(t *testing.T) {
	t.Parallel()
	dir := hexagonalRepo(t)
	code, mermaid := runMap(t, dir)
	if code != 0 {
		t.Fatalf("map exit = %d\n%s", code, mermaid)
	}
	assertMapGolden(t, "hexagonal.mmd", mermaid)
	for _, want := range []string{"==>|\"violation", "-.->|\"accepted"} {
		if !strings.Contains(mermaid, want) {
			t.Errorf("mermaid map missing %q:\n%s", want, mermaid)
		}
	}

	code, text := runMap(t, dir, "--format", "text")
	if code != 0 {
		t.Fatalf("map --format text exit = %d\n%s", code, text)
	}
	assertMapGolden(t, "hexagonal.txt", text)

	code, focused := runMap(t, dir, "--format", "text", "--focus", "audit")
	if code != 0 {
		t.Fatalf("map --focus exit = %d\n%s", code, focused)
	}
	assertMapGolden(t, "hexagonal-focus.txt", focused)
	if !strings.Contains(focused, "OMITTED BY --focus:") || !strings.Contains(focused, "1 violation") {
		t.Errorf("--focus must count the omitted violation:\n%s", focused)
	}

	if code, out := runMap(t, dir, "--focus", "nowhere"); code != 3 {
		t.Errorf("unknown --focus exit = %d, want 3\n%s", code, out)
	}
}
