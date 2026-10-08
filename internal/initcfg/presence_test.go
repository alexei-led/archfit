package initcfg

import (
	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
)

// Names the retired archfit-specific Go layer table and the retired Python
// sub-package layer guess used; tests still use them as plain module and
// layer names.
const (
	adapterExtract = "extract"
	layerEngine    = "engine"
	layerModel     = "model"
	layerCore      = "core"
	layerAdapter   = "adapter"
	layerCmd       = "cmd"
)

// ProbePresence answers language presence the way cmd's languagePresence
// does: through the extractor registry's applicability functions, here with a
// default extractor config. Exported from a test file so the external test
// package can use it too.
func ProbePresence(root string) Presence {
	var cfg evidenceports.ExtractConfig
	p := Presence{
		Go:         registry.ProjectPresent(langGo, root, cfg),
		TypeScript: registry.ProjectPresent(langTypeScript, root, cfg),
		Python:     registry.ProjectPresent(langPython, root, cfg),
		Rust:       registry.ProjectPresent(langRust, root, cfg),
	}
	if p.Go {
		p.GoMembers, p.GoWorkOff = registry.GoMembers(root, cfg)
	}
	return p
}
