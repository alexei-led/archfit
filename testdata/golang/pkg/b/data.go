package b

// MaxRetries is an exported constant for testing const-reference strength detection.
const MaxRetries = 3

// DefaultName is an exported variable for testing var-reference strength detection.
var DefaultName = "b"

// DefaultHandler is a func-typed var: stored behavior, not pure data.
var DefaultHandler = func() string { return "b" }
