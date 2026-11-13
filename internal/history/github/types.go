package requestsend

import "github.com/alexei-led/archfit/v3/internal/model/result"

// Input is the content to send. The caller validates the reason and date.
// Can is the complete source answer; nil means the source was a check, which
// may only be sent as a pull-request comment.
type Input struct {
	Reason  string
	Expires string
	Can     *result.CanAnswer
}

// Outcome identifies the GitHub comment or issue. Its zero value means the
// checkout has no GitHub remote and nothing was sent.
type Outcome struct {
	Number int
	URL    string
}
