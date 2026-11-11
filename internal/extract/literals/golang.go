package literals

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
)

// readGo returns the string literals of one Go file. Import paths and struct
// tags are not literals; comments are not nodes of the tree, so they are never
// read. Lines are physical: a //line directive does not move them. A file
// that does not parse gives no literal and parseErr; err is a file it cannot
// read.
func readGo(fsys fs.FS, file string) (lits []Literal, parseErr, err error) {
	src, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, nil, fmt.Errorf("literals: read %s: %w", file, err)
	}
	fset := token.NewFileSet()
	f, parseErr := parser.ParseFile(fset, file, src, parser.SkipObjectResolution)
	if parseErr != nil {
		return nil, parseErr, nil
	}
	skip := map[token.Pos]bool{} // import paths and struct tags
	ast.Inspect(f, func(n ast.Node) bool {
		// Inspect visits a spec or field before its children, so skip is set
		// before the path or tag itself is visited.
		switch n := n.(type) {
		case *ast.ImportSpec:
			skip[n.Path.ValuePos] = true
		case *ast.Field:
			if n.Tag != nil {
				skip[n.Tag.ValuePos] = true
			}
		case *ast.BasicLit:
			if n.Kind == token.STRING && !skip[n.ValuePos] && !strings.Contains(n.Value, "\n") {
				lits = append(lits, Literal{File: file, Line: fset.PositionFor(n.ValuePos, false).Line, Value: n.Value[1 : len(n.Value)-1]})
			}
		}
		return true
	})
	return lits, nil, nil
}
