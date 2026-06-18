// Package initcfg — source patcher for .archfit.yaml files.
// ApplyEdits locates nodes via goccy AST, then mutates the original bytes
// via line splices so comments, formatting, and untargeted sections are preserved.
package initcfg

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// Edit is a sealed mutation to apply to an .archfit.yaml source.
// Only the types defined in this package satisfy Edit.
type Edit interface{ editMarker() }

// ModuleField is the set of module scalar fields that SetModuleFieldsEdit can insert.
type ModuleField int

// ModuleField enum values in canonical order.
const (
	FieldSubdomain  ModuleField = iota // subdomain: ...
	FieldVolatility                    // volatility: ...
	FieldLayer                         // layer: ...
	FieldReviewedAt                    // reviewed_at: 2026-06-18T...
	FieldReviewedBy                    // reviewed_by: ...
)

// AddModuleEdit inserts a new module stanza. NO-OP if the module key already exists live.
type AddModuleEdit struct {
	Def ModuleDef
	Ann *ModuleAnnotation // may be nil
}

func (AddModuleEdit) editMarker() {}

// SetModuleFieldsEdit inserts absent scalar fields (subdomain/volatility/layer) into
// an existing module. All absent fields are coalesced into ONE block insertion,
// in canonical order subdomain→volatility→layer. Skips Layer if value not in layers:.
type SetModuleFieldsEdit struct {
	Module string
	Fields map[ModuleField]string
}

func (SetModuleFieldsEdit) editMarker() {}

// UpdateModulePathsEdit replaces the paths: block of an existing module.
// If the module exists but has no paths:, inserts block-style paths: as first key.
// REJECTS inline flow-style paths: [...].
type UpdateModulePathsEdit struct {
	Module string
	Paths  []string
}

func (UpdateModulePathsEdit) editMarker() {}

// CommentModuleEdit prefixes the module's full source range (incl. head comment)
// with a removal marker. NO-OP if the marker already exists.
type CommentModuleEdit struct {
	Module string
	Note   string
}

func (CommentModuleEdit) editMarker() {}

// ---------------------------------------------------------------------------
// Internal state gathered from one parse pass
// ---------------------------------------------------------------------------

// parsedFile holds everything ApplyEdits needs from the AST.
type parsedFile struct {
	allowedLayers []string
	totalLines    int // number of lines in src (1-based max)

	// modules block
	modulesKeyLine int // 1-based line of "modules:" key; 0 = absent
	modulesEndLine int // 1-based exclusive end of the modules: block
	modules        []parsedModule

	// positions of other top-level keys (1-based), for insertion-location logic
	layersKeyLine int
	layersEndLine int
	toolsKeyLine  int
	toolsEndLine  int
	rulesKeyLine  int
}

type parsedModule struct {
	name      string
	keyLine   int // 1-based, the "name:" key line
	headStart int // 1-based; first line of head comment (== keyLine if no head comment)
	endLine   int // 1-based exclusive end (next sibling key line or modules block end)

	// paths block
	pathsKeyLine   int             // 0 = absent
	pathsEndLine   int             // 1-based exclusive end of paths block
	pathsIsFlow    bool            // true = paths: [...] inline — reject
	pathsExists    bool            //
	existingFields map[string]bool // field keys present in the module body
}

// splice is a single line-range replacement to apply to the source.
type splice struct {
	startLine   int // 1-based inclusive
	endLine     int // 1-based exclusive
	replacement []byte
	editDesc    string // for overlap error messages
}

// ---------------------------------------------------------------------------
// ApplyEdits
// ---------------------------------------------------------------------------

// ApplyEdits applies a list of edits to src in a single bottom-up pass.
// The original bytes outside targeted line ranges are preserved exactly,
// keeping comments, key order, rules, and exceptions intact.
func ApplyEdits(src []byte, edits []Edit) ([]byte, error) {
	lines := splitLines(src)
	pf, err := parseFile(src, lines)
	if err != nil {
		return nil, err
	}

	var splices []splice

	// When there is no existing modules: block, all AddModuleEdits must share a
	// single "modules:\n" header. Collect them first, coalesce into one splice,
	// then process the remaining edits normally.
	if pf.modulesKeyLine == 0 {
		var addEdits []AddModuleEdit
		var rest []Edit
		for _, e := range edits {
			if ae, ok := e.(AddModuleEdit); ok {
				addEdits = append(addEdits, ae)
			} else {
				rest = append(rest, e)
			}
		}
		if len(addEdits) > 0 {
			sp := resolveAddModulesNoSection(addEdits, pf)
			if sp != nil {
				splices = append(splices, *sp)
			}
		}
		edits = rest
	}

	for _, e := range edits {
		var sp *splice
		switch ed := e.(type) {
		case AddModuleEdit:
			sp = resolveAddModule(ed, pf)
		case SetModuleFieldsEdit:
			sp, err = resolveSetModuleFields(ed, pf)
		case UpdateModulePathsEdit:
			sp, err = resolveUpdateModulePaths(ed, pf)
		case CommentModuleEdit:
			sp, err = resolveCommentModule(ed, pf, src, lines)
		default:
			return nil, fmt.Errorf("yamledit: unknown edit type %T", e)
		}
		if err != nil {
			return nil, err
		}
		if sp != nil {
			splices = append(splices, *sp)
		}
	}

	if err := checkOverlaps(splices); err != nil {
		return nil, err
	}

	// Sort bottom-up (descending start line) so replacements don't shift offsets.
	sort.SliceStable(splices, func(i, j int) bool {
		if splices[i].startLine != splices[j].startLine {
			return splices[i].startLine > splices[j].startLine
		}
		// Tie-break: ascending editDesc for deterministic ordering of same-line inserts.
		return splices[i].editDesc < splices[j].editDesc
	})

	for _, sp := range splices {
		lo := sp.startLine - 1 // 0-based inclusive
		hi := sp.endLine - 1   // 0-based exclusive
		if lo < 0 {
			lo = 0
		}
		if hi > len(lines) {
			hi = len(lines)
		}
		repLines := splitLines(sp.replacement)
		newLines := make([][]byte, 0, len(lines)-(hi-lo)+len(repLines))
		newLines = append(newLines, lines[:lo]...)
		newLines = append(newLines, repLines...)
		newLines = append(newLines, lines[hi:]...)
		lines = newLines
	}

	return joinLines(lines), nil
}

// ---------------------------------------------------------------------------
// Parse pass
// ---------------------------------------------------------------------------

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

		case "tools":
			pf.toolsKeyLine = keyLine
			pf.toolsEndLine = endLineOf(keyLine)

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

// ---------------------------------------------------------------------------
// Edit resolvers
// ---------------------------------------------------------------------------

// resolveAddModulesNoSection coalesces multiple AddModuleEdits when there is no
// existing modules: block into a single splice with one "modules:\n" header.
// Stanzas are emitted in alphabetical order for deterministic output.
// Modules that already exist in pf are silently skipped (NO-OP semantics).
func resolveAddModulesNoSection(eds []AddModuleEdit, pf parsedFile) *splice {
	// Sort by module name for deterministic output.
	sorted := make([]AddModuleEdit, len(eds))
	copy(sorted, eds)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Def.Name < sorted[j].Def.Name
	})

	var b strings.Builder
	b.WriteString("\nmodules:\n")
	wrote := 0
	for _, ed := range sorted {
		if pf.findModule(ed.Def.Name) != nil {
			continue // already exists — skip
		}
		writeModuleStanza(&b, "  ", ed.Def.Name, ed.Def, pf.allowedLayers, ed.Ann, true)
		wrote++
	}
	if wrote == 0 {
		return nil
	}

	insertBefore := insertionPoint(pf)
	return &splice{
		startLine:   insertBefore,
		endLine:     insertBefore,
		replacement: []byte(b.String()),
		editDesc:    "AddModules+modules-section",
	}
}

func resolveAddModule(ed AddModuleEdit, pf parsedFile) *splice {
	// NO-OP if module already exists live.
	if pf.findModule(ed.Def.Name) != nil {
		return nil
	}

	var b strings.Builder
	writeModuleStanza(&b, "  ", ed.Def.Name, ed.Def, pf.allowedLayers, ed.Ann, true)
	stanza := b.String()

	if pf.modulesKeyLine > 0 {
		// Append into the existing modules: block, just before its end.
		insertLine := pf.modulesEndLine
		return &splice{
			startLine:   insertLine,
			endLine:     insertLine,
			replacement: []byte(stanza),
			editDesc:    fmt.Sprintf("AddModule(%s)", ed.Def.Name),
		}
	}

	// No modules: section — create one.
	insertBefore := insertionPoint(pf)
	block := "\nmodules:\n" + stanza
	return &splice{
		startLine:   insertBefore,
		endLine:     insertBefore,
		replacement: []byte(block),
		editDesc:    fmt.Sprintf("AddModule(%s)+modules-section", ed.Def.Name),
	}
}

// insertionPoint returns the 1-based line before which to insert a new modules: block.
func insertionPoint(pf parsedFile) int {
	switch {
	case pf.layersKeyLine > 0:
		return pf.layersEndLine
	case pf.toolsKeyLine > 0:
		return pf.toolsEndLine
	case pf.rulesKeyLine > 0:
		return pf.rulesKeyLine
	default:
		return pf.totalLines + 1
	}
}

func resolveSetModuleFields(ed SetModuleFieldsEdit, pf parsedFile) (*splice, error) {
	pm := pf.findModule(ed.Module)
	if pm == nil {
		return nil, fmt.Errorf("yamledit: SetModuleFields: module %q not found", ed.Module)
	}

	allowedSet := make(map[string]bool, len(pf.allowedLayers))
	for _, l := range pf.allowedLayers {
		allowedSet[l] = true
	}

	type fieldLine struct{ key, value string }
	var toInsert []fieldLine

	// Canonical order: subdomain → volatility → layer → reviewed_at → reviewed_by.
	for _, mf := range []ModuleField{FieldSubdomain, FieldVolatility, FieldLayer, FieldReviewedAt, FieldReviewedBy} {
		val, ok := ed.Fields[mf]
		if !ok || val == "" {
			continue
		}
		key := moduleFieldKey(mf)
		if mf == FieldLayer && !allowedSet[val] {
			continue // skip out-of-set layer
		}
		if pm.existingFields[key] {
			continue // never overwrite an existing field
		}
		toInsert = append(toInsert, fieldLine{key: key, value: val})
	}

	if len(toInsert) == 0 {
		return nil, nil
	}

	// Fields are children of the module key (which is at 2-space indent),
	// so they belong at 4-space indent — same as paths:, layer:, etc.
	var b strings.Builder
	for _, fl := range toInsert {
		fmt.Fprintf(&b, "    %s: %s\n", fl.key, yamlScalarQuote(fl.value))
	}

	// Insert immediately before the end of this module's stanza (i.e., before
	// the next sibling module key or the end of the modules: block). This places
	// the new fields inside the module body at the correct indentation.
	return &splice{
		startLine:   pm.endLine,
		endLine:     pm.endLine,
		replacement: []byte(b.String()),
		editDesc:    fmt.Sprintf("SetModuleFields(%s)", ed.Module),
	}, nil
}

// moduleFieldKey returns the YAML key string for a ModuleField constant.
func moduleFieldKey(mf ModuleField) string {
	switch mf {
	case FieldSubdomain:
		return "subdomain"
	case FieldVolatility:
		return "volatility"
	case FieldLayer:
		return "layer"
	case FieldReviewedAt:
		return "reviewed_at"
	case FieldReviewedBy:
		return "reviewed_by"
	default:
		return ""
	}
}

func resolveUpdateModulePaths(ed UpdateModulePathsEdit, pf parsedFile) (*splice, error) {
	pm := pf.findModule(ed.Module)
	if pm == nil {
		return nil, fmt.Errorf("yamledit: UpdateModulePaths: module %q not found", ed.Module)
	}
	if pm.pathsIsFlow {
		return nil, fmt.Errorf("yamledit: UpdateModulePaths: module %q has flow-style paths: [...] which is not supported", ed.Module)
	}

	var b strings.Builder
	b.WriteString("    paths:\n")
	for _, p := range ed.Paths {
		fmt.Fprintf(&b, "      - %q\n", p)
	}

	if pm.pathsExists {
		return &splice{
			startLine:   pm.pathsKeyLine,
			endLine:     pm.pathsEndLine,
			replacement: []byte(b.String()),
			editDesc:    fmt.Sprintf("UpdateModulePaths(%s)", ed.Module),
		}, nil
	}

	// No existing paths block: insert right after the module key line.
	insertLine := pm.keyLine + 1
	return &splice{
		startLine:   insertLine,
		endLine:     insertLine,
		replacement: []byte(b.String()),
		editDesc:    fmt.Sprintf("UpdateModulePaths(%s)+insert", ed.Module),
	}, nil
}

func resolveCommentModule(ed CommentModuleEdit, pf parsedFile, src []byte, lines [][]byte) (*splice, error) {
	// Idempotency: marker already present → no-op (check before module lookup so
	// a re-apply on an already-commented file is always a no-op, not an error).
	marker := fmt.Sprintf(`# archfit: removed module %q`, ed.Module)
	if strings.Contains(string(src), marker) {
		return nil, nil
	}

	pm := pf.findModule(ed.Module)
	if pm == nil {
		return nil, fmt.Errorf("yamledit: CommentModule: module %q not found", ed.Module)
	}

	note := sanitizeComment(ed.Note)
	var b strings.Builder
	fmt.Fprintf(&b, "# archfit: removed module %q — verify before deleting (%s)\n", ed.Module, note)

	lo := pm.headStart - 1 // 0-based
	hi := pm.endLine - 1   // 0-based exclusive
	if lo < 0 {
		lo = 0
	}
	if hi > len(lines) {
		hi = len(lines)
	}
	for _, line := range lines[lo:hi] {
		trimmed := bytes.TrimLeft(line, " \t")
		if len(trimmed) == 0 {
			b.WriteString("#\n")
		} else {
			b.WriteString("# ")
			b.Write(trimmed)
			b.WriteByte('\n')
		}
	}

	return &splice{
		startLine:   pm.headStart,
		endLine:     pm.endLine,
		replacement: []byte(b.String()),
		editDesc:    fmt.Sprintf("CommentModule(%s)", ed.Module),
	}, nil
}

// ---------------------------------------------------------------------------
// Overlap detection
// ---------------------------------------------------------------------------

func checkOverlaps(splices []splice) error {
	for i := 0; i < len(splices); i++ {
		for j := i + 1; j < len(splices); j++ {
			a, b := splices[i], splices[j]
			if a.endLine > b.startLine && b.endLine > a.startLine {
				return fmt.Errorf("yamledit: conflicting edits overlap: %s [%d,%d) and %s [%d,%d)",
					a.editDesc, a.startLine, a.endLine,
					b.editDesc, b.startLine, b.endLine)
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Line utilities
// ---------------------------------------------------------------------------

// splitLines splits src on '\n'. The trailing empty element (when src ends with '\n')
// is kept so joinLines reconstructs the original trailing newline.
func splitLines(src []byte) [][]byte {
	if len(src) == 0 {
		return nil
	}
	return bytes.Split(src, []byte("\n"))
}

// joinLines rejoins lines with '\n', preserving the trailing newline captured
// by splitLines as a trailing empty element.
func joinLines(lines [][]byte) []byte {
	return bytes.Join(lines, []byte("\n"))
}

// ---------------------------------------------------------------------------
// YAML scalar quoting helper
// ---------------------------------------------------------------------------

// yamlScalarQuote returns v bare if it is safe to write unquoted in YAML,
// or double-quoted otherwise.
func yamlScalarQuote(v string) string {
	if v == "" {
		return `""`
	}
	for _, r := range v {
		switch r {
		case ':', '#', '[', ']', '{', '}', '&', '*', '!', '|', '>', '\'', '"', '%', '@', '`':
			return quoteDouble(v)
		}
	}
	if v[0] == ' ' || v[len(v)-1] == ' ' {
		return quoteDouble(v)
	}
	return v
}

func quoteDouble(v string) string {
	escaped := strings.ReplaceAll(v, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	escaped = strings.ReplaceAll(escaped, "\r", `\r`)
	escaped = strings.ReplaceAll(escaped, "\t", `\t`)
	return `"` + escaped + `"`
}

// yamlScalar returns s unquoted when it matches the safe plain-scalar pattern
// (starts with a letter or digit, contains only [A-Za-z0-9_.-]). This covers
// all normal layer/subdomain/volatility names (core, adapter, engine, low, …)
// and guarantees they are byte-identical before and after a config round-trip.
// All other values are passed through yamlScalarQuote for safe double-quoting.
func yamlScalar(s string) string {
	if safePlainScalar(s) {
		return s
	}
	return yamlScalarQuote(s)
}

// safePlainScalar reports whether s can be written as a bare YAML scalar
// without any quoting: non-empty, starts with [A-Za-z0-9], contains only
// [A-Za-z0-9_.-].
func safePlainScalar(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
				return false
			}
		} else {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '.' && r != '-' {
				return false
			}
		}
	}
	return true
}
