package acquisition

import (
	evidencecontract "github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
)

func sourceSelectorsOf(f evidencecontract.Facts) map[string]string {
	var roots []graph.CrateRoot
	if f.Graph != nil {
		roots = f.Graph.CrateRoots()
	}
	selectors := make(map[string]string, len(f.FileLOC)+len(f.FileClassIndex))
	mm := policy.ModuleMap{}
	add := func(file string) {
		language, selector, supported := mm.RuleSelectorForFile(file, roots...)
		if supported {
			if language == graph.LangRust && len(roots) == 0 {
				selector = ""
			}
			selectors[file] = selector
		}
	}
	for file := range f.FileLOC {
		add(file)
	}
	for file := range f.FileClassIndex {
		add(file)
	}
	return selectors
}
