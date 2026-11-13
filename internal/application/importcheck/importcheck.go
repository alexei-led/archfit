// Package importcheck runs `archfit can` for an agent after an edit (design
// 10.2, 11.3): it asks about every import on a line the edit added and keeps
// the noes that check would report. The post-tool-use hook shows them.
//
// The imports are the ones the check reads (the head check facts), so the
// hook and check never read a file two ways. An import on a line the change
// did not add is not asked about. A no that an exception of the policy covers
// is dropped: check would pass it. A yes or an unknown is dropped too: the
// check at the end of the session gives the verdict.
//
// `can` sees no cycle the import would close; check sees it.
//
// The package imports no adapter: cmd/archfit wires the ports.
package importcheck

import (
	"context"
	"slices"
	"time"

	"github.com/alexei-led/archfit/v3/internal/application/canquery"
	"github.com/alexei-led/archfit/v3/internal/assessment/count"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// Policies reads the local policy. ok is false when there is none, or when
// the file is not a valid version 3 policy: then there is nothing to ask can
// with, and the check reports why.
type Policies interface {
	Local(ctx context.Context) (p resolved.Policy, ok bool, err error)
}

// Facts reads the check facts of the working tree with the settings of p.
type Facts interface {
	Head(ctx context.Context, p resolved.Policy) (evidence.CheckFacts, error)
}

// Repository reads the lines the working tree adds against a commit.
type Repository interface {
	AddedLines(ctx context.Context, base string) (evidence.AddedLines, error)
}

// Can answers one `archfit can` question.
type Can interface {
	Can(ctx context.Context, q canquery.Question) (result.CanAnswer, error)
}

// Clock gives the date the exceptions are judged on.
type Clock interface {
	Now() time.Time
}

// Service is the use case. Every port is required.
type Service struct {
	Policies   Policies
	Facts      Facts
	Repository Repository
	Can        Can
	Clock      Clock
}

// Request names the edit.
type Request struct {
	// Base is the commit the edit is compared with. Empty: every line of the
	// working tree is an added line.
	Base string
	// Stamp is what each answer records about the run.
	Stamp canquery.Stamp
}

// Refusal is one import that `can` answers no for.
type Refusal struct {
	// File and Line are where the import is, repository-relative.
	File string
	Line int
	// Import is what the file imports: a path of the tree, or a library.
	Import string
	// Answer is the answer of `can`.
	Answer result.CanAnswer
}

// Refusals returns the noes for the imports on added lines, in the order of
// the facts. An error of a port comes back as it is.
func (s Service) Refusals(ctx context.Context, r Request) ([]Refusal, error) {
	p, ok, err := s.Policies.Local(ctx)
	if err != nil || !ok {
		return nil, err
	}
	facts, err := s.Facts.Head(ctx, p)
	if err != nil {
		return nil, err
	}
	var added evidence.AddedLines
	if r.Base != "" {
		if added, err = s.Repository.AddedLines(ctx, r.Base); err != nil {
			return nil, err
		}
	}
	today := s.Clock.Now()
	var out []Refusal
	for _, imp := range facts.Imports {
		if added != nil && !slices.Contains(added[imp.File], imp.Line) {
			continue
		}
		target := imp.Target
		if !imp.External {
			// A path of the tree, also where it spells a module name.
			target = "./" + target
		}
		a, err := s.Can.Can(ctx, canquery.Question{From: imp.File, To: target, Stamp: r.Stamp})
		if err != nil {
			return nil, err
		}
		if a.Answer != result.ReplyNo || count.Covered(p, a.Rule, a.From, a.To, today) {
			continue
		}
		out = append(out, Refusal{File: imp.File, Line: imp.Line, Import: imp.Target, Answer: a})
	}
	return out, nil
}
