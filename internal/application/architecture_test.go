package application

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationAnalysisContractStaysNarrow(t *testing.T) {
	data, err := os.ReadFile("analysis.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, forbidden := range []string{"internal/assessment/rules", "internal/assessment/metrics", "internal/assessment/signals", "internal/assessment/status", "report.MetricSnapshot"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("analysis.go contains forbidden dependency %q", forbidden)
		}
	}
}

func TestApplicationStageContractsHaveNoBroadTypes(t *testing.T) {
	forbiddenTypes := map[string]bool{"EvidenceStage" + "Result": true, "RelationshipStage" + "Result": true}
	forbiddenFields := map[string]bool{"Config": true, "PreAugmentedModules": true, "CloneEvidence": true}
	broadType := "an" + "y"
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		tree, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range tree.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok.String() != "type" {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				if forbiddenTypes[typeSpec.Name.Name] {
					t.Errorf("%s defines forbidden compatibility type %s", path, typeSpec.Name.Name)
				}
				st, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range st.Fields.List {
					if len(field.Names) == 0 {
						continue
					}
					if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == broadType {
						t.Errorf("%s field %s uses a broad empty type", path, field.Names[0].Name)
					}
					if sel, ok := field.Type.(*ast.InterfaceType); ok && sel.Methods != nil && len(sel.Methods.List) == 0 {
						t.Errorf("%s field %s uses interface{}", path, field.Names[0].Name)
					}
					for _, name := range field.Names {
						if forbiddenFields[name.Name] {
							t.Errorf("%s field %s is a forbidden broad fallback", path, name.Name)
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplicationNoConcreteAdapters(t *testing.T) {
	forbidden := []string{
		"github.com/alexei-led/archfit/internal/analysispipeline",
		"github.com/alexei-led/archfit/internal/extract/",
		"github.com/alexei-led/archfit/internal/factcache",
		"github.com/alexei-led/archfit/internal/history/",
		"github.com/alexei-led/archfit/internal/ownership",
		"github.com/alexei-led/archfit/internal/labels/labelsio",
		"github.com/alexei-led/archfit/internal/baseline",
		"github.com/alexei-led/archfit/internal/llm",
		"github.com/alexei-led/archfit/internal/config",
	}
	root := "."
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		tree, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range tree.Imports {
			name := strings.Trim(imp.Path.Value, `"`)
			for _, prefix := range forbidden {
				if name == prefix || strings.HasPrefix(name, prefix) {
					t.Errorf("%s imports concrete adapter %s", path, name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
