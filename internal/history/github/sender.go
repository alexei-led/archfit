package requestsend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const timeout = 30 * time.Second

// Sender posts a request to GitHub. Client may be supplied for a custom HTTP
// transport. Every call has a timeout and refuses redirects; credentials must
// never be forwarded to another host.
type Sender struct {
	dir    string
	runner toolrun.Runner
	Client *http.Client
}

// New wires the GitHub adapter. A non-GitHub checkout returns an empty delivery
// without looking up credentials, preserving the printed offline request.
func New(dir string, runner toolrun.Runner) *Sender {
	return &Sender{dir: dir, runner: runner}
}

// Send finds exactly one open pull request and posts one comment. Without a
// pull request, only a can answer may open an issue. POST is never retried.
func (s *Sender) Send(ctx context.Context, r Input) (Outcome, error) {
	repo, head, err := s.repository(ctx)
	if err != nil {
		return Outcome{}, err
	}
	if repo == "" {
		return Outcome{}, nil
	}
	if head == "" && r.Can == nil {
		return Outcome{}, openPull()
	}
	token, err := s.credentials(ctx)
	if err != nil {
		return Outcome{}, err
	}
	path := "/repos/" + repo
	var pulls []struct {
		Number int `json:"number"`
	}
	if head != "" {
		q := url.Values{"state": {"open"}, "head": {head}, "per_page": {"100"}}
		more, err := s.call(ctx, token, http.MethodGet, path+"/pulls?"+q.Encode(), nil, &pulls)
		if err != nil {
			return Outcome{}, err
		}
		if pulls == nil {
			return Outcome{}, errors.New("GitHub lookup returned no list; nothing sent")
		}
		if len(pulls) > 1 || more {
			numbers := make([]string, 0, len(pulls))
			for _, p := range pulls {
				numbers = append(numbers, "#"+strconv.Itoa(p.Number))
			}
			return Outcome{}, fmt.Errorf("several open pull requests match: %s; nothing sent", strings.Join(numbers, ", "))
		}
	}
	var payload any
	if len(pulls) == 1 {
		if pulls[0].Number < 1 {
			return Outcome{}, errors.New("GitHub returned an invalid pull request; nothing sent")
		}
		path += "/issues/" + strconv.Itoa(pulls[0].Number) + "/comments"
		payload = struct {
			Body string `json:"body"`
		}{Line(r)}
	} else {
		if r.Can == nil {
			return Outcome{}, openPull()
		}
		path += "/issues"
		payload = struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}{"archfit request: " + r.Can.From + " -> " + r.Can.To, issueBody(r)}
	}
	var sent struct {
		Number int    `json:"number"`
		ID     int    `json:"id"`
		URL    string `json:"html_url"`
	}
	if _, err := s.call(ctx, token, http.MethodPost, path, payload, &sent); err != nil {
		return Outcome{}, err
	}
	// A comment has an id, not an issue number. Preserve the containing PR number.
	number := sent.Number
	if len(pulls) == 1 {
		number = pulls[0].Number
	}
	u, err := url.Parse(sent.URL)
	repoPath := "/" + repo + "/"
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" ||
		len(u.Path) < len(repoPath) || !strings.EqualFold(u.Path[:len(repoPath)], repoPath) || strings.Contains(sent.URL, token) ||
		strings.Contains(u.Path+"#"+u.Fragment, token) || number < 1 {
		return Outcome{}, errors.New("GitHub returned an invalid receipt; outcome unknown: check GitHub before trying again")
	}
	return Outcome{Number: number, URL: sent.URL}, nil
}

func openPull() error {
	return errors.New("no open pull request for this branch; nothing sent\nnext: open a pull request first")
}

// credentials never returns the output or error of gh: either can contain a
// credential. Tokens are passed only in the HTTP Authorization header.
func (s *Sender) credentials(ctx context.Context) (string, error) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if token := strings.TrimSpace(os.Getenv(name)); token != "" {
			return token, nil
		}
	}
	out, err := s.runner.Run(ctx, toolrun.ToolCmd{Name: "gh", Args: []string{"auth", "token"}, WorkDir: s.dir, Timeout: timeout})
	if err == nil && out.ExitCode == 0 {
		if token := strings.TrimSpace(string(out.Stdout)); token != "" {
			return token, nil
		}
	}
	return "", errors.New("no GitHub credentials; nothing sent\nnext: gh auth login")
}

// call deliberately omits response bodies, URLs and transport errors from
// diagnostics: an upstream or proxy error may echo the Authorization header.
func (s *Sender) call(ctx context.Context, token, method, path string, payload, result any) (bool, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return false, errors.New("cannot encode the request; nothing sent")
		}
		// A reader rather than a bytes.Reader leaves GetBody nil: no automatic
		// replay of a POST by net/http.
		body = io.LimitReader(bytes.NewReader(data), int64(len(data)))
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com"+path, body)
	if err != nil {
		return false, errors.New("cannot build GitHub request; nothing sent")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := http.Client{}
	if s.Client != nil {
		client = *s.Client
	}
	if client.Timeout <= 0 || client.Timeout > timeout {
		client.Timeout = timeout
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		if method == http.MethodPost {
			return false, errors.New("GitHub send failed; outcome unknown: check GitHub before trying again (not retried)")
		}
		return false, errors.New("GitHub lookup failed; nothing sent")
	}
	defer func() { _ = response.Body.Close() }()
	want := http.StatusOK
	if method == http.MethodPost {
		want = http.StatusCreated
	}
	if response.StatusCode != want {
		suffix := "; nothing sent"
		if method == http.MethodPost && (response.StatusCode < 400 || response.StatusCode >= 500) {
			suffix = "; outcome unknown: check GitHub before trying again (not retried)"
		}
		return false, fmt.Errorf("GitHub returned HTTP %d%s", response.StatusCode, suffix)
	}
	dec := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	err = dec.Decode(result)
	if err == nil {
		var extra any
		if dec.Decode(&extra) != io.EOF {
			err = errors.New("trailing JSON")
		}
	}
	if err != nil {
		if method == http.MethodPost {
			return false, errors.New("GitHub receipt unreadable; outcome unknown: check GitHub before trying again")
		}
		return false, errors.New("GitHub lookup unreadable; nothing sent")
	}
	return strings.Contains(response.Header.Get("Link"), `rel="next"`), nil
}

// git runs only read-only commands and never surfaces remote URLs or stderr.
func (s *Sender) git(ctx context.Context, args ...string) (string, int, error) {
	out, err := s.runner.Run(ctx, toolrun.ToolCmd{Name: "git", Args: args, WorkDir: s.dir, Timeout: timeout})
	if err != nil {
		return "", 0, errors.New("cannot read GitHub repository from git; nothing sent")
	}
	return strings.TrimSpace(string(out.Stdout)), out.ExitCode, nil
}

func (s *Sender) repository(ctx context.Context) (string, string, error) {
	origin, code, err := s.git(ctx, "config", "--get", "remote.origin.url")
	if err != nil {
		return "", "", err
	}
	if code != 0 && code != 1 {
		return "", "", errors.New("cannot read origin; nothing sent")
	}
	repo := githubRepository(origin)
	if repo == "" {
		return "", "", nil
	}
	target, _, err := s.git(ctx, "config", "--get", "remote.upstream.url")
	if err != nil {
		return "", "", err
	}
	if upstream := githubRepository(target); upstream != "" {
		repo = upstream
	}
	branch, code, err := s.git(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", "", err
	}
	if code == 1 {
		return repo, "", nil
	} // Detached HEAD.
	if code != 0 {
		return "", "", errors.New("cannot read current branch; nothing sent")
	}
	upstream, code, err := s.git(ctx, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return "", "", err
	}
	if code != 0 {
		return repo, "", nil
	}
	pushRemote, _, err := s.git(ctx, "for-each-ref", "--format=%(push:remotename)", "refs/heads/"+branch)
	if err != nil {
		return "", "", err
	}
	if pushRemote == "" {
		pushRemote = "origin"
	}
	remote, code, err := s.git(ctx, "remote", "get-url", "--push", pushRemote)
	if err != nil {
		return "", "", err
	}
	pushRepo := githubRepository(remote)
	if code != 0 || pushRepo == "" {
		return "", "", errors.New("cannot identify the GitHub push destination; nothing sent")
	}
	_, remoteBranch, ok := strings.Cut(upstream, "/")
	if !ok || remoteBranch == "" {
		return "", "", errors.New("cannot identify the upstream branch; nothing sent")
	}
	owner, _, _ := strings.Cut(pushRepo, "/")
	return repo, owner + ":" + remoteBranch, nil
}

// githubRepository accepts GitHub HTTPS, SSH and scp-style clone URLs. Reject
// local paths, arbitrary hosts and malformed names before any credential lookup.
func githubRepository(remote string) string {
	if strings.HasPrefix(remote, "git@github.com:") {
		remote = "ssh://git@github.com/" + strings.TrimPrefix(remote, "git@github.com:")
	}
	u, err := url.Parse(remote)
	if err != nil || (u.Scheme != "https" && u.Scheme != "ssh") || u.Host != "github.com" || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	if u.User != nil && (u.Scheme != "ssh" || u.User.String() != "git") {
		return ""
	}
	path := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return ""
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return ""
		}
		for _, c := range part {
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
			default:
				return ""
			}
		}
	}
	return path
}
