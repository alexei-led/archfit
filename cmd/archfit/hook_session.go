package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/application"
	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	historygit "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// sessionDirName holds the session files under the cache directory.
const sessionDirName = "session"

// sessionBootstrap is what session-start prints. Claude Code adds a
// SessionStart hook's stdout to the agent's context.
const sessionBootstrap = `archfit: before you add a cross-module import, run: archfit policy can-import <file> <target>
archfit: before you finish, run: archfit check --format agent and follow next_action
archfit: never edit .archfit.yaml, .archfit-baseline.json or .archfit-labels.yaml to make a check pass; ask the owner
`

// Prefixes the repair task puts on a rule's declared rationale and
// alternatives (internal/assessment/agenttask); check's text output splits
// the constraints into why and instead the same way.
const (
	constraintRationale   = "rationale: "
	constraintAlternative = "allowed alternative: "
)

// hookSession is what the hooks remember about one agent session, in
// <repo>/.archfit-cache/session/<session_id>.json.
type hookSession struct {
	// BaseSHA is HEAD when the session started; empty when HEAD named no
	// commit yet, which makes the whole tree the change.
	BaseSHA   string    `json:"base_sha"`
	StartedAt time.Time `json:"started_at"`
	// Files maps each source file post-tool-use read to the sha256 of its
	// content then: an unchanged file is not read again.
	Files map[string]string `json:"files,omitempty"`
	// Reported maps a source file to the finding IDs of the denials
	// post-tool-use reported on it and that it still holds. A denial that
	// leaves the file is dropped, so the import coming back is reported again.
	Reported map[string][]string `json:"reported,omitempty"`
	// BlockedDigest identifies the last result the stop hook blocked on.
	BlockedDigest string `json:"blocked_digest,omitempty"`

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

// sessionStart records the session base and prints the bootstrap. A session
// that already has a file keeps its base: Claude Code fires SessionStart again
// on resume and after compaction, and the change still began at the old base.
func sessionStart(ctx context.Context, deps *appDeps, dir, id string) error {
	sess, err := openSession(ctx, deps.Runner, dir, id)
	if err == nil && !sess.exists {
		err = sess.save()
	}
	if _, werr := fmt.Fprint(deps.Stdout, sessionBootstrap); werr != nil {
		return werr
	}
	if err != nil {
		// The bootstrap still helps; the later hooks start the session themselves.
		_, _ = fmt.Fprintf(deps.stderr(), "archfit hook: the session was not recorded: %v\n", err)
	}
	return nil
}

// postToolUse reports the imports added since the session base that the
// policy denies, once while they stay. It reads the files from git, never from
// the tool payload, so an edit made through a shell command counts too.
func postToolUse(ctx context.Context, deps *appDeps, dir, config, id string) error {
	sess, err := openSession(ctx, deps.Runner, dir, id)
	if err != nil {
		return systemMessage(deps, "archfit hook failed open: "+err.Error())
	}
	edits, err := sess.importEdits(ctx, deps.Runner)
	if err != nil {
		return systemMessage(deps, "archfit hook failed open: "+err.Error())
	}
	denials, err := sess.newDenials(ctx, deps, configIn(sess.repo, config), edits)
	if err != nil {
		return systemMessage(deps, "archfit hook failed open: "+err.Error())
	}
	if err := sess.save(); err != nil {
		return systemMessage(deps, "archfit hook failed open: "+err.Error())
	}
	if len(denials) == 0 {
		return nil
	}
	denials = append(denials, "archfit: remove the denied import; if it is intended, ask the architecture owner, never edit the policy to allow it")
	return &exitError{code: 2, msg: strings.Join(denials, "\n")}
}

// importEdit is one source file read again and the import targets its added
// lines name; none means its earlier imports are gone.
type importEdit struct {
	file    string
	targets []string
}

// importEdits lists the source files whose content changed since the last
// post-tool-use run, among those that differ from the session base, with the
// imports their added lines introduce. It records the new content hashes,
// except for a file with a relative import of a file that does not exist yet:
// that file is read again until the import resolves. It forgets the reported
// denials of files that no longer differ from the base.
func (s *hookSession) importEdits(ctx context.Context, runner toolrun.Runner) ([]importEdit, error) {
	touched, err := touchedFiles(ctx, runner, s.repo, s.BaseSHA)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(touched))
	for file := range touched {
		files = append(files, file)
	}
	slices.Sort(files)
	isFile := func(rel string) bool {
		info, err := os.Stat(filepath.Join(s.repo, filepath.FromSlash(rel))) //nolint:gosec // resolveRelative keeps rel inside the repository
		return err == nil && info.Mode().IsRegular()
	}
	hashes := map[string]string{}
	var edits []importEdit
	for _, file := range files {
		lang := registry.LanguageForFile(file)
		if !readsImports(lang) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(s.repo, filepath.FromSlash(file)))
		if err != nil {
			continue // deleted since the base
		}
		sum := sha256.Sum256(content)
		hash := hex.EncodeToString(sum[:])
		if s.Files[file] == hash {
			hashes[file] = hash
			continue
		}
		var added map[int]bool
		if !touched[file] {
			if added, err = addedLines(ctx, runner, s.repo, s.BaseSHA, file); err != nil {
				return nil, err
			}
		}
		targets, unresolved := importTargets(lang, file, content, added, isFile)
		if !unresolved {
			hashes[file] = hash
		}
		edits = append(edits, importEdit{file: file, targets: targets})
	}
	s.Files = hashes
	for file := range s.Reported {
		if _, ok := touched[file]; !ok {
			delete(s.Reported, file)
		}
	}
	return edits, nil
}

// readsImports reports whether post-tool-use reads the imports of a language.
// Rust is left out: can-import cannot decide a Rust import in this release.
func readsImports(lang string) bool {
	return lang == config.LangGo || lang == config.LangTypeScript || lang == config.LangPython
}

// touchedFiles lists the repository-relative files that differ from base:
// tracked changes and untracked files. A file maps to true when every line of
// it is new: it is untracked, or there is no base. archfit's own cache is left
// out, so a hook never reacts to what a hook wrote.
func touchedFiles(ctx context.Context, runner toolrun.Runner, repo, base string) (map[string]bool, error) {
	out := map[string]bool{}
	untracked, err := gitPaths(ctx, runner, repo, "ls-files", "-z", "--others", "--exclude-standard", "--full-name", "--", ":/")
	if err != nil {
		return nil, err
	}
	for _, p := range untracked {
		out[p] = true
	}
	args := []string{"diff", "--name-only", "-z", "--no-renames", base, "--"}
	if base == "" {
		args = []string{"ls-files", "-z", "--cached", "--full-name", "--", ":/"}
	}
	tracked, err := gitPaths(ctx, runner, repo, args...)
	if err != nil {
		return nil, err
	}
	for _, p := range tracked {
		if _, seen := out[p]; !seen {
			out[p] = base == ""
		}
	}
	for p := range out {
		if inArchfitCache(p) {
			delete(out, p)
		}
	}
	return out, nil
}

// gitPaths runs git in repo and splits its NUL-separated path output.
func gitPaths(ctx context.Context, runner toolrun.Runner, repo string, args ...string) ([]string, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{Name: gitBinary, Args: args, WorkDir: repo})
	if err != nil {
		return nil, err
	}
	if out.ExitCode != 0 {
		return nil, fmt.Errorf("git %s exited %d: %s", args[0], out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	var paths []string
	for _, p := range strings.Split(string(out.Stdout), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// addedLines returns the line numbers, in the working-tree file, of the lines
// git diff -U0 adds to file since base.
func addedLines(ctx context.Context, runner toolrun.Runner, repo, base, file string) (map[int]bool, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{Name: gitBinary, WorkDir: repo, Args: []string{
		"diff", "-U0", "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames", base, "--", ":(literal)" + file,
	}})
	if err != nil {
		return nil, err
	}
	if out.ExitCode != 0 {
		return nil, fmt.Errorf("git diff exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	return parseAddedLines(string(out.Stdout)), nil
}

// hunkHeader is a unified diff hunk header; the second range is the new file's.
var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// parseAddedLines reads the added line numbers from a git diff -U0: with no
// context lines, every new-file line a hunk covers is an added line. A body
// line starts with +, - or a space, so it is never read as a header.
func parseAddedLines(diff string) map[int]bool {
	added := map[int]bool{}
	for _, line := range strings.Split(diff, "\n") {
		m := hunkHeader.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		start, _ := strconv.Atoi(m[1])
		count := 1
		if m[2] != "" {
			count, _ = strconv.Atoi(m[2])
		}
		for i := range count {
			added[start+i] = true
		}
	}
	return added
}

// newDenials judges each added import with the policy can-import service and
// returns a message for every denial the session has not reported on its file
// yet. Each file read again keeps only the denials it still holds.
func (s *hookSession) newDenials(ctx context.Context, deps *appDeps, configPath string, edits []importEdit) ([]string, error) {
	if s.Reported == nil {
		s.Reported = map[string][]string{}
	}
	judged := make([]importEdit, 0, len(edits))
	for _, edit := range edits {
		if len(edit.targets) == 0 {
			delete(s.Reported, edit.file)
			continue
		}
		judged = append(judged, edit)
	}
	if len(judged) == 0 {
		return nil, nil
	}
	cfg, err := loadAnalysisConfig(ctx, configPath)
	if err != nil {
		return nil, err
	}
	q, err := newPolicyQuery(ctx, configPath, "", cfg, deps)
	if err != nil {
		return nil, err
	}
	service := newPolicyQueryService(q)
	var msgs []string
	for _, edit := range judged {
		answers, err := service.CanImport(ctx, application.CanImportRequest{
			Policy: cfg.PolicySnapshot(), BundleDir: bundleDirOf(configPath), From: edit.file, Targets: edit.targets, Now: time.Now(),
		})
		if err != nil {
			return nil, err
		}
		var held []string
		for _, answer := range answers {
			for _, d := range answer.Denials {
				held = append(held, d.FindingID)
				if !slices.Contains(s.Reported[edit.file], d.FindingID) {
					msgs = append(msgs, denialMessage(edit.file, answer.Target, d.RuleID, d.FindingID, d.Why, d.Constraints))
				}
			}
		}
		if len(held) == 0 {
			delete(s.Reported, edit.file)
			continue
		}
		slices.Sort(held)
		s.Reported[edit.file] = slices.Compact(held)
	}
	return msgs, nil
}

// denialMessage names the file, the target, the rule, why, and what to do
// instead, as check's text record does: why is the rule's rationale, else its
// first constraint; instead are its declared alternatives.
func denialMessage(file, target, ruleID, findingID, why string, constraints []string) string {
	var rationale, plain, instead []string
	for _, c := range constraints {
		switch {
		case strings.HasPrefix(c, constraintRationale):
			rationale = append(rationale, strings.TrimPrefix(c, constraintRationale))
		case strings.HasPrefix(c, constraintAlternative):
			instead = append(instead, strings.TrimPrefix(c, constraintAlternative))
		default:
			plain = append(plain, c)
		}
	}
	if reasons := slices.Concat(rationale, plain); len(reasons) > 0 {
		why = reasons[0]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "archfit: %s imports %s, which rule %s denies (finding %s)\n", file, target, ruleID, findingID)
	fmt.Fprintf(&b, "  why: %s", why)
	for _, alt := range instead {
		fmt.Fprintf(&b, "\n  instead: %s", alt)
	}
	return b.String()
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
