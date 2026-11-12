// Package changecheck runs `archfit check` (design 7.1, 7.2, 9.1, 11.1): the
// status of the change in the working tree, against the merge base with its
// target branch.
//
// # The run
//
//  1. The target branch: Options.Base, else origin/HEAD. With neither, the
//     result is unknown and says to pass --base.
//  2. The merge base of HEAD and the target. When HEAD is on the target branch
//     (the merge base is HEAD itself), the base is HEAD's first parent. No merge
//     base, as in a shallow clone, is unknown and names the fetch.
//  3. The policy (design 9.1). The policy on the target branch is the approved
//     one: when the local file differs from it, the result is unknown, names
//     the command that restores the file, and no facts are read. A target with
//     no policy file, or with a version 2 file (the conversion pull request of
//     design 14), is not compared: the check judges with the local policy and
//     adds one note. A target file that is not a valid policy is unknown. With
//     Options.Policy the check judges with that file and compares nothing: a
//     preview, which is not stored.
//  4. The policy against the tree: resolved.Policy.CheckTree on the head's
//     source files. A path glob that matches nothing is unknown.
//  5. The facts of the head and the base, the lines the change adds, and
//     count.Assess on the date the Clock gives.
//  6. The result (project.go) and, unless the run is a preview, the store of
//     the last result for `archfit request`. Every other run is stored, pass
//     and unknown too.
//
// The same tree, policy, engine version and date give the same result. An
// error of a port (git cannot run, a cancelled context) comes back as an
// error with no result; whatever the record can name is an unknown result.
//
// The package imports no adapter: cmd/archfit wires the ports.
package changecheck

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/assessment/count"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// PolicyPath is the policy file, relative to the repository root.
const PolicyPath = ".archfit.yaml"

// PolicyFile is one policy file, read and resolved.
type PolicyFile struct {
	// Found is false when there is no file.
	Found bool
	// Digest is resolved.Digest of the file's bytes, set whenever Found.
	Digest string
	// Policy is the policy with its defaults, set when Found and Err is nil.
	Policy resolved.Policy
	// Err says why a file that was found is not a valid version 3 policy.
	Err error
	// Version2 is true when the file is a version 2 policy, the Archfit 3
	// format (policyfile.ErrVersion2). Err is set too.
	Version2 bool
}

// Policies reads policy files. An error is a failure to read, such as git
// failing; a file that is absent or not valid is a PolicyFile.
type Policies interface {
	// Local reads the policy file at path: PolicyPath at the repository root,
	// or the file of --policy.
	Local(ctx context.Context, path string) (PolicyFile, error)
	// Target reads PolicyPath at commit, from git objects.
	Target(ctx context.Context, commit string) (PolicyFile, error)
}

// Repository reads the git history and the working tree of the checkout.
type Repository interface {
	// OriginHead returns the remote's default branch as a ref such as
	// origin/main; ok is false when the clone recorded none.
	OriginHead(ctx context.Context) (ref string, ok bool, err error)
	// Commit resolves a ref such as HEAD or origin/main to a commit; ok is
	// false when it names no commit here.
	Commit(ctx context.Context, ref string) (sha string, ok bool, err error)
	// Parent returns the first parent of a commit; ok is false for a root
	// commit, or a parent this clone does not hold.
	Parent(ctx context.Context, commit string) (sha string, ok bool, err error)
	// MergeBase returns the best common ancestor of two commits; ok is false
	// when they share no history this clone holds.
	MergeBase(ctx context.Context, a, b string) (sha string, ok bool, err error)
	// AddedLines returns the lines the working tree adds against the commit
	// base, per repository-relative file.
	AddedLines(ctx context.Context, base string) (evidence.AddedLines, error)
	// WorkingTree returns the git tree id of the working tree as a commit of
	// every change would hold it (internal/history/git.WorkingTreeID).
	WorkingTree(ctx context.Context) (string, error)
}

// Facts reads the check facts of a tree.
type Facts interface {
	// Head returns the facts of the working tree.
	Head(ctx context.Context) (evidence.CheckFacts, error)
	// Base returns the facts of the tree of commit. A base that cannot be
	// read is an unknown item of cause base, not an error.
	Base(ctx context.Context, commit string) (evidence.CheckFacts, error)
}

// Protection says whether the policy is protected. Milestone 1 (D6): true
// when a workflow in .github/workflows/ uses alexei-led/archfit-action.
type Protection interface {
	Protected(ctx context.Context) (bool, error)
}

// Store keeps the last result for `archfit request`: the file
// .archfit-cache/last.json, {"tree": tree, "output": the result as
// --format json writes it}, which internal/application/request reads.
type Store interface {
	Save(ctx context.Context, tree string, r result.Result) error
}

// Clock gives the date the exceptions are judged on.
type Clock interface {
	Now() time.Time
}

// Service is the `archfit check` use case. Every port is required.
type Service struct {
	Policies   Policies
	Repository Repository
	Facts      Facts
	Protection Protection
	Store      Store
	Clock      Clock
	// Engine is the version of the archfit build.
	Engine string
}

// Options are the flags of `archfit check`.
type Options struct {
	// Base is --base: the target branch. Empty: origin/HEAD.
	Base string
	// Policy is --policy: a candidate policy file to judge with, without the
	// comparison. Empty: PolicyPath.
	Policy string
}

// Outcome is the result of one check.
type Outcome struct {
	Result result.Result
	// Notes are lines for stderr: progress the record does not hold, such
	// as a target branch with no version 3 policy.
	Notes []string
}

// The texts of the run's own unknown reasons and notes.
const (
	whyCompare   = "the check compares the change with the merge base of its target branch"
	whyDiffers   = "only owners change the policy"
	whyInvalid   = "the check cannot judge with a policy that is not valid"
	whyTree      = "the policy does not fit the files of the tree"
	whyTargetBad = "the policy on %s is not valid"

	whatNoTarget   = "no target branch: origin/HEAD is not set"
	whatNoCommit   = "%s names no commit in this clone"
	whatNoHead     = "HEAD names no commit"
	whatNoBase     = "HEAD and %s have no merge base in this clone"
	whatNoParent   = "HEAD is on %s and its parent is not in this clone"
	whatDiffers    = "differs from %s"
	whatNoPolicy   = "no policy file"
	nextBaseFlag   = "pass --base <target branch>"
	nextFetch      = "git fetch"
	nextUnshallow  = "git fetch --unshallow"
	nextCommit     = "make a first commit"
	nextRestore    = "git checkout %s -- " + PolicyPath
	nextInit       = "archfit init"
	nextPolicyFlag = "pass --policy <policy file>"
	nextFix        = "fix %s"
	nextAskOwner   = "ask an owner to fix " + PolicyPath + " on %s"

	noteNoPolicy = "%s has no " + PolicyPath + ": judged with the local policy"
	noteVersion2 = "%s has a version 2 " + PolicyPath + ": judged with the local policy until its conversion pull request merges"
)

// run is what one check found, before projection (project.go).
type run struct {
	// policyPath is the file that judged: PolicyPath, or --policy.
	policyPath string
	// digest is the digest of that file.
	digest string
	// target is the ref the policy was compared with; empty when no
	// comparison was made.
	target  string
	differs bool
	base    string
	head    string
	// reasons are the run's own reasons for an unknown status. A run with
	// reasons stopped before the assessment.
	reasons []reason
	// verdict is the assessment; nil when the run stopped before it.
	verdict *count.Verdict
	notes   []string
}

// reason is one reason the run gives no verdict.
type reason struct {
	path      string
	cause     result.Cause
	what, why string
	kind      result.NextKind
	next      string
}

// Check gives the change its result. See the package doc for the run.
func (s Service) Check(ctx context.Context, o Options) (Outcome, error) {
	now := s.Clock.Now()
	tree := ""
	if o.Policy == "" {
		// The tree id comes first, so the stored result names the tree the
		// facts are read from.
		var err error
		if tree, err = s.Repository.WorkingTree(ctx); err != nil {
			return Outcome{}, fmt.Errorf("read the working tree: %w", err)
		}
	}
	r, err := s.judge(ctx, o, now)
	if err != nil {
		return Outcome{}, err
	}
	protected, err := s.Protection.Protected(ctx)
	if err != nil {
		return Outcome{}, fmt.Errorf("read the workflows: %w", err)
	}
	res := project(r, protected, now, s.Engine)
	if o.Policy == "" {
		if err := s.Store.Save(ctx, tree, res); err != nil {
			return Outcome{}, fmt.Errorf("store the result: %w", err)
		}
	}
	return Outcome{Result: res, Notes: r.notes}, nil
}

// judge runs steps 1 to 5 of the package doc.
func (s Service) judge(ctx context.Context, o Options, now time.Time) (run, error) {
	r := run{policyPath: PolicyPath}
	if o.Policy != "" {
		r.policyPath = o.Policy
	}
	head, ok, err := s.Repository.Commit(ctx, "HEAD")
	if err != nil {
		return run{}, err
	}
	if ok {
		r.head = head
	}
	local, err := s.Policies.Local(ctx, r.policyPath)
	if err != nil {
		return run{}, fmt.Errorf("read %s: %w", r.policyPath, err)
	}
	r.digest = local.Digest
	if !local.Found {
		r.digest = resolved.Digest(nil)
	}
	ref, target, stopped, err := s.target(ctx, o, &r)
	if err != nil || stopped {
		return r, err
	}
	compared := false
	if o.Policy == "" {
		if compared, err = s.compare(ctx, &r, ref, target); err != nil || len(r.reasons) > 0 {
			return r, err
		}
	}
	switch {
	case !local.Found:
		missing := reason{path: r.policyPath, cause: result.CausePolicy, what: whatNoPolicy, kind: result.NextCommand, next: nextInit}
		if o.Policy != "" {
			missing.kind, missing.next = result.NextRestore, nextPolicyFlag
		}
		r.stop(missing)
		return r, nil
	case local.Err != nil:
		r.stopLines(local.Err, whyInvalid, result.NextRestore, fmt.Sprintf(nextFix, r.policyPath))
		return r, nil
	}
	headFacts, err := s.Facts.Head(ctx)
	if err != nil {
		return run{}, fmt.Errorf("read the head: %w", err)
	}
	if err := local.Policy.CheckTree(headFacts.Sources); err != nil {
		next := fmt.Sprintf(nextFix, r.policyPath)
		if compared {
			next = fmt.Sprintf(nextAskOwner, ref)
		}
		r.stopLines(err, whyTree, result.NextRestore, next)
		return r, nil
	}
	baseFacts, err := s.Facts.Base(ctx, r.base)
	if err != nil {
		return run{}, fmt.Errorf("read the base: %w", err)
	}
	added, err := s.Repository.AddedLines(ctx, r.base)
	if err != nil {
		return run{}, fmt.Errorf("read the added lines: %w", err)
	}
	v := count.Assess(count.Input{Policy: local.Policy, Base: baseFacts, Head: headFacts, Added: added, Today: now})
	r.verdict = &v
	return r, nil
}

// target finds the target branch, its commit and the base (steps 1 and 2).
// stopped is true when it could not, with the reason in r.
func (s Service) target(ctx context.Context, o Options, r *run) (ref, commit string, stopped bool, err error) {
	baseStop := func(what, next string) (string, string, bool, error) {
		r.stop(reason{cause: result.CauseBase, what: what, why: whyCompare, kind: result.NextRestore, next: next})
		return "", "", true, nil
	}
	ref = o.Base
	if ref == "" {
		var ok bool
		if ref, ok, err = s.Repository.OriginHead(ctx); err != nil {
			return "", "", false, err
		} else if !ok {
			return baseStop(whatNoTarget, nextBaseFlag)
		}
	}
	commit, ok, err := s.Repository.Commit(ctx, ref)
	if err != nil {
		return "", "", false, err
	}
	if !ok {
		return baseStop(fmt.Sprintf(whatNoCommit, ref), nextFetch)
	}
	if r.head == "" {
		return baseStop(whatNoHead, nextCommit)
	}
	base, ok, err := s.Repository.MergeBase(ctx, r.head, commit)
	if err != nil {
		return "", "", false, err
	}
	if !ok {
		return baseStop(fmt.Sprintf(whatNoBase, ref), nextUnshallow)
	}
	if base == r.head {
		// HEAD is on the target branch: the change is what HEAD adds to its
		// first parent.
		if base, ok, err = s.Repository.Parent(ctx, r.head); err != nil {
			return "", "", false, err
		} else if !ok {
			return baseStop(fmt.Sprintf(whatNoParent, ref), nextUnshallow)
		}
	}
	r.base = base
	return ref, commit, false, nil
}

// compare compares the local policy with the target's (step 3). compared is
// true when the local policy is the target's; a stop leaves its reason in r.
func (s Service) compare(ctx context.Context, r *run, ref, commit string) (compared bool, err error) {
	t, err := s.Policies.Target(ctx, commit)
	if err != nil {
		return false, fmt.Errorf("read %s on %s: %w", PolicyPath, ref, err)
	}
	switch {
	case !t.Found:
		r.notes = append(r.notes, fmt.Sprintf(noteNoPolicy, ref))
		return false, nil
	case t.Version2:
		r.notes = append(r.notes, fmt.Sprintf(noteVersion2, ref))
		return false, nil
	}
	r.target, r.differs = ref, t.Digest != r.digest
	switch {
	case t.Err != nil:
		r.stopLines(t.Err, fmt.Sprintf(whyTargetBad, ref), result.NextRestore, fmt.Sprintf(nextAskOwner, ref))
	case r.differs:
		r.stop(reason{
			path: PolicyPath, cause: result.CausePolicy, what: fmt.Sprintf(whatDiffers, ref), why: whyDiffers,
			kind: result.NextRestore, next: fmt.Sprintf(nextRestore, ref),
		})
	}
	return true, nil
}

func (r *run) stop(why reason) {
	r.reasons = append(r.reasons, why)
}

// stopLines stops with one reason per line of a policy error: the policy
// checks list every message, one per line.
func (r *run) stopLines(err error, why string, kind result.NextKind, next string) {
	for line := range strings.SplitSeq(err.Error(), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			r.stop(reason{path: r.policyPath, cause: result.CausePolicy, what: line, why: why, kind: kind, next: next})
		}
	}
}
