package report

// MeasurementProfile publishes the producers and settings behind a measurement.
type MeasurementProfile struct {
	Version      string                `json:"version"`
	SettingsHash string                `json:"settings_hash"`
	Producers    []MeasurementProducer `json:"producers"`
	Unknowns     []string              `json:"unknowns"`
}

// MeasurementProducer publishes an extractor contract and its availability.
type MeasurementProducer struct {
	Tool             string `json:"tool"`
	SemanticsVersion string `json:"semantics_version"`
	ToolVersion      string `json:"tool_version,omitempty"`
	Status           string `json:"status"`
	PartialBasis     string `json:"partial_basis,omitempty"`
}
