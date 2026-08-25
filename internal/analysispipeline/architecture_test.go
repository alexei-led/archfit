package pipeline_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const configImport = "github.com/alexei-led/archfit/internal/config"

func TestArchitecturePolicyBoundaries(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(file)
	projectRoot := filepath.Clean(filepath.Join(root, "../.."))
	checkImports := func(dir string, forbidden map[string]bool, allow map[string]bool) {
		t.Helper()
		entries, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range entries {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range f.Imports {
				name := strings.Trim(imp.Path.Value, `"`)
				if forbidden[name] && !allow[filepath.Base(path)] {
					t.Errorf("%s imports forbidden %s", path, name)
				}
			}
		}
	}
	checkImports(filepath.Join(projectRoot, "internal/analysispipeline"), map[string]bool{
		configImport: true,
	}, map[string]bool{"adapter_config.go": true, "analyzer.go": true, "stage_prepare.go": true, "worktree.go": true})
	checkImports(filepath.Join(projectRoot, "internal/policy"), map[string]bool{
		configImport:       true,
		"gopkg.in/yaml.v3": true,
		"os":               true,
		"os/exec":          true,
	}, nil)
}

func TestStageInputHasSinglePolicySource(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	engineFile := filepath.Clean(filepath.Join(filepath.Dir(file), "stages.go"))
	tree, err := parser.ParseFile(token.NewFileSet(), engineFile, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range tree.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok.String() != "type" {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			if typeSpec.Name.Name != "StageInput" {
				continue
			}
			st := typeSpec.Type.(*ast.StructType)
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 {
					continue
				}
				switch field.Names[0].Name {
				case "Classify", "Staleness", "Waivers", "MetricGates", "ApprovedLabels", "Labels":
					t.Errorf("StageInput retains duplicate policy field %s", field.Names[0].Name)
				}
			}
		}
	}
}
