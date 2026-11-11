package record

import (
	"io"
	"net/url"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/result"
)

const (
	sarifSchema  = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion = "2.1.0"
	toolName     = "archfit"
	// fingerprintKey names the error key in partialFingerprints, so code
	// scanning keeps one alert for one error when its line moves.
	fingerprintKey = "archfitKey/v1"
)

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool         `json:"tool"`
	Invocations []sarifInvocation `json:"invocations"`
	Results     []sarifResult     `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type sarifInvocation struct {
	ExecutionSuccessful bool                `json:"executionSuccessful"`
	Notifications       []sarifNotification `json:"toolExecutionNotifications,omitempty"`
}

type sarifNotification struct {
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"`
	Message             sarifMessage      `json:"message"`
	Locations           []sarifLocation   `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

// ResultSARIF writes a check result as a SARIF 2.1.0 log for code scanning:
// one result per error, in the order given. The message holds the record's
// lines: what, then why, instead, balance and next, in the words of the text
// record. Unknown reasons are tool notifications, not results, and an
// unknown status is an execution that did not succeed. A pass has no result.
func ResultSARIF(w io.Writer, r result.Result) error {
	results := make([]sarifResult, 0, len(r.Errors))
	for _, e := range r.Errors {
		res := sarifResult{
			RuleID:    e.Rule,
			Level:     "error",
			Message:   sarifMessage{Text: messageText(e.What, e.Why, e.Instead, e.Balance, &e.Next)},
			Locations: []sarifLocation{sarifLocationOf(e.Path, e.Line)},
		}
		if e.Key != "" {
			res.PartialFingerprints = map[string]string{fingerprintKey: e.Key}
		}
		results = append(results, res)
	}
	notes := make([]sarifNotification, 0, len(r.Unknown))
	for _, u := range r.Unknown {
		n := sarifNotification{
			Level:   "warning",
			Message: sarifMessage{Text: messageText(string(u.Cause)+": "+u.What, u.Why, "", nil, &u.Next)},
		}
		if u.Path != "" {
			n.Locations = []sarifLocation{sarifLocationOf(u.Path, u.Line)}
		}
		notes = append(notes, n)
	}
	log := sarifLog{
		Schema:  sarifSchema,
		Version: sarifVersion,
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{Name: toolName, Version: r.Engine}},
			Invocations: []sarifInvocation{{
				ExecutionSuccessful: r.Status != result.StatusUnknown,
				Notifications:       notes,
			}},
			Results: results,
		}},
	}
	return writeJSON(w, log)
}

// literalBraces doubles { and }: SARIF reads a single brace as the start of
// a placeholder such as {0} (SARIF 3.11.5), and a record has no arguments.
var literalBraces = strings.NewReplacer("{", "{{", "}", "}}")

// messageText is the first line, then the detail lines of the text record
// without their indent, with literal braces doubled.
func messageText(first, why, instead string, balance *result.Balance, next *result.Next) string {
	var b strings.Builder
	writeDetails(&b, why, instead, balance, next)
	lines := []string{first}
	for line := range strings.Lines(b.String()) {
		lines = append(lines, strings.TrimPrefix(strings.TrimSuffix(line, "\n"), "  "))
	}
	return literalBraces.Replace(strings.Join(lines, "\n"))
}

// sarifLocationOf turns a repository-relative file path into a relative URI
// reference: "#", "%", spaces and non-ASCII bytes are escaped, "/" is kept,
// and a first segment with ":" gets "./" so it is not read as a scheme.
func sarifLocationOf(path string, line int) sarifLocation {
	uri := (&url.URL{Path: path}).String()
	loc := sarifLocation{PhysicalLocation: sarifPhysicalLocation{ArtifactLocation: sarifArtifactLocation{URI: uri}}}
	if line > 0 {
		loc.PhysicalLocation.Region = &sarifRegion{StartLine: line}
	}
	return loc
}
