package evidence

import (
	modelclone "github.com/alexei-led/archfit/internal/model/clone"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
)

// SizeSignals carries neutral per-file source facts.
type SizeSignals struct {
	FileLOC        map[string]int
	FileClassIndex map[string]fileclass.FileClass
}

// DuplicationSignals carries neutral clone facts.
type DuplicationSignals struct{ Clusters []modelclone.Cluster }

// DynamicImportSignals carries neutral dynamic-import facts.
type DynamicImportSignals struct {
	Sites []modevidence.DynamicImportSite
}

// RuntimeAsyncSignals carries neutral runtime integration facts.
type RuntimeAsyncSignals struct {
	Sites      []modevidence.RuntimeAsyncSite
	Confidence string
}

// RunSignals is the neutral producer-side bundle assembled before assessment.
type RunSignals struct {
	Size           SizeSignals
	Duplication    DuplicationSignals
	ExtraCoverage  []modevidence.Coverage
	DynamicImports DynamicImportSignals
	RuntimeAsync   RuntimeAsyncSignals
	DeprecatedDeps []modevidence.DeprecatedDep
}
