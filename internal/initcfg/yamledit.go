// Package initcfg — source patcher for .archfit.yaml files.
// ApplyEdits locates nodes via goccy AST, then mutates the original bytes
// via line splices so comments, formatting, and untargeted sections are preserved.
package initcfg

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
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
	FieldOwner                         // owner: ...
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
