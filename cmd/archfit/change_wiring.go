package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"golang.org/x/mod/modfile"

	"github.com/alexei-led/archfit/v3/internal/application/canquery"
	"github.com/alexei-led/archfit/v3/internal/application/changecheck"
	"github.com/alexei-led/archfit/v3/internal/application/rulesquery"
	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition/checkfacts"
	"github.com/alexei-led/archfit/v3/internal/factcache"
	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/output/record"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// This file is the composition root of `check`, `can` and `rules` on a
// version 3 policy: it gives the ports of internal/application/changecheck,
// canquery and rulesquery their adapters over git, the file system and the
// check facts. The commands themselves live in change_check.go, can.go and
// rules.go.

// lastFile is where check and can keep their last output for
// `archfit request`, under the repository root.
var lastFile = filepath.Join(cacheDirName, "last.json")

// actionRepo is the Action a protecting workflow uses (D6).
const actionRepo = "alexei-led/archfit-action"

// usesAction matches a step that uses the Action: a "uses:" key, quoted or
// not, whose value is the Action at a ref. A comment, another key, or another
// Action with the same prefix does not match.
var usesAction = regexp.MustCompile(`(?m)^[ \t]*(?:-[ \t]+)?uses:[ \t]*["']?` + regexp.QuoteMeta(actionRepo) + `@`)

// workflowsDir holds the GitHub workflows, under the repository root.
var workflowsDir = filepath.Join(".github", "workflows")

// checkout is the repository a version 3 command reads.
type checkout struct {
	// root is the repository root; start is the directory the command runs
	// in (-C), against which a relative --policy is read.
	root, start string
	runner      toolrun.Runner
	engine      string
	now         func() time.Time
}

// openCheckout finds the repository of dir. Not a git repository is an
// error: the version 3 commands compare git trees.
func openCheckout(ctx context.Context, deps *appDeps, dir string) (checkout, error) {
	start, err := filepath.Abs(dir)
	if err != nil {
		return checkout{}, err
	}
	root, err := gitpkg.RepoRoot(ctx, start, deps.Runner)
	if err != nil {
		return checkout{}, fmt.Errorf("%s is not in a git repository: %w", start, err)
	}
	// git names the root with its symlinks resolved (/private/var on macOS):
	// the start directory must be named the same way to compare with it.
	if resolved, err := filepath.EvalSymlinks(start); err == nil {
		start = resolved
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return checkout{root: root, start: start, runner: deps.Runner, engine: version, now: deps.clock()}, nil
}

// policyAt returns the bytes of the policy file a version 3 command reads:
// file, or .archfit.yaml at the repository root when file is empty. ok is false
// when there is no file.
func (co checkout) policyAt(file string) (data []byte, ok bool, err error) {
	data, err = os.ReadFile(co.policyPath(file))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

// policyPath is the path of a policy file: .archfit.yaml at the root when file
// is empty, else file, relative to the root or absolute (policyArg).
func (co checkout) policyPath(file string) string {
	switch {
	case file == "":
		return filepath.Join(co.root, changecheck.PolicyPath)
	case filepath.IsAbs(file):
		return file
	default:
		return filepath.Join(co.root, filepath.FromSlash(file))
	}
}

// policyArg names a --policy file as changecheck and the record use it: read
// against the start directory (-C, or the current one), then written relative
// to the repository root, or absolute outside it. So `--policy .archfit.yaml`
// in a subdirectory names that directory's file, not the root's.
func (co checkout) policyArg(file string) string {
	abs := file
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(co.start, file)
	}
	rel, err := filepath.Rel(co.root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return abs
	}
	return filepath.ToSlash(rel)
}

// isVersion3 reports whether data declares policy version 3, valid or not: the
// version 3 commands judge it and report what is wrong with it. A file that
// does not parse, or names another version, is not version 3. As in
// policyfile, a number with no fraction is an integer: 3.0 is 3.
func isVersion3(data []byte) bool {
	var header struct {
		Version any `yaml:"version"`
	}
	if yaml.Unmarshal(data, &header) != nil {
		return false
	}
	switch v := header.Version.(type) {
	case uint64:
		return v == policyfile.Version
	case int64:
		return v == policyfile.Version
	case float64:
		return v == policyfile.Version
	}
	return false
}

func (co checkout) changeCheck() changecheck.Service {
	return changecheck.Service{
		Policies:   policySources{co},
		Repository: repository{co},
		Facts:      checkFacts{co},
		Protection: protection{co},
		Store:      lastStore{co},
		Clock:      clockFunc(co.now),
		Engine:     co.engine,
	}
}

func (co checkout) canQuery() canquery.Service {
	src := localPolicy{co}
	return canquery.Service{Policies: src, Tree: &tree{co: co, policy: src}, Go: &goModules{co: co}}
}

func (co checkout) rulesQuery() rulesquery.Service {
	return rulesquery.Service{Policies: localPolicy{co}, Changed: changedFiles{co}}
}

// stamp is what a `can` answer records about the run.
func (co checkout) stamp(ctx context.Context) (canquery.Stamp, error) {
	protected, err := protection{co}.Protected(ctx)
	if err != nil {
		return canquery.Stamp{}, err
	}
	head, _, err := gitpkg.Lookup(ctx, co.root, "HEAD", co.runner)
	if err != nil {
		return canquery.Stamp{}, err
	}
	return canquery.Stamp{
		PolicyPath: changecheck.PolicyPath, Protected: protected, Head: head,
		Date: co.now().UTC().Format(time.DateOnly), Engine: co.engine,
	}, nil
}

// workingTree is the git tree id of the working tree.
func (co checkout) workingTree(ctx context.Context) (string, error) {
	return gitpkg.WorkingTreeID(ctx, co.root, co.runner)
}

// saveAnswer keeps a `can` answer as the last output, under the tree id read
// before the answer.
func (co checkout) saveAnswer(tree string, a result.CanAnswer) error {
	var doc bytes.Buffer
	if err := record.AnswerJSON(&doc, a); err != nil {
		return err
	}
	return co.saveLast(tree, doc.Bytes())
}

// saveLast writes .archfit-cache/last.json: {"tree": tree, "output": doc}.
// It writes a temporary file and renames it, so a reader never sees half a
// file.
func (co checkout) saveLast(tree string, doc []byte) error {
	data, err := json.Marshal(struct {
		Tree   string          `json:"tree"`
		Output json.RawMessage `json:"output"`
	}{tree, doc})
	if err != nil {
		return err
	}
	file := filepath.Join(co.root, lastFile)
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), "last-*.json")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

// readPolicy reads one policy file into the form changecheck uses. A version
// 2 file is found, not valid, and marked.
func readPolicy(data []byte) changecheck.PolicyFile {
	p, err := policyfile.Resolve(data)
	return changecheck.PolicyFile{
		Found: true, Digest: resolved.Digest(data), Policy: p, Err: err,
		Version2: errors.Is(err, policyfile.ErrVersion2),
	}
}

// policySources is changecheck.Policies: the local file from disk, the target's
// from git objects.
type policySources struct{ co checkout }

func (p policySources) Local(_ context.Context, file string) (changecheck.PolicyFile, error) {
	data, ok, err := p.co.policyAt(file)
	if err != nil || !ok {
		return changecheck.PolicyFile{}, err
	}
	return readPolicy(data), nil
}

func (p policySources) Target(ctx context.Context, commit string) (changecheck.PolicyFile, error) {
	data, ok, err := gitpkg.FileAt(ctx, p.co.root, commit, changecheck.PolicyPath, p.co.runner)
	if err != nil || !ok {
		return changecheck.PolicyFile{}, err
	}
	return readPolicy(data), nil
}

// repository is changecheck.Repository over internal/history/git.
type repository struct{ co checkout }

func (r repository) OriginHead(ctx context.Context) (string, bool, error) {
	ref, ok := gitpkg.OriginHead(ctx, r.co.root, r.co.runner)
	return ref, ok, nil
}

func (r repository) Commit(ctx context.Context, ref string) (string, bool, error) {
	return gitpkg.Lookup(ctx, r.co.root, ref, r.co.runner)
}

func (r repository) Parent(ctx context.Context, commit string) (string, bool, error) {
	return gitpkg.Lookup(ctx, r.co.root, commit+"^", r.co.runner)
}

func (r repository) MergeBase(ctx context.Context, a, b string) (string, bool, error) {
	return gitpkg.MergeBase(ctx, r.co.root, a, b, r.co.runner)
}

func (r repository) AddedLines(ctx context.Context, base string) (evidence.AddedLines, error) {
	return gitpkg.AddedLines(ctx, r.co.root, base, r.co.runner)
}

func (r repository) WorkingTree(ctx context.Context) (string, error) {
	return r.co.workingTree(ctx)
}

func (r repository) Files(ctx context.Context, tree string) ([]string, error) {
	return gitpkg.TreeFiles(ctx, r.co.root, tree, r.co.runner)
}

// checkFacts is changecheck.Facts: the check facts, read with the settings of
// the policy that judges.
type checkFacts struct{ co checkout }

func (f checkFacts) Head(ctx context.Context, p resolved.Policy) (evidence.CheckFacts, error) {
	return f.acquirer(p).Head(ctx, f.co.root)
}

func (f checkFacts) Base(ctx context.Context, p resolved.Policy, commit string) (evidence.CheckFacts, error) {
	return f.acquirer(p).Base(ctx, f.co.root, commit)
}

func (f checkFacts) acquirer(p resolved.Policy) *checkfacts.Acquirer {
	return f.co.facts(checkfacts.ConfigFrom(p))
}

// facts reads the check facts with cfg. It caches facts by tree under
// .archfit-cache/facts. A dev build cannot name its own code, so it reads
// every fact afresh.
func (co checkout) facts(cfg checkfacts.Config) *checkfacts.Acquirer {
	a := &checkfacts.Acquirer{Runner: co.runner, Config: cfg}
	if co.engine != devVersion {
		a.Store, a.Version = factcache.NewStore(factsCacheDir(co.root)), co.engine
	}
	return a
}

// protection is the milestone 1 test of design 9.4 (D6): the policy is
// protected when a workflow uses the Archfit Action.
type protection struct{ co checkout }

func (p protection) Protected(context.Context) (bool, error) {
	dir := filepath.Join(p.co.root, workflowsDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if ext := filepath.Ext(e.Name()); e.IsDir() || (ext != ".yml" && ext != ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // a workflow of this repository
		if err != nil {
			return false, err
		}
		if usesAction.Match(data) {
			return true, nil
		}
	}
	return false, nil
}

// lastStore is changecheck.Store.
type lastStore struct{ co checkout }

func (s lastStore) Save(_ context.Context, tree string, r result.Result) error {
	var doc bytes.Buffer
	if err := record.ResultJSON(&doc, r); err != nil {
		return err
	}
	return s.co.saveLast(tree, doc.Bytes())
}

// clockFunc is changecheck.Clock.
type clockFunc func() time.Time

func (f clockFunc) Now() time.Time { return f() }

// errNoPolicy is the error of can and rules without a policy file.
var errNoPolicy = errors.New("no " + changecheck.PolicyPath + ": run archfit init")

// localPolicy is the PolicySource of can and rules: .archfit.yaml at the root.
type localPolicy struct{ co checkout }

func (l localPolicy) Policy(context.Context) (resolved.Policy, error) {
	data, ok, err := l.co.policyAt("")
	switch {
	case err != nil:
		return resolved.Policy{}, err
	case !ok:
		return resolved.Policy{}, errNoPolicy
	}
	p, err := policyfile.Resolve(data)
	if err != nil {
		return resolved.Policy{}, fmt.Errorf("%s: %w", changecheck.PolicyPath, err)
	}
	return p, nil
}

// tree is canquery.Tree over the working tree. A directory's files are the
// sources the head check facts list, read once and only when a directory is
// asked for.
type tree struct {
	co     checkout
	policy localPolicy
	facts  *evidence.CheckFacts
}

func (t *tree) Entry(ctx context.Context, rel string) (canquery.Entry, error) {
	if rel == ".." || strings.HasPrefix(rel, "../") || path.IsAbs(rel) {
		return canquery.Entry{}, nil
	}
	info, err := os.Stat(filepath.Join(t.co.root, filepath.FromSlash(rel)))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return canquery.Entry{}, nil
	case err != nil:
		return canquery.Entry{}, err
	case !info.IsDir():
		return canquery.Entry{Kind: canquery.KindFile}, nil
	}
	facts, err := t.headFacts(ctx)
	if err != nil {
		return canquery.Entry{}, err
	}
	files, err := dirFiles(facts, rel)
	if err != nil {
		return canquery.Entry{}, err
	}
	return canquery.Entry{Kind: canquery.KindDir, Files: files}, nil
}

func (t *tree) headFacts(ctx context.Context) (evidence.CheckFacts, error) {
	if t.facts != nil {
		return *t.facts, nil
	}
	p, err := t.policy.Policy(ctx)
	if err != nil {
		return evidence.CheckFacts{}, err
	}
	facts, err := checkFacts{t.co}.Head(ctx, p)
	if err != nil {
		return evidence.CheckFacts{}, err
	}
	t.facts = &facts
	return facts, nil
}

// dirFiles returns the sources directly in dir. The sources are not all known
// when a tool is missing, a tree could not be read, or a file of dir is
// unknown and left out of the sources (a file that does not parse, a cgo file
// with CGO_ENABLED unset): a directory missing a file could answer yes where
// the file answers no. That is an error, which `can` reports as no verdict. An
// import that cannot be resolved in a listed file leaves the sources known.
func dirFiles(facts evidence.CheckFacts, dir string) ([]string, error) {
	listed := make(map[string]bool, len(facts.Sources))
	for _, f := range facts.Sources {
		listed[f] = true
	}
	for _, u := range facts.Unknown {
		dropped := u.Cause == evidence.CauseImport && path.Dir(u.File) == dir && !listed[u.File]
		if u.Cause != evidence.CauseImport || dropped {
			return nil, fmt.Errorf("the files of %s are not all known: %s: %s", dir, u.Reason, u.Fix)
		}
	}
	var files []string
	for _, f := range facts.Sources {
		if path.Dir(f) == dir {
			files = append(files, f)
		}
	}
	return files, nil
}

// goModules is canquery.GoModules: the module line of each go.mod of the
// working tree.
type goModules struct {
	co      checkout
	modules map[string]string // module path -> repository-relative directory
}

func (g *goModules) Dir(ctx context.Context, importPath string) (string, bool, error) {
	if g.modules == nil {
		if err := g.read(ctx); err != nil {
			return "", false, err
		}
	}
	// The longest module path that holds the import path owns it.
	var best string
	for mod := range g.modules {
		if (importPath == mod || strings.HasPrefix(importPath, mod+"/")) && len(mod) > len(best) {
			best = mod
		}
	}
	if best == "" {
		return "", false, nil
	}
	return path.Join(g.modules[best], strings.TrimPrefix(importPath, best)), true, nil
}

func (g *goModules) read(ctx context.Context) error {
	id, err := g.co.workingTree(ctx)
	if err != nil {
		return err
	}
	files, err := gitpkg.TreeFiles(ctx, g.co.root, id, g.co.runner)
	if err != nil {
		return err
	}
	g.modules = map[string]string{}
	for _, f := range files {
		if path.Base(f) != "go.mod" || goIgnores(path.Dir(f)) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(g.co.root, filepath.FromSlash(f)))
		if err != nil {
			return err
		}
		mod, dir := modfile.ModulePath(data), path.Dir(f)
		if mod == "" {
			continue
		}
		// Two go.mod files that declare one module: the shallower one, then
		// the first by name, as check's root module is.
		if prev, ok := g.modules[mod]; ok && !shallower(dir, prev) {
			continue
		}
		g.modules[mod] = dir
	}
	return nil
}

// goIgnores reports whether the go command ignores the directory: a segment
// named testdata or vendor, or one that starts with . or _.
func goIgnores(dir string) bool {
	if dir == "." {
		return false
	}
	for seg := range strings.SplitSeq(dir, "/") {
		if seg == "testdata" || seg == "vendor" || strings.HasPrefix(seg, ".") || strings.HasPrefix(seg, "_") {
			return true
		}
	}
	return false
}

// shallower orders two directories by depth, then by name.
func shallower(a, b string) bool {
	depth := func(d string) int {
		if d == "." {
			return 0
		}
		return strings.Count(d, "/") + 1
	}
	if da, db := depth(a), depth(b); da != db {
		return da < db
	}
	return a < b
}

// changedFiles is rulesquery.ChangedFiles: the files with lines the change
// adds against the base `check` uses.
type changedFiles struct{ co checkout }

func (c changedFiles) Changed(ctx context.Context) ([]string, error) {
	repo := repository(c)
	head, _, err := repo.Commit(ctx, "HEAD")
	if err != nil {
		return nil, err
	}
	t, err := changecheck.Base(ctx, repo, "", head)
	if err != nil {
		return nil, err
	}
	if t.Base == "" {
		// rules takes no --base: the way out is a path or a rule id.
		return nil, fmt.Errorf("%w: %s: name a path or a rule id", errNoChanged, t.What)
	}
	added, err := repo.AddedLines(ctx, t.Base)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(added))
	for f := range added {
		files = append(files, f)
	}
	slices.Sort(files)
	return files, nil
}

// errNoChanged is the error of `rules` with no argument when the change has
// no base to list its files against.
var errNoChanged = errors.New("the changed files are not known")
