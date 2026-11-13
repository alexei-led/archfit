package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/application/importcheck"
	historygit "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/output/record"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// sessionDirName holds the session files under the cache directory.
const sessionDirName = "session"

// hookSession is what the hooks remember about one agent session, in
// <repo>/.archfit-cache/session/<session_id>.json.
type hookSession struct {
	// BaseSHA is HEAD when the session started; empty when HEAD named no
	// commit yet, which makes every line of the tree an added line.
	BaseSHA   string    `json:"base_sha"`
	StartedAt time.Time `json:"started_at"`
	// LastTree is the working tree id post-tool-use judged last: an unchanged
	// tree is not judged again.
	LastTree string `json:"last_tree,omitempty"`
	// Reported maps a source file to the imports post-tool-use reported on it
	// and that it still adds, as "target NUL rule". An import that leaves the
	// file is dropped, so it is reported again when it comes back.
	Reported map[string][]string `json:"reported,omitempty"`
	// StoppedOn identifies the set of errors the stop hook stopped on last.
	StoppedOn string `json:"stopped_on,omitempty"`

	repo   string
	path   string
	exists bool
}

// openSession reads the session of id in the repository that holds dir. A
// session with no file yet starts now: its base is the current HEAD.
func openSession(ctx context.Context, runner toolrun.Runner, dir, id string) (*hookSession, error) {
	repo, err := historygit.RepoRoot(ctx, dir, runner)
	if err != nil {
		return nil, fmt.Errorf("the session hooks need a git repository: %w", err)
	}
	sess := &hookSession{repo: repo, path: filepath.Join(repo, cacheDirName, sessionDirName, sessionFileName(id))}
	raw, err := os.ReadFile(sess.path)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, sess); err != nil {
			return nil, fmt.Errorf("read the session %s: %w", sess.path, err)
		}
		sess.exists = true
		return sess, nil
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	head, ok, err := historygit.Lookup(ctx, repo, headRef, runner)
	if err != nil {
		return nil, err
	}
	if ok {
		sess.BaseSHA = head
	}
	sess.StartedAt = time.Now().UTC()
	return sess, nil
}

// sessionFileName keeps a session id to [A-Za-z0-9_-]: the id comes from the
// harness and becomes a file name.
func sessionFileName(id string) string {
	clean := strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, id)
	if clean == "" {
		clean = "default"
	}
	return clean + ".json"
}

// save writes the session through a temporary file, so a reader never sees
// half of it.
func (s *hookSession) save() error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, append(raw, '\n'), 0o600)
}

// untouched reports whether nothing changed since the session base: HEAD is
// still the base and the working tree is clean, archfit's cache aside.
func (s *hookSession) untouched(ctx context.Context, runner toolrun.Runner) (bool, error) {
	head, ok, err := historygit.Lookup(ctx, s.repo, headRef, runner)
	if err != nil {
		return false, err
	}
	if !ok {
		head = ""
	}
	if head != s.BaseSHA {
		return false, nil
	}
	dirty, err := worktreeDirty(ctx, runner, s.repo)
	return !dirty, err
}

// sessionStart records the session base and prints the five lines of design
// 11.3, the lines archfit init writes for agents. A session
// that already has a file keeps its base: Claude Code fires SessionStart again
// on resume and after compaction, and the change still began at the old base.
func sessionStart(ctx context.Context, deps *appDeps, dir, id string) error {
	sess, err := openSession(ctx, deps.Runner, dir, id)
	if err == nil && !sess.exists {
		err = sess.save()
	}
	if _, werr := fmt.Fprint(deps.Stdout, agentLines); werr != nil {
		return werr
	}
	if err != nil {
		// The lines still help; the later hooks start the session themselves.
		_, _ = fmt.Fprintf(deps.stderr(), "archfit: the session was not recorded: %v\n", err)
	}
	return nil
}

// postToolUse reports the imports added since the session base that archfit
// can answers no for, once while they stay. It reads the tree, never the tool
// payload, so an edit made through a shell command counts too. The imports
// are the ones the check reads (the head check facts), on lines added since
// the session base (internal/application/importcheck), with the errors an
// exception covers left out. A yes, an unknown, or a tree without a valid
// version 3 policy is silent: the stop hook reports what the check makes of
// it. The answer of the first no is stored for archfit request, as archfit
// can stores its answer. A failure of the hook's own is a message, never a
// stop.
func postToolUse(ctx context.Context, deps *appDeps, dir, id string) error {
	sess, err := openSession(ctx, deps.Runner, dir, id)
	if err != nil {
		return systemMessage(deps, couldNotRun(err))
	}
	co, err := openCheckout(ctx, deps, sess.repo)
	if err != nil {
		return systemMessage(deps, couldNotRun(err))
	}
	tree, err := co.workingTree(ctx)
	if err != nil {
		return systemMessage(deps, couldNotRun(err))
	}
	if tree == sess.LastTree {
		return nil
	}
	shown, err := sess.newRefusals(ctx, co)
	if err != nil {
		return systemMessage(deps, couldNotRun(err))
	}
	sess.LastTree = tree
	var notes []string
	if err := sess.save(); err != nil {
		notes = append(notes, "archfit: the session was not recorded: "+err.Error())
	}
	if len(shown) == 0 {
		// A failure of the hook's own never stops the agent.
		if len(notes) > 0 {
			return systemMessage(deps, strings.Join(notes, "\n"))
		}
		return nil
	}
	// archfit request reads the answer of the first no, as after archfit can.
	if err := co.saveAnswer(tree, shown[0].Answer); err != nil {
		notes = append(notes, "archfit: the answer was not stored for archfit request: "+err.Error())
	}
	var b strings.Builder
	for _, r := range shown {
		if err := refusalText(&b, r); err != nil {
			return systemMessage(deps, couldNotRun(err))
		}
	}
	for _, note := range notes {
		b.WriteString(note + "\n")
	}
	return &exitError{code: 2, msg: strings.TrimSuffix(b.String(), "\n")}
}

// newRefusals asks archfit can about each import on a line added since the
// session base (internal/application/importcheck) and returns every no the
// session has not shown yet. It keeps in Reported the noes each file still
// adds.
func (s *hookSession) newRefusals(ctx context.Context, co checkout) ([]importcheck.Refusal, error) {
	base := s.BaseSHA
	if base != "" && !refExists(ctx, co.runner, co.root, base) {
		// A rebase dropped the base commit: HEAD is the nearest one left.
		base = headRef
	}
	stamp, err := co.stamp(ctx)
	if err != nil {
		return nil, err
	}
	refusals, err := co.importCheck().Refusals(ctx, importcheck.Request{Base: base, Stamp: stamp})
	if err != nil {
		return nil, err
	}
	held := map[string][]string{}
	var shown []importcheck.Refusal
	for _, r := range refusals {
		key := r.Import + "\x00" + r.Answer.Rule
		if slices.Contains(held[r.File], key) {
			continue
		}
		held[r.File] = append(held[r.File], key)
		if !slices.Contains(s.Reported[r.File], key) {
			shown = append(shown, r)
		}
	}
	for file := range held {
		slices.Sort(held[file])
	}
	s.Reported = held
	return shown, nil
}

// refusalText writes what post-tool-use shows for one import that archfit
// can answers no for: the file, the line and the import, then the record of
// the answer.
func refusalText(b *strings.Builder, r importcheck.Refusal) error {
	fmt.Fprintf(b, "%s:%d: import %s\n", r.File, r.Line, r.Import)
	return record.Answer(b, r.Answer, record.Options{})
}

// writeFileAtomic writes data to path through a temporary file in the same
// directory, creating the directory when it is missing.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		return err
	}
	return os.Rename(name, path)
}
