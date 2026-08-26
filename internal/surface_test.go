// Package arch_test also owns the published-surface gate for the migrated
// capability packages: an exported declaration is a promise to a consumer, and
// a promise nobody consumes is migration residue.
package arch_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/model/report"
)

// surfaceOwners are the packages the capability migration reshaped. Every
// exported declaration in them must have a consumer outside its own package —
// otherwise it is either dead code or a compatibility alias the migration was
// supposed to delete.
var surfaceOwners = []string{
	modulePrefix + "internal/application",
	modulePrefix + "internal/evidence",
	modulePrefix + "internal/evidence/acquisition",
	modulePrefix + "internal/assessment/evaluation",
	modulePrefix + "internal/relationship/analysis",
}

// TestMigratedPackagesPublishNoUnusedBehavior is the published-surface gate.
// It checks exported FUNCTIONS and package-level values, not types: a type is a
// contract a caller may name at any time, but an exported function nobody calls
// is behavior with no consumer — dead code, or a compatibility shim the
// migration was meant to delete.
func TestMigratedPackagesPublishNoUnusedBehavior(t *testing.T) {
	loaded, err := packages.Load(&packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: ".."}, surfaceOwners...)
	if err != nil {
		t.Fatalf("load migrated packages: %v", err)
	}
	uses := identifierUses(t)
	for _, pkg := range loaded {
		if pkg.Types == nil {
			t.Fatalf("no type information for %s", pkg.PkgPath)
		}
		dir := strings.TrimPrefix(pkg.PkgPath, modulePrefix) + "/"
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if !obj.Exported() {
				continue
			}
			if _, isFunc := obj.(*types.Func); !isFunc {
				continue
			}
			if !usedOutside(uses[name], dir) {
				t.Errorf("%s.%s is exported but never called outside its package: give it a caller or unexport it", pkg.PkgPath, name)
			}
		}
	}
}

// TestMigratedPackagesPublishNoCompatibilityAliases rejects the other shape of
// migration residue: an exported alias that keeps an old name alive next to its
// new owner. Two names for one contract is exactly how the previous horizon's
// façade survived longer than it should have.
func TestMigratedPackagesPublishNoCompatibilityAliases(t *testing.T) {
	for _, owner := range surfaceOwners {
		dir := filepath.Join("..", strings.TrimPrefix(owner, modulePrefix))
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test"+goSourceExt) {
				continue
			}
			tree, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			for _, decl := range tree.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.TYPE {
					continue
				}
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || !ts.Assign.IsValid() || !ts.Name.IsExported() {
						continue
					}
					if _, allowed := aliasExempt[ts.Name.Name]; !allowed {
						t.Errorf("%s declares the exported alias %s: name a contract once, in the package that owns it", path, ts.Name.Name)
					}
				}
			}
		}
	}
}

// aliasExempt names the one alias the evidence contract keeps deliberately.
var aliasExempt = map[string]string{
	"Coverage": "internal/evidence re-exports the model coverage type as the stage contract's own",
}

// identifierUses maps every qualified identifier written in the repository to
// the files that write it. It is a textual index on purpose: it sees test files
// and cmd wiring the type checker would need a second load to reach.
func identifierUses(t *testing.T) map[string][]string {
	t.Helper()
	uses := map[string][]string{}
	qualified := regexp.MustCompile(`\b[a-z][A-Za-z0-9_]*\.([A-Z][A-Za-z0-9_]*)\b`)
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case dirGit, dirFactCache, ".bin", "docs", dirVendor:
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != goSourceExt {
			return nil
		}
		data, readErr := readSource(path)
		if readErr != nil {
			return readErr
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, "../"))
		for _, m := range qualified.FindAllStringSubmatch(data, -1) {
			uses[m[1]] = append(uses[m[1]], rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return uses
}

func usedOutside(files []string, dir string) bool {
	for _, file := range files {
		if !strings.HasPrefix(file, dir) {
			return true
		}
	}
	return false
}

func readSource(path string) (string, error) {
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	ast.Inspect(tree, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				b.WriteString(ident.Name + "." + sel.Sel.Name + "\n")
			}
		}
		return true
	})
	return b.String(), nil
}

// TestScorecardRubricVersionsAgree pins the two copies of the rubric version
// equal. The write path stamps score.RubricVersion into the stored baseline
// snapshot; the read path compares it against report.RubricVersion. score is a
// core-ring package and cannot import the report contract, so the constant is
// duplicated — a one-sided bump would make every stored baseline read as an
// incompatible snapshot and silently skip the coupling gate's max_drop check.
func TestScorecardRubricVersionsAgree(t *testing.T) {
	t.Parallel()

	if score.RubricVersion != report.RubricVersion {
		t.Fatalf("score.RubricVersion = %d, report.RubricVersion = %d; bump both or neither",
			score.RubricVersion, report.RubricVersion)
	}
}
