package changecheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/application/changecheck"
	"github.com/alexei-led/archfit/v3/internal/application/request"
	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/output/record"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
	"github.com/alexei-led/archfit/v3/internal/testutil"
)

// The commits and the tree of the checkout. head and base are those of the
// result goldens.
const (
	mainRef      = "origin/main"
	tcNetem      = "pkg/tc/netem.go"
	tcIface      = "pkg/tc/interface.go"
	chaosFile    = "pkg/chaos/netem/netem.go"
	noTargetNote = "origin/main has no .archfit.yaml: judged with the local policy"
	headSHA      = "1a954628a960aaef81d7b2d4521929579f3541e6"
	baseSHA      = "1405df66cbe219b0bf6355bc3d60361a8376b6b4"
	mainSHA      = "9d5ed678fe57bcca610140957afab571f0bf8d76"
	releaseSHA   = "c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00"
	forkSHA      = "5eed5eed5eed5eed5eed5eed5eed5eed5eed5eed"
	treeID       = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	candidate    = "candidate.yaml"
	engine       = "4.0.0"
)

// policyYAML is the policy of the examples of design 8.1.
const policyYAML = `version: 3
owners: [alexei-led]
modules:
  pkg_chaos:
    paths: [pkg/chaos/**]
    depends_on: [pkg_runtime, pkg_util]
  pkg_runtime:
    paths: [pkg/runtime/**]
  pkg_tc:
    paths: [pkg/tc/**]
  pkg_util:
    paths: [pkg/util/**]
    volatility: low
rules:
  - id: no-chaos-to-runtime
    deny: { from: pkg_chaos, to: pkg_runtime }
    why: A direct import ties a chaos action to one runtime.
    instead: Add a method to container.Client.
`

const expiredYAML = policyYAML + `exceptions:
  - rule: duplication
    from: pkg_tc
    to: pkg_util
    expires: 2026-11-19
    why: "Request #431 is open."
`

const copyText = "unexpected netem output"

var sources = []string{
	chaosFile, "pkg/runtime/docker/client.go", tcIface, tcNetem,
	"pkg/util/util.go",
}

var (
	tcToUtil     = evidence.ImportFact{File: tcNetem, Line: 12, Target: "pkg/util"}
	chaosToDock  = evidence.ImportFact{File: chaosFile, Line: 8, Target: "pkg/runtime/docker"}
	chaosToUtil  = evidence.ImportFact{File: chaosFile, Line: 5, Target: "pkg/util"}
	utilCopy     = evidence.LiteralOccurrence{File: "pkg/util/util.go", Line: 15, Value: copyText, Shape: evidence.ShapeOther}
	tcCopy       = evidence.LiteralOccurrence{File: tcIface, Line: 12, Value: copyText, Shape: evidence.ShapeOther}
	baseUnreadIt = evidence.UnknownItem{Specifier: baseSHA, Reason: "the base cannot be read", Cause: evidence.CauseBase, Fix: "run git fetch, then archfit check again"}
)

func tree(imports []evidence.ImportFact, lits ...evidence.LiteralOccurrence) evidence.CheckFacts {
	return evidence.CheckFacts{Sources: sources, Imports: imports, Literals: lits}
}

// file reads a policy as the policy adapter does.
func file(text string) changecheck.PolicyFile {
	data := []byte(text)
	p, err := policyfile.Resolve(data)
	return changecheck.PolicyFile{
		Found: true, Digest: resolved.Digest(data), Policy: p, Err: err, Version2: errors.Is(err, policyfile.ErrVersion2),
	}
}

// fail holds the error each port method returns, by method name.
type fail map[string]error

type repo struct {
	fail       fail
	origin     string
	commits    map[string]string
	parents    map[string]string
	mergeBases map[string]string // by the target commit
	added      evidence.AddedLines
	addedBase  string
	// files are the files of every tree; filesOf the tree ids they were
	// listed for.
	files   []string
	filesOf []string
	// trees are the ids WorkingTree returns, one per call, the last repeated.
	trees     []string
	treeReads int
}

func (r *repo) OriginHead(context.Context) (string, bool, error) {
	return r.origin, r.origin != "", r.fail["OriginHead"]
}

func (r *repo) Commit(_ context.Context, ref string) (string, bool, error) {
	sha, ok := r.commits[ref]
	return sha, ok, r.fail["Commit"]
}

func (r *repo) Parent(_ context.Context, commit string) (string, bool, error) {
	sha, ok := r.parents[commit]
	return sha, ok, r.fail["Parent"]
}

func (r *repo) MergeBase(_ context.Context, a, b string) (string, bool, error) {
	if a != headSHA {
		return "", false, errors.New("merge base of " + a)
	}
	sha, ok := r.mergeBases[b]
	return sha, ok, r.fail["MergeBase"]
}

func (r *repo) AddedLines(_ context.Context, base string) (evidence.AddedLines, error) {
	r.addedBase = base
	return r.added, r.fail["AddedLines"]
}

func (r *repo) WorkingTree(context.Context) (string, error) {
	id := r.trees[min(r.treeReads, len(r.trees)-1)]
	r.treeReads++
	return id, r.fail["WorkingTree"]
}

func (r *repo) Files(_ context.Context, tree string) ([]string, error) {
	r.filesOf = append(r.filesOf, tree)
	return r.files, r.fail["Files"]
}

type policies struct {
	fail   fail
	local  map[string]changecheck.PolicyFile // by path
	target map[string]changecheck.PolicyFile // by commit
}

func (p *policies) Local(_ context.Context, path string) (changecheck.PolicyFile, error) {
	return p.local[path], p.fail["Local"]
}

func (p *policies) Target(_ context.Context, commit string) (changecheck.PolicyFile, error) {
	return p.target[commit], p.fail["Target"]
}

type facts struct {
	fail       fail
	head, base evidence.CheckFacts
	headReads  int
	baseReads  []string
	// digests are the digests of the policies the facts were read with.
	digests []string
}

func (f *facts) Head(_ context.Context, p resolved.Policy) (evidence.CheckFacts, error) {
	f.headReads++
	f.digests = append(f.digests, p.Digest)
	return f.head, f.fail["Head"]
}

func (f *facts) Base(_ context.Context, p resolved.Policy, commit string) (evidence.CheckFacts, error) {
	f.baseReads = append(f.baseReads, commit)
	f.digests = append(f.digests, p.Digest)
	return f.base, f.fail["Base"]
}

type protection struct {
	on  bool
	err error
}

func (p protection) Protected(context.Context) (bool, error) { return p.on, p.err }

type saved struct {
	tree string
	res  result.Result
}

type store struct {
	saves []saved
	err   error
}

func (s *store) Save(_ context.Context, tree string, r result.Result) error {
	s.saves = append(s.saves, saved{tree, r})
	return s.err
}

type clock time.Time

func (c clock) Now() time.Time { return time.Time(c) }

// world is a checkout on a branch of origin/main that adds tcToUtil: the
// graph example of design 8.1.
type world struct {
	repo       *repo
	policies   *policies
	facts      *facts
	protection protection
	store      *store
	today      string
}

func newWorld() *world {
	return &world{
		repo: &repo{
			origin:     mainRef,
			commits:    map[string]string{"HEAD": headSHA, mainRef: mainSHA},
			parents:    map[string]string{},
			mergeBases: map[string]string{mainSHA: baseSHA},
			added:      evidence.AddedLines{tcNetem: {12}},
			files:      append(slices.Clone(sources), "README.md"),
			trees:      []string{treeID},
		},
		policies: &policies{
			local:  map[string]changecheck.PolicyFile{changecheck.PolicyPath: file(policyYAML)},
			target: map[string]changecheck.PolicyFile{mainSHA: file(policyYAML)},
		},
		facts:      &facts{head: tree([]evidence.ImportFact{tcToUtil}), base: tree(nil)},
		protection: protection{on: true},
		store:      &store{},
		today:      "2026-11-10",
	}
}

func (w *world) service(t *testing.T) changecheck.Service {
	t.Helper()
	today, err := time.Parse(time.DateOnly, w.today)
	if err != nil {
		t.Fatal(err)
	}
	return changecheck.Service{
		Policies: w.policies, Repository: w.repo, Facts: w.facts, Protection: w.protection, Store: w.store,
		Clock: clock(today.Add(15 * time.Hour)), Engine: engine,
	}
}

func (w *world) check(t *testing.T, o changecheck.Options) changecheck.Outcome {
	t.Helper()
	out, err := w.service(t).Check(context.Background(), o)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return out
}

func render(t *testing.T, r result.Result) string {
	t.Helper()
	var b strings.Builder
	if err := record.Result(&b, r, record.Options{}); err != nil {
		t.Fatal(err)
	}
	if words := testutil.NotUsedIn(b.String()); len(words) > 0 {
		t.Errorf("the record uses %q:\n%s", words, b.String())
	}
	return b.String()
}

func jsonOf(t *testing.T, r result.Result) string {
	t.Helper()
	var b bytes.Buffer
	if err := record.ResultJSON(&b, r); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestDesignExamples runs each check example of design 8.1 through the whole
// flow and compares the record with the text golden of internal/output/record
// byte for byte, and the JSON with the result golden of internal/model/result.
// The JSON goldens come from pumba: their policy digest is of pumba's file, and
// their keys name Go import paths where the check facts give directories, so
// the test puts this fixture's digest and key into the golden first.
func TestDesignExamples(t *testing.T) {
	tests := []struct {
		name  string
		setup func(w *world)
		key   string
	}{
		{name: "pass", setup: func(w *world) {
			w.facts.head = tree([]evidence.ImportFact{chaosToUtil})
			w.facts.base = w.facts.head
			w.repo.added = nil
		}},
		{name: "graph", key: "graph pkg_tc pkg_util pkg/util", setup: func(*world) {}},
		{name: "deny", key: "no-chaos-to-runtime pkg_chaos pkg_runtime pkg/runtime/docker", setup: func(w *world) {
			w.facts.head = tree([]evidence.ImportFact{chaosToDock})
			w.repo.added = evidence.AddedLines{chaosToDock.File: {chaosToDock.Line}}
		}},
		{name: "duplication", key: "duplication pkg_tc pkg_util " + resolved.Digest([]byte(copyText)), setup: func(w *world) {
			w.facts.head = tree(nil, utilCopy, tcCopy)
			w.facts.base = tree(nil, utilCopy)
			w.repo.added = evidence.AddedLines{tcCopy.File: {tcCopy.Line}}
		}},
		{name: "expired_exception", setup: func(w *world) {
			w.policies.local[changecheck.PolicyPath] = file(expiredYAML)
			w.policies.target[mainSHA] = file(expiredYAML)
			w.facts.head = tree(nil, utilCopy, tcCopy)
			w.facts.base = w.facts.head
			w.repo.added = nil
			w.today = "2026-11-20"
		}},
		{name: "policy_differs", setup: func(w *world) {
			w.policies.local[changecheck.PolicyPath] = file(policyYAML + "# edited\n")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld()
			tt.setup(w)
			got := w.check(t, changecheck.Options{}).Result

			text, err := os.ReadFile(filepath.Join("..", "..", "output", "record", "testdata", tt.name+".txt"))
			if err != nil {
				t.Fatal(err)
			}
			if record := render(t, got); record != string(text) {
				t.Errorf("record:\n--- got\n%s--- want\n%s", record, text)
			}

			data, err := os.ReadFile(filepath.Join("..", "..", "model", "result", "testdata", tt.name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var want result.Result
			if err := json.Unmarshal(data, &want); err != nil {
				t.Fatal(err)
			}
			local := w.policies.local[changecheck.PolicyPath]
			if got.Policy.Digest != local.Digest {
				t.Errorf("digest = %s, want the local file's %s", got.Policy.Digest, local.Digest)
			}
			want.Policy.Digest = local.Digest
			if tt.key != "" {
				want.Errors[0].Key = tt.key
			}
			if g, w := jsonOf(t, got), jsonOf(t, want); g != w {
				t.Errorf("JSON:\n--- got\n%s--- want\n%s", g, w)
			}
		})
	}
}

// want is what one check case expects: the record, the notes, and what the
// flow read and stored.
type want struct {
	record string
	notes  []string
	// policy is the result's policy, without the digest.
	target  string
	differs bool
	base    string
	// facts says whether the facts of the head and the base were read.
	facts bool
	// stored is false for a preview.
	stored bool
}

const (
	graphRecord = "pkg/tc/netem.go:12: graph: pkg_tc -> pkg_util is not an allowed dependency\n" +
		"  why: a new dependency between modules needs an owner\n" +
		"  balance: 9/10 (min 7)\n" +
		"  next: archfit request -m \"<why you need it>\"     (do not copy the code)\n" +
		"archfit: fail, 1 error\n"
	noVerdict  = "archfit: unknown, no verdict\n"
	whyCompare = "  why: the check compares the change with the merge base of its target branch\n"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name  string
		setup func(w *world)
		opts  changecheck.Options
		want  want
	}{
		{
			name:  "a new dependency fails",
			setup: func(*world) {},
			want:  want{record: graphRecord, target: mainRef, base: baseSHA, facts: true, stored: true},
		},
		{
			name: "one more use of a dependency that already breaks a rule fails",
			setup: func(w *world) {
				second := tcToUtil
				second.File, second.Line = tcIface, 4
				w.facts.base = tree([]evidence.ImportFact{tcToUtil})
				w.facts.head = tree([]evidence.ImportFact{tcToUtil, second})
				w.repo.added = evidence.AddedLines{tcIface: {4}}
			},
			want: want{
				record: "pkg/tc/interface.go:4: graph: pkg_tc -> pkg_util is not an allowed dependency\n" +
					strings.SplitN(graphRecord, "\n", 2)[1],
				target: mainRef, base: baseSHA, facts: true, stored: true,
			},
		},
		{
			name:  "--base names the target branch instead of origin/HEAD",
			setup: func(w *world) { w.withRelease() },
			opts:  changecheck.Options{Base: "origin/release"},
			want:  want{record: graphRecord, target: "origin/release", base: forkSHA, facts: true, stored: true},
		},
		{
			name:  "no --base and no origin/HEAD",
			setup: func(w *world) { w.repo.origin = "" },
			want: want{
				record: "base: no target branch: origin/HEAD is not set\n" + whyCompare +
					"  next: pass --base <target branch>\n" + noVerdict,
				stored: true,
			},
		},
		{
			name:  "--base names no commit",
			setup: func(*world) {},
			opts:  changecheck.Options{Base: "origin/gone"},
			want: want{
				record: "base: origin/gone names no commit in this clone\n" + whyCompare + "  next: git fetch\n" + noVerdict,
				stored: true,
			},
		},
		{
			name:  "a shallow clone has no merge base",
			setup: func(w *world) { delete(w.repo.mergeBases, mainSHA) },
			want: want{
				record: "base: HEAD and origin/main have no merge base in this clone\n" + whyCompare +
					"  next: git fetch --unshallow\n" + noVerdict,
				stored: true,
			},
		},
		{
			name:  "HEAD at the tip of the target branch is compared with its first parent",
			setup: func(w *world) { w.atTip(baseSHA) },
			want:  want{record: graphRecord, target: mainRef, base: baseSHA, facts: true, stored: true},
		},
		{
			name:  "HEAD at the tip of the target branch without its parent in the clone",
			setup: func(w *world) { w.atTip("") },
			want: want{
				record: "base: HEAD is the tip of origin/main and its parent is not in this clone\n" + whyCompare +
					"  next: git fetch --unshallow\n" + noVerdict,
				stored: true,
			},
		},
		{
			// The commits HEAD shares with the target are on the target: they
			// add nothing, so the base is HEAD, not its parent.
			name:  "HEAD behind the tip of the target branch is compared with itself",
			setup: func(w *world) { w.repo.mergeBases[mainSHA] = headSHA },
			want:  want{record: graphRecord, target: mainRef, base: headSHA, facts: true, stored: true},
		},
		{
			name: "a module whose files no extractor reads still matches them",
			setup: func(w *world) {
				withWeb := strings.Replace(policyYAML, "rules:\n", "  web:\n    paths: [web/**]\nrules:\n", 1)
				w.policies.local[changecheck.PolicyPath] = file(withWeb)
				w.policies.target[mainSHA] = file(withWeb)
				w.repo.files = append(w.repo.files, "web/app.ts")
			},
			want: want{record: graphRecord, target: mainRef, base: baseSHA, facts: true, stored: true},
		},
		{
			name:  "a working tree edited during the check stores nothing",
			setup: func(w *world) { w.repo.trees = []string{treeID, "0e1d2c3b4a5968778695a4b3c2d1e0f9e8d7c6b5"} },
			want: want{
				record: graphRecord, target: mainRef, base: baseSHA, facts: true,
				notes: []string{"the working tree changed during the check, so archfit request cannot use this result: run archfit check again"},
			},
		},
		{
			name: "a code change that also edits the policy has no verdict",
			setup: func(w *world) {
				w.policies.local[changecheck.PolicyPath] = file(policyYAML + "# edited\n")
			},
			want: want{
				record: ".archfit.yaml: policy: differs from origin/main\n  why: only owners change the policy\n" +
					"  next: git checkout origin/main -- .archfit.yaml\n" + noVerdict,
				target: mainRef, differs: true, base: baseSHA, stored: true,
			},
		},
		{
			name:  "a deleted local policy differs from the target's",
			setup: func(w *world) { delete(w.policies.local, changecheck.PolicyPath) },
			want: want{
				record: ".archfit.yaml: policy: differs from origin/main\n  why: only owners change the policy\n" +
					"  next: git checkout origin/main -- .archfit.yaml\n" + noVerdict,
				target: mainRef, differs: true, base: baseSHA, stored: true,
			},
		},
		{
			name: "--policy previews a candidate without the comparison",
			setup: func(w *world) {
				w.policies.local[candidate] = file(strings.Replace(policyYAML,
					"  pkg_tc:\n    paths: [pkg/tc/**]\n", "  pkg_tc:\n    paths: [pkg/tc/**]\n    depends_on: [pkg_util]\n", 1))
			},
			opts: changecheck.Options{Policy: candidate},
			want: want{record: "archfit: pass\n", base: baseSHA, facts: true},
		},
		{
			name:  "no policy on the target branch: judged with the local policy",
			setup: func(w *world) { delete(w.policies.target, mainSHA) },
			want: want{
				record: graphRecord, base: baseSHA, facts: true, stored: true,
				notes: []string{noTargetNote},
			},
		},
		{
			name:  "a version 2 policy on the target branch: judged with the local policy",
			setup: func(w *world) { w.policies.target[mainSHA] = file("version: 2\nmodules: {}\n") },
			want: want{
				record: graphRecord, base: baseSHA, facts: true, stored: true,
				notes: []string{"origin/main has a version 2 .archfit.yaml: judged with the local policy until its conversion pull request merges"},
			},
		},
		{
			name:  "an invalid version 3 policy on the target branch",
			setup: func(w *world) { w.policies.target[mainSHA] = file("version: 3\nowners: [alexei-led]\n") },
			want: want{
				record: ".archfit.yaml: policy: modules: required: declare at least one module\n" +
					"  why: the policy on origin/main is not valid\n" +
					"  next: ask an owner to fix .archfit.yaml on origin/main\n" + noVerdict,
				target: mainRef, differs: true, base: baseSHA, stored: true,
			},
		},
		{
			name: "an invalid local policy",
			setup: func(w *world) {
				w.policies.local[changecheck.PolicyPath] = file("version: 3\nowners: []\nmodules: {}\n")
				delete(w.policies.target, mainSHA)
			},
			want: want{
				record: ".archfit.yaml: policy: owners: required: list at least one GitHub login\n" +
					"  why: the check cannot judge with a policy that is not valid\n  next: fix .archfit.yaml\n" +
					".archfit.yaml: policy: modules: required: declare at least one module\n" +
					"  why: the check cannot judge with a policy that is not valid\n  next: fix .archfit.yaml\n" + noVerdict,
				base: baseSHA, stored: true,
				notes: []string{noTargetNote},
			},
		},
		{
			name: "no policy anywhere",
			setup: func(w *world) {
				w.policies.local = map[string]changecheck.PolicyFile{}
				w.policies.target = map[string]changecheck.PolicyFile{}
			},
			want: want{
				record: ".archfit.yaml: policy: no policy file\n  next: archfit init\n" + noVerdict,
				base:   baseSHA, stored: true,
				notes: []string{noTargetNote},
			},
		},
		{
			name: "a path of the protected policy matches nothing",
			setup: func(w *world) {
				w.policies.local[changecheck.PolicyPath] = file(policyYAML + "tests: [test/**]\n")
				w.policies.target[mainSHA] = w.policies.local[changecheck.PolicyPath]
			},
			want: want{
				record: `.archfit.yaml: policy: tests: "test/**" matches no file` + "\n" +
					"  why: the policy does not fit the files of the tree\n" +
					"  next: ask an owner to fix .archfit.yaml on origin/main\n" + noVerdict,
				target: mainRef, base: baseSHA, stored: true,
			},
		},
		{
			name: "a path of a candidate policy matches nothing",
			setup: func(w *world) {
				w.policies.local[candidate] = file(policyYAML + "tests: [test/**]\n")
			},
			opts: changecheck.Options{Policy: candidate},
			want: want{
				record: `candidate.yaml: policy: tests: "test/**" matches no file` + "\n" +
					"  why: the policy does not fit the files of the tree\n  next: fix candidate.yaml\n" + noVerdict,
				base: baseSHA,
			},
		},
		{
			name:  "a base that cannot be read has no verdict",
			setup: func(w *world) { w.facts.base = evidence.CheckFacts{Unknown: []evidence.UnknownItem{baseUnreadIt}} },
			want: want{
				record: "base: the base cannot be read\n" +
					"  why: the base cannot be read, so the change cannot be compared with it\n" +
					"  next: run git fetch, then archfit check again\n" + noVerdict,
				target: mainRef, base: baseSHA, facts: true, stored: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld()
			tt.setup(w)
			out := w.check(t, tt.opts)
			if got := render(t, out.Result); got != tt.want.record {
				t.Errorf("record:\n--- got\n%s--- want\n%s", got, tt.want.record)
			}
			for _, note := range out.Notes {
				if words := testutil.NotUsedIn(note); len(words) > 0 {
					t.Errorf("note %q uses %q", note, words)
				}
			}
			if !slices.Equal(out.Notes, tt.want.notes) {
				t.Errorf("notes = %q, want %q", out.Notes, tt.want.notes)
			}
			p := out.Result.Policy
			if p.Target != tt.want.target || p.Differs != tt.want.differs || !p.Protected {
				t.Errorf("policy = %+v, want target %q, differs %v, protected", p, tt.want.target, tt.want.differs)
			}
			if out.Result.Base != tt.want.base || out.Result.Head != headSHA {
				t.Errorf("base, head = %s, %s; want %s, %s", out.Result.Base, out.Result.Head, tt.want.base, headSHA)
			}
			wantPath := changecheck.PolicyPath
			if tt.opts.Policy != "" {
				wantPath = tt.opts.Policy
			}
			if p.Path != wantPath || !strings.HasPrefix(p.Digest, "sha256:") {
				t.Errorf("policy path, digest = %q, %q; want %q and a digest", p.Path, p.Digest, wantPath)
			}
			read := w.facts.headReads > 0 && len(w.facts.baseReads) > 0
			if read != tt.want.facts {
				t.Errorf("facts read = %v (head %d, base %q), want %v", read, w.facts.headReads, w.facts.baseReads, tt.want.facts)
			}
			if read && (!slices.Equal(w.facts.baseReads, []string{tt.want.base}) || w.repo.addedBase != tt.want.base) {
				t.Errorf("base facts of %q, added lines against %q; want %s", w.facts.baseReads, w.repo.addedBase, tt.want.base)
			}
			for _, digest := range w.facts.digests {
				if digest != p.Digest {
					t.Errorf("facts read with the policy %s, want the policy that judges, %s", digest, p.Digest)
				}
			}
			for _, id := range w.repo.filesOf {
				if id != treeID {
					t.Errorf("listed the files of tree %s, want the tree the run started on, %s", id, treeID)
				}
			}
			checkStored(t, w, out.Result, tt.want.stored)
		})
	}
}

// withRelease adds the branch origin/release, forked from HEAD's history at
// forkSHA, with the same policy.
func (w *world) withRelease() {
	w.repo.commits["origin/release"] = releaseSHA
	w.repo.mergeBases[releaseSHA] = forkSHA
	w.policies.target[releaseSHA] = file(policyYAML)
}

// atTip puts HEAD at the tip of origin/main, with parent as its first
// parent; an empty parent is not in the clone.
func (w *world) atTip(parent string) {
	w.repo.commits[mainRef] = headSHA
	if parent != "" {
		w.repo.parents[headSHA] = parent
	}
	w.policies.target = map[string]changecheck.PolicyFile{headSHA: file(policyYAML)}
}

func checkStored(t *testing.T, w *world, got result.Result, stored bool) {
	t.Helper()
	if !stored {
		if len(w.store.saves) != 0 {
			t.Errorf("stored %d results, want none", len(w.store.saves))
		}
		return
	}
	if len(w.store.saves) != 1 {
		t.Fatalf("stored %d results, want 1", len(w.store.saves))
	}
	if s := w.store.saves[0]; s.tree != treeID || jsonOf(t, s.res) != jsonOf(t, got) {
		t.Errorf("stored tree %q and a result that differs from the output, want tree %s", s.tree, treeID)
	}
}

// The same tree, policy, engine and date give the same output; the date is
// an input, read from the clock, and it decides an exception.
func TestDateIsAnInput(t *testing.T) {
	run := func(today string) result.Result {
		w := newWorld()
		w.policies.local[changecheck.PolicyPath] = file(expiredYAML)
		w.policies.target[mainSHA] = file(expiredYAML)
		w.facts.head = tree(nil, utilCopy, tcCopy)
		w.facts.base = w.facts.head
		w.repo.added = nil
		w.today = today
		return w.check(t, changecheck.Options{}).Result
	}
	first, second := run("2026-11-20"), run("2026-11-20")
	if a, b := jsonOf(t, first), jsonOf(t, second); a != b {
		t.Errorf("two runs differ:\n%s\n%s", a, b)
	}
	if first.Date != "2026-11-20" || first.Status != result.StatusFail {
		t.Errorf("on 2026-11-20: date %s, status %s; want 2026-11-20, fail", first.Date, first.Status)
	}
	if before := run("2026-11-19"); before.Date != "2026-11-19" || before.Status != result.StatusPass {
		t.Errorf("on 2026-11-19: date %s, status %s; want 2026-11-19, pass", before.Date, before.Status)
	}
}

type stored struct{ last request.Last }

func (s stored) Last(context.Context) (request.Last, bool, error) { return s.last, true, nil }

type checkout struct{}

func (checkout) Checkout(context.Context) (request.Checkout, error) {
	return request.Checkout{Head: headSHA, Tree: treeID}, nil
}

// The stored result is what `archfit request` builds from.
func TestStoredResultBuildsTheRequest(t *testing.T) {
	w := newWorld()
	w.check(t, changecheck.Options{})
	s := w.store.saves[0]
	svc := request.Service{
		Last: stored{request.Last{Tree: s.tree, Check: &s.res}}, Checkout: checkout{}, Clock: w.service(t).Clock,
	}
	out, err := svc.Ask(context.Background(), request.Input{Reason: "netem needs the shared parser"})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	want := request.Request{
		Reason: "netem needs the shared parser",
		Errors: []request.Error{{
			Path: tcNetem, Line: 12, Rule: "graph", From: "pkg_tc", To: "pkg_util",
			Balance: &result.Balance{Value: 9, Min: 7},
		}},
		Head: headSHA, Base: baseSHA, PolicyDigest: w.policies.local[changecheck.PolicyPath].Digest,
	}
	if got, _ := json.Marshal(out.Request); string(got) != mustJSON(t, want) {
		t.Errorf("request = %s, want %s", got, mustJSON(t, want))
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUnprotectedPolicy(t *testing.T) {
	w := newWorld()
	w.protection.on = false
	if got := w.check(t, changecheck.Options{}).Result.Policy; got.Protected {
		t.Errorf("policy = %+v, want not protected", got)
	}
}

// An error of a port is an error of the check: no result, nothing stored. A
// target policy that cannot be read is never taken for a target with none,
// which would judge with a local policy that may be edited.
func TestPortErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := map[string]func(w *world){
		"WorkingTree": func(w *world) { w.repo.fail = fail{"WorkingTree": boom} },
		"Commit":      func(w *world) { w.repo.fail = fail{"Commit": boom} },
		"OriginHead":  func(w *world) { w.repo.fail = fail{"OriginHead": boom} },
		"MergeBase":   func(w *world) { w.repo.fail = fail{"MergeBase": boom} },
		"Parent":      func(w *world) { w.atTip(baseSHA); w.repo.fail = fail{"Parent": boom} },
		"Files":       func(w *world) { w.repo.fail = fail{"Files": boom} },
		"AddedLines":  func(w *world) { w.repo.fail = fail{"AddedLines": boom} },
		"Local":       func(w *world) { w.policies.fail = fail{"Local": boom} },
		"Target":      func(w *world) { w.policies.fail = fail{"Target": boom} },
		"Head":        func(w *world) { w.facts.fail = fail{"Head": boom} },
		"Base":        func(w *world) { w.facts.fail = fail{"Base": boom} },
		"Protected":   func(w *world) { w.protection.err = boom },
		"Save":        func(w *world) { w.store.err = boom },
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			w := newWorld()
			setup(w)
			_, err := w.service(t).Check(context.Background(), changecheck.Options{})
			if !errors.Is(err, boom) {
				t.Errorf("Check error = %v, want %v", err, boom)
			}
			if name != "Save" && len(w.store.saves) != 0 {
				t.Errorf("stored %d results after a port error", len(w.store.saves))
			}
		})
	}
}

// pushWith puts HEAD at the tip of origin/main after a commit that adds
// nothing, judged with policyText on today.
func (w *world) pushWith(policyText, today string, head evidence.CheckFacts) {
	w.atTip(baseSHA)
	w.policies.local[changecheck.PolicyPath] = file(policyText)
	w.policies.target[headSHA] = file(policyText)
	w.facts.head, w.facts.base = head, head
	w.repo.added = nil
	w.today = today
}

// unresolved is an import of a file the last commit did not touch, which the
// parent could not resolve either: it gives no reason for unknown.
var unresolved = evidence.UnknownItem{
	File: "web/app.ts", Line: 3, Specifier: "./x", Reason: "import ./x does not resolve", Cause: evidence.CauseImport,
	Fix: "add web/x.ts",
}

// TestRepositoryStateOnAPush: a run whose HEAD is the tip of the target
// branch carries the repository state of its head, in the JSON of the result
// golden of each state. The state does not change the status: drift that the
// last commit did not add passes, and an expired exception fails every run.
// A place the check could not read is listed beside a clean state.
func TestRepositoryStateOnAPush(t *testing.T) {
	copies := tree(nil, utilCopy, tcCopy)
	tests := []struct {
		name   string
		setup  func(w *world)
		status result.Status
	}{
		{name: "repository_clean", status: result.StatusPass, setup: func(w *world) {
			head := tree(nil, utilCopy, tcCopy)
			head.Unknown = []evidence.UnknownItem{unresolved}
			w.pushWith(expiredYAML, "2026-11-10", head)
		}},
		{name: "repository_drift", status: result.StatusPass, setup: func(w *world) {
			w.pushWith(policyYAML, "2026-11-10", tree([]evidence.ImportFact{tcToUtil}))
		}},
		{name: "repository_expired", status: result.StatusFail, setup: func(w *world) {
			w.pushWith(expiredYAML, "2026-11-20", copies)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld()
			tt.setup(w)
			got := w.check(t, changecheck.Options{}).Result
			if got.Status != tt.status {
				t.Errorf("status = %s, want %s", got.Status, tt.status)
			}
			data, err := os.ReadFile(filepath.Join("..", "..", "model", "result", "testdata", tt.name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var want result.Result
			if err := json.Unmarshal(data, &want); err != nil {
				t.Fatal(err)
			}
			if g, w := jsonOf(t, got), jsonOf(t, want); g != w {
				t.Errorf("JSON:\n--- got\n%s--- want\n%s", g, w)
			}
		})
	}
}

// A missing tool does not hide a failure, so a push that fails carries the
// state, and the tool it could not run is unread beside it.
func TestRepositoryStateListsAMissingTool(t *testing.T) {
	w := newWorld()
	head := tree(nil, utilCopy, tcCopy)
	head.Unknown = []evidence.UnknownItem{missingTool}
	w.pushWith(expiredYAML, "2026-11-20", head)
	got := w.check(t, changecheck.Options{}).Result
	r := got.Repository
	if got.Status != result.StatusFail || r == nil || r.State != result.RepositoryExpired {
		t.Fatalf("status %s, repository %+v; want fail and expired", got.Status, r)
	}
	want := []result.UnreadPlace{{Reason: missingTool.Reason, Fix: missingTool.Fix}}
	if !slices.Equal(r.Unread, want) {
		t.Errorf("unread = %+v, want %+v", r.Unread, want)
	}
}

var missingTool = evidence.UnknownItem{Reason: "ast-grep is not installed", Cause: evidence.CauseTool, Fix: "install ast-grep"}

// An exception is listed from 14 days before its date (2026-11-19), and not
// 15 days before.
func TestRepositoryStateExpiringWindow(t *testing.T) {
	for today, listed := range map[string]bool{"2026-11-05": true, "2026-11-04": false} {
		t.Run(today, func(t *testing.T) {
			w := newWorld()
			w.pushWith(expiredYAML, today, tree(nil, utilCopy, tcCopy))
			r := w.check(t, changecheck.Options{}).Result.Repository
			if r == nil {
				t.Fatal("no repository state on a push")
			}
			if got := len(r.Expiring) == 1; got != listed {
				t.Errorf("expiring = %+v, want listed: %t", r.Expiring, listed)
			}
		})
	}
}

// Only a run whose HEAD is the tip of the target branch and that gives a
// verdict carries a state. Every other run carries none, and none of them is
// an error: an absent state is no state, never clean.
func TestNoRepositoryState(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(w *world)
		opts   changecheck.Options
		status result.Status
	}{
		{name: "a pull-request run", setup: func(*world) {}, status: result.StatusFail},
		{
			// A later push moved the tip: the run of the older push is
			// behind it and never reports the state of an older tree.
			name: "a push a later push overtook", setup: func(w *world) { w.repo.mergeBases[mainSHA] = headSHA },
			status: result.StatusFail,
		},
		{
			name: "a preview on the tip", setup: func(w *world) {
				w.pushWith(policyYAML, "2026-11-10", tree([]evidence.ImportFact{tcToUtil}))
				w.policies.local[candidate] = file(policyYAML)
			},
			opts: changecheck.Options{Policy: candidate}, status: result.StatusPass,
		},
		{
			name: "a push with no verdict", setup: func(w *world) {
				head := tree([]evidence.ImportFact{tcToUtil})
				head.Unknown = []evidence.UnknownItem{missingTool}
				w.pushWith(policyYAML, "2026-11-10", head)
			},
			status: result.StatusUnknown,
		},
		{name: "a shallow push without the parent of HEAD", setup: func(w *world) { w.atTip("") }, status: result.StatusUnknown},
		{
			name: "a shallow push without the target branch", setup: func(w *world) {
				w.atTip(baseSHA)
				delete(w.repo.commits, mainRef)
			},
			status: result.StatusUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld()
			tt.setup(w)
			got := w.check(t, tt.opts).Result
			if got.Status != tt.status {
				t.Errorf("status = %s, want %s", got.Status, tt.status)
			}
			if got.Repository != nil {
				t.Errorf("repository = %+v, want none", got.Repository)
			}
			if strings.Contains(jsonOf(t, got), `"repository"`) {
				t.Errorf("the JSON has a repository key:\n%s", jsonOf(t, got))
			}
		})
	}
}
