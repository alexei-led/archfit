package evaluation

import (
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/policy"
)

// InventoryFile is one file of the rule-scope source inventory, projected the
// way rule scope and config lint read it.
type InventoryFile struct {
	// Path is the repo-relative slash path.
	Path string
	// Language is the source language the file's extension names.
	Language string
	// Selector is the graph-node path rule selectors and public: globs match
	// for the file; empty when the inventory cannot name it (a Rust file
	// without cargo metadata).
	Selector string
	// Class is the file's FileClass; a file outside the class index is
	// production, as for every other Production-file metric.
	Class fileclass.FileClass
}

// SourceInventory lists the rule-scope source inventory of f, sorted by path:
// every walked supported source file inside the declared analysis scope, with
// the selector rule scope matches for it. config init and config update read
// it to propose only modules this inventory holds source for, so the config
// they write is judged by lint and check over the same files.
func SourceInventory(f Observations) []InventoryFile {
	files := sourceInventoryFiles(f)
	out := make([]InventoryFile, 0, len(files))
	for _, file := range files {
		language, selector, supported := ruleFileSelector(policy.ModuleMap{}, file, f.SourceSelectors)
		if !supported {
			continue
		}
		out = append(out, InventoryFile{Path: file, Language: language, Selector: selector, Class: classOf(f.FileClassIndex, file)})
	}
	return out
}
