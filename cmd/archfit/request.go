package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/application/request"
	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	requestsend "github.com/alexei-led/archfit/v3/internal/history/github"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// RequestCmd is `archfit request` (internal/application/request): ask an
// owner to change the policy for the errors of the last check, or for the
// dependency of the last `archfit can` answer. It posts to GitHub when a
// GitHub remote is present, otherwise prints the request (D5).
type RequestCmd struct {
	Message string `short:"m" help:"Why you need the change. On a terminal archfit asks for it when -m is not given." placeholder:"WHY"`
	Expires string `help:"Ask for an exception until DATE (YYYY-MM-DD), at most 365 days ahead. Without it the request asks for a permanent change." placeholder:"DATE"`
	Dir     string `short:"C" name:"directory" help:"Run in DIR instead of the current directory." placeholder:"DIR"`
}

func (*RequestCmd) Help() string {
	return `Ask an owner to change the policy. The request takes the errors of the last
archfit check (or the dependency of the last archfit can answer) on this tree,
so you write only the reason.

Only an error whose next step is archfit request joins the request. Run
archfit check again after an edit: a request never uses an older tree.

With one open pull request, post the request as a comment. Before a pull
request exists, only a can answer may open an issue. Uses GH_TOKEN, then
GITHUB_TOKEN, then gh auth token. Without a GitHub remote the request is
printed, not sent: show it to an owner.

Exit codes:
  0  sent: prints the comment or issue URL
  2  not sent, unknown send outcome, wrong usage, or a tool error

Examples:
  archfit request -m "netem needs the shared retry helper"
  archfit request -m "tc parses the name before pkg_util loads" --expires 2026-12-10`
}

// askOwner is the last line of a request no App sent (D5).
const askOwner = "next: ask an owner"

// noPullRequest is the note on a request built from check output: the App
// joins such a request to a pull request. Milestone 1 cannot see one.
const noPullRequest = "open a pull request first if there is none"

func (c *RequestCmd) Run(deps *appDeps) error {
	ctx := deps.runContext()
	co, err := openCheckout(ctx, deps, dirOr(c.Dir))
	if err != nil {
		return noVerdict(err)
	}
	sender := requestsend.New(co.root, co.runner)
	sender.Client = deps.requestClient
	svc := request.Service{
		Sender: requestWiring{sender: sender},
		Last:   lastReader{co}, Checkout: checkoutReader{co}, Clock: clockFunc(co.now),
		Reason: terminalReason(deps),
	}
	out, err := svc.Ask(ctx, request.Input{Reason: c.Message, Expires: c.Expires})
	if err != nil {
		return noVerdict(err)
	}
	if out.URL != "" {
		_, err := fmt.Fprintln(deps.Stdout, out.URL)
		if err != nil {
			return noVerdict(err)
		}
		return nil
	}
	protected, err := protection{co}.Protected(ctx)
	if err != nil {
		return noVerdict(err)
	}
	if err := writeRequest(deps.Stdout, out.Request, protected, co.now()); err != nil {
		return noVerdict(err)
	}
	return exitWith(exitUnknown)
}

// writeRequest prints a request no App sent: the errors it asks about, the
// reason and the date asked for, then the next step last.
func writeRequest(w io.Writer, r request.Request, protected bool, today time.Time) error {
	var b strings.Builder
	for _, e := range r.Errors {
		where := e.From + " -> " + e.To
		if e.Path != "" {
			where = location(e.Path, e.Line) + ": " + e.Rule + ": " + where
		} else {
			where += ": " + e.Rule
		}
		b.WriteString(where + "\n")
		if e.Balance != nil {
			b.WriteString("  balance: " + strconv.Itoa(e.Balance.Value) + "/10 (min " + strconv.Itoa(e.Balance.Min) + ")\n")
		}
	}
	b.WriteString("request: " + r.Reason + "\n")
	if r.Expires != "" {
		day, err := time.Parse(time.DateOnly, r.Expires)
		if err != nil {
			return err
		}
		b.WriteString("  until: " + resolved.FormatExpires(day, today) + "\n")
	}
	if !protected {
		b.WriteString("policy: not protected\n")
	}
	b.WriteString(askOwner)
	// Only check output has a base; a can answer comes before a pull request.
	if r.Base != "" {
		b.WriteString("     (" + noPullRequest + ")")
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// location is "path:line", or the path alone when the line is 0.
func location(path string, line int) string {
	if line == 0 {
		return path
	}
	return path + ":" + strconv.Itoa(line)
}

// lastReader is request.LastStore over .archfit-cache/last.json.
type lastReader struct{ co checkout }

func (l lastReader) Last(context.Context) (request.Last, bool, error) {
	data, err := os.ReadFile(filepath.Join(l.co.root, lastFile))
	if errors.Is(err, fs.ErrNotExist) {
		return request.Last{}, false, nil
	}
	if err != nil {
		return request.Last{}, false, err
	}
	var doc struct {
		Tree   string          `json:"tree"`
		Output json.RawMessage `json:"output"`
	}
	var head struct {
		Schema string `json:"schema"`
	}
	// A file another version wrote, or one cut short, is not usable: the
	// worker runs check again.
	if json.Unmarshal(data, &doc) != nil || json.Unmarshal(doc.Output, &head) != nil {
		return request.Last{}, false, nil
	}
	last := request.Last{Tree: doc.Tree}
	switch head.Schema {
	case result.SchemaVersion:
		last.Check = &result.Result{}
		err = json.Unmarshal(doc.Output, last.Check)
	case result.CanSchemaVersion:
		last.Can = &result.CanAnswer{}
		err = json.Unmarshal(doc.Output, last.Can)
	default:
		return request.Last{}, false, nil
	}
	if err != nil {
		return request.Last{}, false, nil
	}
	return last, true, nil
}

// checkoutReader is request.CheckoutReader: HEAD and the working tree id.
type checkoutReader struct{ co checkout }

func (c checkoutReader) Checkout(ctx context.Context) (request.Checkout, error) {
	head, _, err := gitpkg.Lookup(ctx, c.co.root, "HEAD", c.co.runner)
	if err != nil {
		return request.Checkout{}, err
	}
	tree, err := c.co.workingTree(ctx)
	if err != nil {
		return request.Checkout{}, err
	}
	return request.Checkout{Head: head, Tree: tree}, nil
}

// terminalReason is the reason reader of a terminal, or nil off one: then a
// missing -m is a usage error. Both stdin and stderr, where the question
// goes, must be terminals: /dev/null is a character device too.
func terminalReason(deps *appDeps) request.ReasonReader {
	if deps.Stdin != nil || !isTerminal(os.Stdin) || !isTerminal(deps.stderr()) {
		return nil
	}
	return lineReason{in: os.Stdin, prompt: deps.stderr()}
}

// lineReason asks for the reason on prompt and reads one line from in.
type lineReason struct {
	in     io.Reader
	prompt io.Writer
}

// Reason returns when a line is read or ctx ends. A signal cancels ctx, and a
// read of a terminal does not stop for it, so the read runs apart: the process
// exits with the read still waiting.
func (r lineReason) Reason(ctx context.Context) (string, error) {
	if _, err := fmt.Fprint(r.prompt, "reason: "); err != nil {
		return "", err
	}
	type read struct {
		line string
		err  error
	}
	done := make(chan read, 1)
	go func() {
		line, err := bufio.NewReader(r.in).ReadString('\n')
		done <- read{line, err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case got := <-done:
		if got.err != nil && !errors.Is(got.err, io.EOF) {
			return "", got.err
		}
		return got.line, nil
	}
}
