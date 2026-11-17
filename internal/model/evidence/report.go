package evidence

// ReportOrigin separates observations, declarations, proxies and defaults.
type ReportOrigin string

// Report origins describe the source of a reported value.
const (
	ReportObserved ReportOrigin = "observed"
	ReportAsserted ReportOrigin = "asserted"
	ReportAssessed ReportOrigin = "assessed"
	ReportAssumed  ReportOrigin = "assumed"
	ReportUnknown  ReportOrigin = "unknown"
)

// ReportUnknownFact is an unresolved report-only evidence gap.
type ReportUnknownFact struct{ Fact, Reason, Owner string }

// ReportRef binds a location or decoded-policy selector to captured raw bytes.
type ReportRef struct {
	Path     string
	SHA256   *string
	Line     *int
	Selector string
	Status   string
}

// ReportStringFact preserves omission separately from the effective value.
type ReportStringFact struct {
	Raw, Effective *string
	Origin         ReportOrigin
	Derivation     string
	Refs           []ReportRef
}

// ReportPathsFact preserves an omitted list separately from an explicit list.
type ReportPathsFact struct {
	Raw, Effective *[]string
	Origin         ReportOrigin
	Derivation     string
	Refs           []ReportRef
}

// ReportComponent describes a declared or observed module without new inference.
type ReportComponent struct {
	Name                  string
	Declared              bool
	Public                ReportPathsFact
	Subdomain, Volatility ReportStringFact
	Unknowns              []ReportUnknownFact
}

// ReportLanguage is the effective requested extractor mode, not its coverage.
type ReportLanguage struct{ Name, Mode string }

// ReportScope names the policy-scoped boundary, not complete input closure.
type ReportScope struct {
	Abstraction, SourceRoot   string
	PolicyRef                 ReportRef
	Exclude, Tests, Generated []string
	Languages                 []ReportLanguage
}

// ReportFileSnapshot retains raw file hashes for reference validation.
type ReportFileSnapshot struct {
	Digest, PolicyDigest *string
	Files                map[string]string
	HeadCommit, HeadTree *string
	Cleanliness          string
	Unknowns             []ReportUnknownFact
}

// ReportContext is run-owned report input, separate from enforcement policy.
type ReportContext struct {
	Scope      ReportScope
	Components []ReportComponent
	Before     ReportFileSnapshot
}

// ReportSite and ReportConnascence preserve retained edge facts.
type ReportSite struct {
	Path string
	Line int
}

// ReportConnascence is one retained static observation.
type ReportConnascence struct{ Kind, Source, Detail string }

// ReportObservation is one retained seam-eligible Set edge, not an interface.
type ReportObservation struct {
	FromNode, ToNode, EdgeKind, Language string
	Sites                                []ReportSite
	Connascence                          []ReportConnascence
}

// ReportPair is an ordered module pair with an unknown interface inventory.
type ReportPair struct {
	PairID       string
	Observations []ReportObservation
	Unknowns     []ReportUnknownFact
}

// ReportRelationships regroups the unchanged classified set for reporting only.
type ReportRelationships struct {
	ModuleInventoryHash   string
	Components            []ReportComponent
	Pairs                 []ReportPair
	PairEdges, TotalEdges int
}

// ReportEvidence carries decided report-only context to the sole public projection.
type ReportEvidence struct {
	Context       *ReportContext
	After         ReportFileSnapshot
	Relationships ReportRelationships
}
