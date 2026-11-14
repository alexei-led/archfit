package policy

// GateMode controls how a missing tool or regressed metric affects the verdict.
type GateMode string

// GateOff, GateWarn, and GateFail define gate behavior.
const (
	GateOff  GateMode = "off"
	GateWarn GateMode = "warn"
	GateFail GateMode = "fail"
)

// MetricEntry holds the settings for a single metric inside the metrics map.
// Gate sets what a baseline regression does to the verdict: off skips the
// check, warn caps at WARN, fail/unset blocks — the rule-gate convention.
// MinDelta (ratio metrics) is the tolerated drop below the baseline; MaxNew
// (count metrics) is the allowed increase. Both default to 0 when unset (any
// worsening move trips). Validation rejects a knob on a metric of the wrong
// kind — pointers so a zero-valued wrong-kind knob (e.g. `max_new: 0` on a
// ratio metric) is still seen as present and rejected, not silently inert.
type MetricEntry struct {
	// Metrics run by default: a knob-only entry (e.g. only `gate: warn`)
	// stays enabled, and only an explicit `enabled: false` disables the
	// metric. (Pointer so absent and false decode differently.)
	Enabled  *bool    `yaml:"enabled"`
	Gate     string   `yaml:"gate"`
	MinDelta *float64 `yaml:"min_delta"`
	MaxNew   *int     `yaml:"max_new"`
}

// MetricConfig is the per-metric policy projection.
type MetricConfig = MetricEntry
