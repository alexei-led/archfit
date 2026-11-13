package requestsend

// Input is the content to send. The caller validates the reason and date.
// Can holds the rendered source answer; nil means the source was a check, which
// may only be sent as a pull-request comment.
type Input struct {
	Reason  string
	Expires string
	Can     *CanRecord
}

// CanRecord carries the dependency and its verbatim rendered can answer.
// Rendering belongs to the composition root, not the GitHub transport.
type CanRecord struct {
	From   string
	To     string
	Record string
}

// Outcome identifies the GitHub comment or issue. Its zero value means the
// checkout has no GitHub remote and nothing was sent.
type Outcome struct {
	Number int
	URL    string
}
