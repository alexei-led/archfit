package pipeline

// Mode controls technical pipeline behavior. It is a sequencing concern, not
// a domain input owned by the legacy engine package.
type Mode struct {
	Base       string
	Head       string
	Full       bool
	Advisory   bool
	ReportOnly bool
	Formats    []string
}
