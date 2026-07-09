package initcfg

// Edit resolvers that translate Edit values into splices (see yamledit.go).

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

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
		writeModuleStanza(&b, ed.Def.Name, ed.Def, pf.allowedLayers, ed.Ann, true)
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
	writeModuleStanza(&b, ed.Def.Name, ed.Def, pf.allowedLayers, ed.Ann, true)
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
	case pf.analysisKeyLine > 0:
		return pf.analysisEndLine
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

	// Canonical order: subdomain → volatility → owner → layer → reviewed_at → reviewed_by.
	for _, mf := range []ModuleField{FieldSubdomain, FieldVolatility, FieldOwner, FieldLayer, FieldReviewedAt, FieldReviewedBy} {
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
	case FieldOwner:
		return "owner"
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
	var b strings.Builder
	b.WriteString("    paths:\n")
	for _, p := range ed.Paths {
		fmt.Fprintf(&b, "      - %q\n", p)
	}

	if pm.pathsIsFlow {
		return &splice{
			startLine:   pm.pathsFlowLine,
			endLine:     pm.pathsFlowLine + 1,
			replacement: []byte(b.String()),
			editDesc:    fmt.Sprintf("UpdateModulePaths(%s)", ed.Module),
		}, nil
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
