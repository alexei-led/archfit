// Package rulesquery answers `archfit rules` (design 10.2 and 11.1): the rules
// that apply, in plain words.
//
//   - Paths: each path's module, with its public part, volatility, layer and
//     allowed dependencies, the rules and exceptions that apply, and the notes
//     shown as "not checked" (design 6.3).
//   - A rule id: the rule in full, with its exceptions.
//   - No argument: the paths form for the changed files.
//   - Graph: the modules by layer, inner first, with their allowed dependencies.
//
// The package reads the policy model and the changed files through its ports
// and imports no adapter: cmd/archfit wires them. It writes its own text,
// because renderers read only the result contracts and a rules listing is not
// one. File-to-module ownership is resolved.Policy.ModuleOf, and every other
// path glob is matched by resolved.Policy.Covers.
package rulesquery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// PolicySource gives the local policy, resolved with its defaults.
type PolicySource interface {
	Policy(ctx context.Context) (resolved.Policy, error)
}

// ChangedFiles lists the files of the change, repository-relative and
// slash-separated: the set `archfit check` judges.
type ChangedFiles interface {
	Changed(ctx context.Context) ([]string, error)
}

// Request is one `archfit rules` question.
type Request struct {
	// Args are paths, or one rule id. Empty: the changed files.
	Args []string
	// Graph asks for the module graph; it takes no Args.
	Graph bool
	// Date is the day the exception dates are read on. It is an input, so the
	// same policy and date give the same text.
	Date time.Time
}

// ErrUsage marks a request that mixes the forms.
var ErrUsage = errors.New("usage")

// Service answers Requests.
type Service struct {
	Policies PolicySource
	Changed  ChangedFiles
}

// Listing is the answer, as text.
type Listing struct {
	text string
}

// Write writes the listing.
func (l Listing) Write(w io.Writer) error {
	_, err := io.WriteString(w, l.text)
	return err
}

// String returns the listing.
func (l Listing) String() string { return l.text }

// Rules answers req. An error leaves no listing.
func (s Service) Rules(ctx context.Context, req Request) (Listing, error) {
	if req.Graph && len(req.Args) > 0 {
		return Listing{}, fmt.Errorf("%w: --graph takes no path or rule id", ErrUsage)
	}
	p, err := s.Policies.Policy(ctx)
	if err != nil {
		return Listing{}, err
	}
	switch {
	case req.Graph:
		return Listing{text: graph(p)}, nil
	case len(req.Args) == 0:
		files, err := s.Changed.Changed(ctx)
		if err != nil {
			return Listing{}, err
		}
		if len(files) == 0 {
			return Listing{text: "no changed files: name a path or a rule id\n"}, nil
		}
		return Listing{text: paths(p, req.Date, files)}, nil
	}
	ids := slices.DeleteFunc(slices.Clone(req.Args), func(arg string) bool { return !isRule(p, arg) })
	switch {
	case len(ids) == 0:
		return Listing{text: paths(p, req.Date, req.Args)}, nil
	case len(req.Args) == 1:
		return Listing{text: rule(p, req.Date, ids[0])}, nil
	default:
		return Listing{}, fmt.Errorf("%w: %q is a rule id: name one rule id, or paths", ErrUsage, ids[0])
	}
}

// isRule reports whether id is a built-in rule or a rule of the policy.
func isRule(p resolved.Policy, id string) bool {
	_, ok := customRule(p, id)
	return ok || slices.Contains(resolved.BuiltinRules, id)
}

func customRule(p resolved.Policy, id string) (resolved.Rule, bool) {
	i := slices.IndexFunc(p.Rules, func(r resolved.Rule) bool { return r.ID == id })
	if i < 0 {
		return resolved.Rule{}, false
	}
	return p.Rules[i], true
}
