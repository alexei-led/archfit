// Package requestsend sends an owner's request as a GitHub comment or issue.
// The App owns the wire grammar; testdata vendors its canonical examples.
package requestsend

import (
	"strings"

	"github.com/alexei-led/archfit/v3/internal/output/record"
)

// Line is the canonical comment line, including its final newline. The
// application validates the reason and date before calling the sender.
func Line(r Input) string {
	line := "/archfit request " + r.Reason
	if r.Expires != "" {
		line += " --expires " + r.Expires
	}
	return line + "\n"
}

// issueBody keeps the can record verbatim inside a fence longer than any
// backtick run in the record, as required by the App's issue-form grammar.
func issueBody(r Input) string {
	var b strings.Builder
	_ = record.Answer(&b, *r.Can, record.Options{})
	body := b.String()
	longest, run := 2, 0
	for _, c := range body {
		if c == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	return Line(r) + "\n" + fence + "text\n" + body + fence + "\n"
}
