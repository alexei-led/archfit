package report

// EvidenceRef names repository-relative bytes and an optional decoded-policy
// JSON Pointer. It is neither authenticated ownership nor protected Git policy.
type EvidenceRef struct {
	Path     string  `json:"path"`
	SHA256   *string `json:"sha256" jsonschema:"nullable,pattern=^sha256:[0-9a-f]{64}$"`
	Line     *int    `json:"line,omitempty" jsonschema:"minimum=1"`
	Selector string  `json:"selector,omitempty"`
	Status   string  `json:"status" jsonschema:"enum=captured,enum=changed,enum=unknown"`
}

// EvidenceStringFact separates a raw declaration from a resolved policy value.
type EvidenceStringFact struct {
	Raw        *string       `json:"raw" jsonschema:"nullable"`
	Effective  *string       `json:"effective" jsonschema:"nullable"`
	Origin     string        `json:"origin" jsonschema:"enum=observed,enum=asserted,enum=assessed,enum=assumed,enum=unknown"`
	Derivation string        `json:"derivation"`
	Refs       []EvidenceRef `json:"refs"`
}

// EvidencePathsFact preserves an omitted path list as null, not an empty list.
type EvidencePathsFact struct {
	Raw        *[]string     `json:"raw" jsonschema:"nullable"`
	Effective  *[]string     `json:"effective" jsonschema:"nullable"`
	Origin     string        `json:"origin" jsonschema:"enum=observed,enum=asserted,enum=assessed,enum=assumed,enum=unknown"`
	Derivation string        `json:"derivation"`
	Refs       []EvidenceRef `json:"refs"`
}

// EvidenceComponent is a declared or observed module with origin per fact.
type EvidenceComponent struct {
	Name       string             `json:"name"`
	Declared   bool               `json:"declared"`
	Public     EvidencePathsFact  `json:"public"`
	Subdomain  EvidenceStringFact `json:"subdomain"`
	Volatility EvidenceStringFact `json:"volatility"`
	Unknowns   []UnknownFact      `json:"unknowns"`
}

// EvidenceLanguage describes requested mode, not measured tool availability.
type EvidenceLanguage struct {
	Name string `json:"name"`
	Mode string `json:"mode" jsonschema:"enum=true,enum=false,enum=auto"`
}

// EvidenceScope is the effective declared boundary, not proof of input closure.
type EvidenceScope struct {
	Abstraction         string             `json:"abstraction" jsonschema:"enum=module"`
	SourceRoot          string             `json:"source_root"`
	PolicyRef           EvidenceRef        `json:"policy_ref"`
	ModuleInventoryHash string             `json:"module_inventory_hash"`
	ClassificationHash  string             `json:"classification_hash"`
	Exclude             []string           `json:"exclude"`
	Tests               []string           `json:"tests"`
	Generated           []string           `json:"generated"`
	Languages           []EvidenceLanguage `json:"languages"`
}

// EvidenceSnapshot records pre/post raw-byte stability, not immutable execution,
// full build-input closure or transient ABA-change detection.
type EvidenceSnapshot struct {
	HeadCommit         *string       `json:"head_commit" jsonschema:"nullable"`
	HeadTree           *string       `json:"head_tree" jsonschema:"nullable"`
	BeforeDigest       *string       `json:"before_digest" jsonschema:"nullable"`
	AfterDigest        *string       `json:"after_digest" jsonschema:"nullable"`
	PolicyBeforeDigest *string       `json:"policy_before_digest" jsonschema:"nullable"`
	PolicyAfterDigest  *string       `json:"policy_after_digest" jsonschema:"nullable"`
	Cleanliness        string        `json:"cleanliness" jsonschema:"enum=clean,enum=dirty,enum=unknown"`
	Stability          string        `json:"stability" jsonschema:"enum=stable_by_pre_post_check,enum=changed,enum=unknown"`
	PolicyBinding      string        `json:"policy_binding" jsonschema:"enum=match,enum=mismatch,enum=unknown"`
	Unknowns           []UnknownFact `json:"unknowns"`
}

// EvidencePopulation counts seam-eligible retained edges, not module-pair rows.
type EvidencePopulation struct {
	Population string `json:"population" jsonschema:"enum=seam_eligible_set_edges"`
	Unit       string `json:"unit" jsonschema:"enum=edge"`
	TotalEdges int    `json:"total_edges" jsonschema:"minimum=0"`
}

// EvidenceConnascenceItem is a retained producer observation, not strength.
type EvidenceConnascenceItem struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Detail string `json:"detail"`
}

// EvidenceObservation is one Set edge with its exact neutral kind.
type EvidenceObservation struct {
	FromNode    string                    `json:"from_node"`
	ToNode      string                    `json:"to_node"`
	EdgeKind    string                    `json:"edge_kind"`
	Language    string                    `json:"language"`
	Sites       []EvidenceRef             `json:"sites"`
	Connascence []EvidenceConnascenceItem `json:"connascence"`
}

// InterfaceInventory remains unknown because no interface collector exists.
type InterfaceInventory struct {
	Status string `json:"status" jsonschema:"enum=unknown"`
	Reason string `json:"reason"`
}

// EvidencePair joins one existing seam by pair_id; it is not an interface ID.
type EvidencePair struct {
	PairID             string                `json:"pair_id"`
	Observations       []EvidenceObservation `json:"observations"`
	InterfaceInventory InterfaceInventory    `json:"interface_inventory"`
	Unknowns           []UnknownFact         `json:"unknowns"`
}

// EvidenceConnascence counts all retained Set edges. Summary TotalEvidence is
// an item count and can exceed TotalEdges.
type EvidenceConnascence struct {
	Population string             `json:"population" jsonschema:"enum=all_set_edges"`
	Unit       string             `json:"unit" jsonschema:"enum=edge"`
	TotalEdges int                `json:"total_edges" jsonschema:"minimum=0"`
	Summary    *ConnascenceReport `json:"summary" jsonschema:"nullable"`
}

// ReportEvidence is advisory report-only context. It never changes assessment
// confidence, qualification, metric quality, caps or check decisions.
//
//nolint:revive // This explicit report contract distinguishes it from neutral evidence.
type ReportEvidence struct {
	Scope          EvidenceScope       `json:"scope"`
	Snapshot       EvidenceSnapshot    `json:"snapshot"`
	Components     []EvidenceComponent `json:"components"`
	PairPopulation EvidencePopulation  `json:"pair_population"`
	Pairs          []EvidencePair      `json:"pairs"`
	Connascence    EvidenceConnascence `json:"connascence"`
}

const evidenceUnknown = "unknown"

// NewReportEvidence returns an honestly unknown unbound evidence surface.
func NewReportEvidence() ReportEvidence {
	return ReportEvidence{
		Scope:          EvidenceScope{Abstraction: "module", SourceRoot: ".", PolicyRef: EvidenceRef{Status: evidenceUnknown}, Exclude: []string{}, Tests: []string{}, Generated: []string{}, Languages: []EvidenceLanguage{}},
		Snapshot:       EvidenceSnapshot{Cleanliness: evidenceUnknown, Stability: evidenceUnknown, PolicyBinding: evidenceUnknown, Unknowns: []UnknownFact{{Fact: "report_context", Reason: "no consumed-policy snapshot context supplied", Owner: "report caller"}}},
		Components:     []EvidenceComponent{},
		PairPopulation: EvidencePopulation{Population: "seam_eligible_set_edges", Unit: "edge"},
		Pairs:          []EvidencePair{},
		Connascence:    EvidenceConnascence{Population: "all_set_edges", Unit: "edge"},
	}
}
