package initcfg

// AST parse pass for ApplyEdits (see yamledit.go).

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// parseFile performs one AST pass over src to collect all positions needed
// to resolve and validate edits.
func parseFile(src []byte, lines [][]byte) (parsedFile, error) {
	pf := parsedFile{totalLines: len(lines)}

	if len(bytes.TrimSpace(src)) == 0 {
		return pf, nil
	}

	f, err := parser.ParseBytes(src, parser.ParseComments)
	if err != nil {
		return pf, fmt.Errorf("yamledit: parse: %w", err)
	}
	if len(f.Docs) == 0 {
		return pf, nil
	}

	body, ok := f.Docs[0].Body.(*ast.MappingNode)
	if !ok {
		return pf, errors.New("yamledit: document body is not a mapping")
	}

	// Collect top-level key positions to compute section end lines.
	topKeyLines := collectTopKeyLines(body)
	endLineOf := func(keyLine int) int {
		next := pf.totalLines + 1
		for _, l := range topKeyLines {
			if l > keyLine && l < next {
				next = l
			}
		}
		return next
	}

	for _, mv := range body.Values {
		sn, ok := mv.Key.(*ast.StringNode)
		if !ok {
			continue
		}
		keyLine := mv.Key.GetToken().Position.Line

		switch sn.Value {
		case "layers":
			pf.layersKeyLine = keyLine
			pf.layersEndLine = endLineOf(keyLine)
			pf.allowedLayers = readLayersSeq(mv.Value)

		case "coupling", "languages", "analyzers", "ai":
			// Pre-modules config sections appear in document order; keep the last
			// one as the anchor so a new modules: block lands right after it.
			pf.analysisKeyLine = keyLine
			pf.analysisEndLine = endLineOf(keyLine)

		case "rules":
			pf.rulesKeyLine = keyLine

		case "modules":
			if err := pf.parseModulesBlock(mv, keyLine, endLineOf, lines); err != nil {
				return pf, err
			}
		}
	}

	return pf, nil
}

// collectTopKeyLines returns the 1-based line number of each top-level mapping key.
func collectTopKeyLines(body *ast.MappingNode) []int {
	lines := make([]int, 0, len(body.Values))
	for _, mv := range body.Values {
		if _, ok := mv.Key.(*ast.StringNode); ok {
			lines = append(lines, mv.Key.GetToken().Position.Line)
		}
	}
	return lines
}

// readLayersSeq extracts string values from a layers: sequence node.
func readLayersSeq(v ast.Node) []string {
	seq, ok := v.(*ast.SequenceNode)
	if !ok {
		return nil
	}
	var out []string
	for _, entry := range seq.Values {
		if s, ok := entry.(*ast.StringNode); ok {
			out = append(out, s.Value)
		}
	}
	return out
}

// parseModulesBlock populates pf with module positions from the modules: mapping value.
func (pf *parsedFile) parseModulesBlock(
	mv *ast.MappingValueNode,
	keyLine int,
	endLineOf func(int) int,
	lines [][]byte,
) error {
	pf.modulesKeyLine = keyLine
	pf.modulesEndLine = endLineOf(keyLine)

	// Reject flow-style modules: {} (value token on same line as key).
	if vt := mv.Value.GetToken(); vt != nil && vt.Position.Line == keyLine {
		return errors.New("yamledit: flow-style modules: {} is not supported; use block style")
	}

	modulesMap, ok := mv.Value.(*ast.MappingNode)
	if !ok {
		return nil
	}

	for i, modMV := range modulesMap.Values {
		modSN, ok := modMV.Key.(*ast.StringNode)
		if !ok {
			continue
		}
		modEndLine := pf.modulesEndLine
		if i+1 < len(modulesMap.Values) {
			modEndLine = modulesMap.Values[i+1].Key.GetToken().Position.Line
		}
		pm := parseModuleEntry(modSN.Value, modMV, modEndLine, lines)
		pf.modules = append(pf.modules, pm)
	}
	return nil
}

// parseModuleEntry builds a parsedModule from a single module mapping-value node.
func parseModuleEntry(name string, modMV *ast.MappingValueNode, modEndLine int, lines [][]byte) parsedModule {
	modKeyLine := modMV.Key.GetToken().Position.Line

	// Walk backwards from keyLine to find head comments.
	headStart := modKeyLine
	for headStart > 1 {
		candidate := headStart - 2 // convert to 0-based for the line ABOVE
		if candidate < 0 || candidate >= len(lines) {
			break
		}
		if strings.HasPrefix(strings.TrimSpace(string(lines[candidate])), "#") {
			headStart--
		} else {
			break
		}
	}

	pm := parsedModule{
		name:           name,
		keyLine:        modKeyLine,
		headStart:      headStart,
		endLine:        modEndLine,
		existingFields: make(map[string]bool),
	}

	modBody, ok := modMV.Value.(*ast.MappingNode)
	if !ok {
		return pm
	}

	for j, fieldMV := range modBody.Values {
		fieldSN, ok := fieldMV.Key.(*ast.StringNode)
		if !ok {
			continue
		}
		pm.existingFields[fieldSN.Value] = true

		if fieldSN.Value != "paths" {
			continue
		}
		pathsKeyLine := fieldMV.Key.GetToken().Position.Line
		pm.pathsKeyLine = pathsKeyLine
		pm.pathsExists = true

		// paths block ends at the next field in the module, or at modEndLine.
		pm.pathsEndLine = modEndLine
		if j+1 < len(modBody.Values) {
			pm.pathsEndLine = modBody.Values[j+1].Key.GetToken().Position.Line
		}

		// Flow-style: paths value token on the same line as the paths key.
		if sv := fieldMV.Value; sv != nil {
			if _, isSeq := sv.(*ast.SequenceNode); isSeq {
				if sv.GetToken().Position.Line == pathsKeyLine {
					pm.pathsIsFlow = true
				}
			}
		}
	}

	return pm
}

// findModule returns a pointer to the parsed module with the given name, or nil.
func (pf *parsedFile) findModule(name string) *parsedModule {
	for i := range pf.modules {
		if pf.modules[i].name == name {
			return &pf.modules[i]
		}
	}
	return nil
}
