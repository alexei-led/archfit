package state

// MetricRatchetStatus describes comparison evidence, not whether a finding was
// waived or accepted. Reporting an evaluation never changes the gate decision.
type MetricRatchetStatus string

// Metric ratchet measurement statuses.
const (
	MetricRatchetEvaluated  MetricRatchetStatus = "evaluated"
	MetricRatchetUnmeasured MetricRatchetStatus = "unmeasured"
)

// MetricRatchetCheck inventories an applicable blocking baseline comparison.
// Unmeasured comparisons carry a reason and no numerical comparison.
type MetricRatchetCheck struct {
	Metric    string
	Status    MetricRatchetStatus
	Threshold string
	Before    *float64
	After     *float64
	Delta     *float64
	Reason    string
}
