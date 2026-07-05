package a

import "example.com/test/pkg/b"

// RetryBudget references only b.MaxRetries — a pure-data const read, no calls,
// no type references. Expected strength hint: model (book Ch7 data sharing).
func RetryBudget() int { return b.MaxRetries }
