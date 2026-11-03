package report

// MetricRatchetStatus describes whether a blocking metric comparison was
// evaluated. Findings remain the authority for acceptance and waiver status.
type MetricRatchetStatus string

// Metric ratchet measurement statuses.
const (
	MetricRatchetEvaluated  MetricRatchetStatus = "evaluated"
	MetricRatchetUnmeasured MetricRatchetStatus = "unmeasured"
)

// MetricRatchetCheck is one applicable implicit accepted-reference check.
// Numerical comparisons are present only when the reference and metric compare.
type MetricRatchetCheck struct {
	Metric    string              `json:"metric"`
	Status    MetricRatchetStatus `json:"status"`
	Threshold string              `json:"threshold,omitempty"`
	Before    *float64            `json:"before,omitempty"`
	After     *float64            `json:"after,omitempty"`
	Delta     *float64            `json:"delta,omitempty"`
	Reason    string              `json:"reason,omitempty"`
}
