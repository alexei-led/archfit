// Model compat gate (CI): the shared kernel (internal/model/*)
// is a pinned published contract — its declared volatility in .archfit.yaml is
// `low`, and this test is what makes that label true. Any change to the
// kernel's exported surface (removed/renamed symbols, changed signatures)
// fails CI until the golden file is deliberately regenerated:
//
//	ARCHFIT_UPDATE_SURFACE=1 go test ./internal/ -run TestModelSurfaceNoDrift -count=1
//
// Regenerating is the explicit "version bump" act: it must be reviewed as a
// contract change, not slipped in with feature work.
package arch_test

import (
	"fmt"
	"go/types"
	"os"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const surfaceGolden = "testdata/model_surface.golden"

func TestModelSurfaceNoDrift(t *testing.T) {
	got := renderKernelSurface(t)

	if os.Getenv("ARCHFIT_UPDATE_SURFACE") == "1" {
		if err := os.WriteFile(surfaceGolden, []byte(got), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(surfaceGolden)
	if err != nil {
		t.Fatalf("read golden (run ARCHFIT_UPDATE_SURFACE=1 go test ./internal/ -run TestModelSurfaceNoDrift to create it): %v", err)
	}
	if got != string(want) {
		t.Errorf("model kernel exported surface drift — this is a published-contract change.\n"+
			"If intentional, regenerate with ARCHFIT_UPDATE_SURFACE=1 and call it out in review.\n%s",
			surfaceDiff(string(want), got))
	}
}

// renderKernelSurface loads the kernel packages and renders every exported
// top-level object (types, funcs, consts, vars) with its full type signature,
// sorted, one per line, grouped by package.
func renderKernelSurface(t *testing.T) string {
	t.Helper()
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: "."}
	pkgs, err := packages.Load(cfg, modulePrefix+"internal/model/...")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].PkgPath < pkgs[j].PkgPath })

	qual := func(p *types.Package) string {
		return strings.TrimPrefix(p.Path(), modulePrefix)
	}
	var b strings.Builder
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			for _, e := range pkg.Errors {
				t.Errorf("load error in %s: %v", pkg.PkgPath, e)
			}
			continue
		}
		fmt.Fprintf(&b, "package %s\n", strings.TrimPrefix(pkg.PkgPath, modulePrefix))
		scope := pkg.Types.Scope()
		names := scope.Names()
		sort.Strings(names)
		for _, name := range names {
			obj := scope.Lookup(name)
			if !obj.Exported() {
				continue
			}
			line := types.ObjectString(obj, qual)
			fmt.Fprintf(&b, "  %s\n", line)
			// Exported methods are contract too.
			if tn, ok := obj.(*types.TypeName); ok {
				if named, ok := tn.Type().(*types.Named); ok {
					for i := range named.NumMethods() {
						m := named.Method(i)
						if m.Exported() {
							fmt.Fprintf(&b, "  %s\n", types.ObjectString(m, qual))
						}
					}
				}
			}
		}
	}
	return b.String()
}

// surfaceDiff returns a minimal line-level diff (missing/added lines) between
// the golden and current surface — enough to name the changed symbols without
// pulling in a diff dependency.
func surfaceDiff(want, got string) string {
	wantSet := make(map[string]bool)
	for _, l := range strings.Split(want, "\n") {
		wantSet[l] = true
	}
	gotSet := make(map[string]bool)
	for _, l := range strings.Split(got, "\n") {
		gotSet[l] = true
	}
	var b strings.Builder
	for _, l := range strings.Split(want, "\n") {
		if l != "" && !gotSet[l] {
			fmt.Fprintf(&b, "- %s\n", l)
		}
	}
	for _, l := range strings.Split(got, "\n") {
		if l != "" && !wantSet[l] {
			fmt.Fprintf(&b, "+ %s\n", l)
		}
	}
	return b.String()
}
