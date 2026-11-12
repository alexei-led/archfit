// Package reporttext renders `archfit report` as text in the words of design
// section 5: the state of the repository for owners first, with the places
// the check could not read beside it, then its errors, the exceptions close
// to or past their date, the rules nothing checks, the balance of each allowed
// dependency, and the measurements of the report pipeline. It shows no
// verdict: the report measures and decides nothing (D2).
//
// The layout:
//
//	report      = state-line { error } [ expiring ] [ not-checked ] [ allowed ]
//	              [ unread ] [ measurements ] [ unprotected ]
//	state-line  = "repository: " state [ ", " count ] [ " (" n " places not read)" ] NL
//	error       = path [ ":" line ] ": " rule-id ": " what NL fields
//	fields      = [ "  why: " text NL ] [ "  instead: " text NL ]
//	              [ "  balance: " n "/10 (min " m ")" NL ]
//	expiring    = "exceptions close to their date:" NL { "  " rule " " place ": " date NL }
//	not-checked = "not checked:" NL { "  " rule-id ": " note NL }
//	allowed     = "allowed dependencies:" NL { "  " from " -> " to ": " n "/10 (min " m ")" NL }
//	unread      = "not read:" NL { "  " [ path [ ":" line ] ": " ] reason NL [ "    fix: " fix NL ] }
//	measurements = "measurements:" NL { "  " name ": " value [ " (" band ")" ] NL }
//	              (the band only when it is not the value, as "n/a" is)
//	unprotected = "policy: not protected" NL
//
// A document without a repository state, from a version 2 policy, says so on
// the state line and shows the measurements.
package reporttext

import (
	"io"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

// Renderer writes the report text.
type Renderer struct{}

// New returns the report text renderer.
func New() *Renderer { return &Renderer{} }

// Format returns "text".
func (*Renderer) Format() string { return "text" }

// noState is the state line of a document with no repository state.
const noState = "repository: no state: the policy is version 2: run archfit init"

// Render writes d as the report text.
func (*Renderer) Render(d report.Document, w io.Writer) error {
	var b strings.Builder
	repo := d.State.Repository
	if repo == nil {
		b.WriteString(noState + "\n")
	} else {
		writeRepository(&b, *repo)
	}
	writeMeasurements(&b, d.Metrics)
	if repo != nil && !repo.Protected {
		b.WriteString("policy: not protected\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeRepository(b *strings.Builder, r report.RepositoryState) {
	b.WriteString("repository: " + r.State)
	switch r.State {
	case report.RepositoryDrift:
		b.WriteString(", " + count(len(r.Drift), "error") + " no exception covers")
	case report.RepositoryExpired:
		b.WriteString(", " + count(len(r.Expired), "exception") + " past the date")
	}
	if len(r.Unread) > 0 {
		b.WriteString(" (" + count(len(r.Unread), "place") + " not read)")
	}
	b.WriteString("\n")
	for _, e := range r.Expired {
		writeError(b, e)
	}
	for _, e := range r.Drift {
		writeError(b, e)
	}
	if len(r.Expiring) > 0 {
		b.WriteString("exceptions close to their date:\n")
		for _, x := range r.Expiring {
			b.WriteString("  " + x.Rule + " " + place(x.From, x.To) + ": " + x.Expires + "\n")
		}
	}
	if len(r.NotChecked) > 0 {
		b.WriteString("not checked:\n")
		for _, n := range r.NotChecked {
			b.WriteString("  " + n.ID + ": " + n.Note + "\n")
		}
	}
	if len(r.Balances) > 0 {
		b.WriteString("allowed dependencies:\n")
		for _, a := range r.Balances {
			b.WriteString("  " + a.From + " -> " + a.To + ": " + balance(a.Balance) + "\n")
		}
	}
	if len(r.Unread) > 0 {
		b.WriteString("not read:\n")
		for _, u := range r.Unread {
			b.WriteString("  ")
			if u.Path != "" {
				b.WriteString(location(u.Path, u.Line) + ": ")
			}
			b.WriteString(u.Reason + "\n")
			if u.Fix != "" {
				b.WriteString("    fix: " + u.Fix + "\n")
			}
		}
	}
}

func writeError(b *strings.Builder, e report.RepositoryError) {
	b.WriteString(location(e.Path, e.Line) + ": " + e.Rule + ": " + e.What + "\n")
	if e.Why != "" {
		b.WriteString("  why: " + e.Why + "\n")
	}
	if e.Instead != "" {
		b.WriteString("  instead: " + e.Instead + "\n")
	}
	if e.Balance != nil {
		b.WriteString("  balance: " + balance(*e.Balance) + "\n")
	}
}

func writeMeasurements(b *strings.Builder, metrics []report.MetricResult) {
	if len(metrics) == 0 {
		return
	}
	b.WriteString("measurements:\n")
	for _, m := range metrics {
		value := m.Display
		if value == "" {
			value = strconv.FormatFloat(m.Value, 'f', -1, 64)
		}
		b.WriteString("  " + m.Name + ": " + value)
		if m.Band != "" && m.Band != value {
			b.WriteString(" (" + m.Band + ")")
		}
		b.WriteString("\n")
	}
}

func balance(b report.RepositoryBalance) string {
	return strconv.Itoa(b.Value) + "/10 (min " + strconv.Itoa(b.Min) + ")"
}

func place(from, to string) string {
	if to == "" {
		return from
	}
	return from + " -> " + to
}

func location(path string, line int) string {
	if line == 0 {
		return path
	}
	return path + ":" + strconv.Itoa(line)
}

// count is "n noun", with the noun in the plural for any n but 1.
func count(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return strconv.Itoa(n) + " " + noun
}
