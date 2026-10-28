package application

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

// BaseAuto asks ResolveScope to choose the base: the merge-base of HEAD and the
// target branch.
const BaseAuto = "auto"

// Branch names tried, in this order of preference, when nothing names the
// target. Exactly one of them must exist.
var conventionalTargets = []string{"main", "master"}

// remoteName is the remote a bare target name is looked up on first: its
// tip is the trusted one, a local branch of the same name may be stale.
const remoteName = "origin"

// ScopeGit is the git evidence scope resolution needs. The history adapter
// implements it; the use case never runs a process.
type ScopeGit interface {
	InRepository(ctx context.Context) bool
	// Lookup resolves ref to a commit SHA; ok is false when it names none.
	Lookup(ctx context.Context, ref string) (sha string, ok bool, err error)
	// OriginHead is the remote's default branch as a remote-tracking ref.
	OriginHead(ctx context.Context) (ref string, ok bool)
	// MergeBase is the common ancestor of a and b; ok is false when the clone
	// holds none.
	MergeBase(ctx context.Context, a, b string) (sha string, ok bool, err error)
	// IsAncestor reports whether a is an ancestor of b, or equal to it.
	IsAncestor(ctx context.Context, a, b string) (bool, error)
	// FileAt reads a repository-relative path at rev as a git object.
	FileAt(ctx context.Context, rev, path string) (content []byte, exists bool, err error)
	// TargetBranchAt reads target_branch from the config at path as it is at
	// rev. It is empty when the file or the key is absent.
	TargetBranchAt(ctx context.Context, rev, path string) (string, error)
}

// PolicyFile is one protected policy file as the working tree holds it.
type PolicyFile struct {
	// Path is relative to the repository root, with forward slashes.
	Path string
	// Content is the working-tree bytes; Exists is false when there is no file.
	Content []byte
	Exists  bool
}

// ScopeRequest is what the caller said about the scope.
type ScopeRequest struct {
	// Target is --target.
	Target string
	// ConfigTarget is target_branch from the working-tree config. It is used
	// only when the repository has no trusted default branch.
	ConfigTarget string
	// Base is --base: empty or BaseAuto to resolve it, any other value is a ref.
	Base string
	// Policy lists the protected files to compare with the target tip.
	Policy []PolicyFile
}

// ScopeResolution is how the change and its policy were resolved.
type ScopeResolution struct {
	Mode       string
	Target     string
	TargetSHA  string
	Base       string
	Unresolved string
	// PolicyChanged are the protected files the branch edited that now differ
	// from the target tip: the branch cannot grade itself under them.
	PolicyChanged []string
	// PolicyStale are the protected files the branch did not touch that differ
	// from the target tip because the target changed them after the fork.
	PolicyStale []string
}

// ResolveScope decides what the change is and whether its policy files differ
// from the target tip. It never falls back to HEAD silently: a target or a
// base it cannot resolve is reported in Unresolved, and the caller must not
// call the change a pass.
func ResolveScope(ctx context.Context, g ScopeGit, req ScopeRequest) (ScopeResolution, error) {
	none := ScopeResolution{Mode: report.ScopeModeNone}
	if !g.InRepository(ctx) {
		return none, nil
	}
	head, ok, err := g.Lookup(ctx, "HEAD")
	if err != nil {
		return none, err
	}
	if !ok {
		// Before the first commit HEAD names nothing: the whole tree is the change.
		return none, nil
	}
	explicit := req.Base != "" && req.Base != BaseAuto
	target, targetSHA, why, err := resolveTarget(ctx, g, req)
	if err != nil {
		return none, err
	}
	res := ScopeResolution{Mode: report.ScopeModeNone, Target: target, TargetSHA: targetSHA}

	if explicit {
		res.Mode = report.ScopeModeExplicit
		baseSHA, found, err := g.Lookup(ctx, req.Base)
		if err != nil {
			return none, err
		}
		if !found {
			res.Mode = report.ScopeModeNone
			res.Unresolved = fmt.Sprintf("base %q names no commit here", req.Base)
			return res, nil
		}
		res.Base = req.Base
		if why != "" {
			// CI passes the target tip as the base. Without a named target the
			// policy guard compares with the base commit, which is the same tip.
			res.Target, res.TargetSHA = req.Base, baseSHA
		}
		fork, err := forkPoint(ctx, g, head, res.TargetSHA)
		if err != nil {
			return none, err
		}
		return withPolicyGuard(ctx, g, req, res, fork)
	}

	if why != "" {
		res.Unresolved = why
		return res, nil
	}
	onTarget, err := g.IsAncestor(ctx, head, targetSHA)
	if err != nil {
		return none, err
	}
	if onTarget {
		res.Mode = report.ScopeModeTargetBranch
		return withPolicyGuard(ctx, g, req, res, head)
	}
	mergeBase, found, err := g.MergeBase(ctx, head, targetSHA)
	if err != nil {
		return none, err
	}
	if !found {
		res.Unresolved = fmt.Sprintf("HEAD and %s share no history in this clone: fetch it (fetch-depth: 0) or pass --base", target)
		return res, nil
	}
	res.Mode, res.Base = report.ScopeModeMergeBase, mergeBase
	return withPolicyGuard(ctx, g, req, res, mergeBase)
}

// forkPoint is the commit the branch left the target at: HEAD itself when it
// is on the target, otherwise the merge-base. Without a common ancestor the
// last commit is the best reference a policy edit can be measured against.
func forkPoint(ctx context.Context, g ScopeGit, head, targetSHA string) (string, error) {
	if targetSHA == "" {
		return head, nil
	}
	onTarget, err := g.IsAncestor(ctx, head, targetSHA)
	if err != nil || onTarget {
		return head, err
	}
	mergeBase, found, err := g.MergeBase(ctx, head, targetSHA)
	if err != nil || !found {
		return head, err
	}
	return mergeBase, nil
}

// withPolicyGuard sorts the protected files that differ from the target tip,
// a missing file counting as a difference. A file the branch edited (it differs
// from the fork point too) is PolicyChanged: the branch cannot grade itself
// under rules it wrote. A file the branch left alone is PolicyStale: the target
// moved it after the fork, and the branch only needs to catch up.
//
// The first entry of req.Policy is the config. A target that holds no config
// has no policy to protect, so nothing is compared: a repository that keeps its
// policy untracked is not flagged on every run.
func withPolicyGuard(ctx context.Context, g ScopeGit, req ScopeRequest, res ScopeResolution, fork string) (ScopeResolution, error) {
	if len(req.Policy) > 0 {
		if _, exists, err := g.FileAt(ctx, res.TargetSHA, req.Policy[0].Path); err != nil || !exists {
			return res, err
		}
	}
	for _, file := range req.Policy {
		theirs, exists, err := g.FileAt(ctx, res.TargetSHA, file.Path)
		if err != nil {
			return res, err
		}
		if exists == file.Exists && (!exists || bytes.Equal(theirs, file.Content)) {
			continue
		}
		forked, existedAtFork, err := g.FileAt(ctx, fork, file.Path)
		if err != nil {
			return res, err
		}
		edited := existedAtFork != file.Exists || (file.Exists && !bytes.Equal(forked, file.Content))
		if edited {
			res.PolicyChanged = append(res.PolicyChanged, file.Path)
		} else {
			res.PolicyStale = append(res.PolicyStale, file.Path)
		}
	}
	return res, nil
}

// resolveTarget decides which branch the change lands on, in this order:
// --target; then, for a config the repository does not track, its
// target_branch; then the trusted default branch (origin/HEAD, else main or
// master when exactly one exists) together with the target_branch the config
// at that tip declares; and only when the repository has no default branch at
// all, the target_branch of the working-tree config.
//
// The working-tree config is the branch's own copy. Whenever a trusted
// reference exists its copy of the config decides, so a branch cannot point the
// guard at itself by editing target_branch; the edit shows up as a policy
// change instead. If both main and master exist, their configs must agree on a
// target, or the scope stays unresolved. A repo with no default branch has
// nothing to check the edit against, so there the working-tree value is used
// and the tamper check does not hold.
//
// A name that was given and resolves to nothing is an answer ("why"), never a
// reason to try the next source.
func resolveTarget(ctx context.Context, g ScopeGit, req ScopeRequest) (ref, sha, why string, err error) {
	if req.Target != "" {
		return namedTarget(ctx, g, req.Target, "--target")
	}
	if len(req.Policy) == 0 && req.ConfigTarget != "" {
		// A config outside the repository is not the branch's to edit.
		return namedTarget(ctx, g, req.ConfigTarget, "target_branch in the config")
	}
	def, err := trustedDefault(ctx, g)
	if err != nil {
		return "", "", "", err
	}
	switch {
	case len(def.ambiguous) > 0:
		return declaredByAll(ctx, g, def, req)
	case def.why != "":
		if req.ConfigTarget != "" {
			return namedTarget(ctx, g, req.ConfigTarget, "target_branch in the config")
		}
		return "", "", def.why, nil
	}
	if len(req.Policy) > 0 {
		declared, err := g.TargetBranchAt(ctx, def.sha, req.Policy[0].Path)
		if err != nil {
			return "", "", "", err
		}
		if declared != "" {
			return namedTarget(ctx, g, declared, "target_branch in the config at "+def.ref)
		}
	}
	return def.ref, def.sha, "", nil
}

// declaredByAll resolves the target when main and master both exist: each
// one's config must declare the same target_branch. The working tree is not
// asked.
func declaredByAll(ctx context.Context, g ScopeGit, def defaultBranch, req ScopeRequest) (ref, sha, why string, err error) {
	var agreed string
	for i, candidate := range def.ambiguous {
		declared := ""
		if len(req.Policy) > 0 {
			if declared, err = g.TargetBranchAt(ctx, candidate.sha, req.Policy[0].Path); err != nil {
				return "", "", "", err
			}
		}
		if declared == "" || (i > 0 && declared != agreed) {
			return "", "", def.why, nil
		}
		agreed = declared
	}
	return namedTarget(ctx, g, agreed, "target_branch declared by both default branches")
}

// namedTarget resolves a branch name someone gave, saying where it came from
// when it names nothing.
func namedTarget(ctx context.Context, g ScopeGit, name, source string) (ref, sha, why string, err error) {
	ref, sha, found, err := lookupBranch(ctx, g, name)
	if err != nil {
		return "", "", "", err
	}
	if !found {
		return "", "", fmt.Sprintf("target %q (%s) names no commit here: fetch it or fix the name", name, source), nil
	}
	return ref, sha, "", nil
}

// branchRef is a resolved branch.
type branchRef struct{ ref, sha string }

// defaultBranch is what trustedDefault found: one branch, a reason it found
// none, or the several conventional branches that exist and so are ambiguous.
type defaultBranch struct {
	branchRef
	why       string
	ambiguous []branchRef
}

// trustedDefault is the branch changes land on when nobody names one: the
// remote's default, else main or master when exactly one exists.
func trustedDefault(ctx context.Context, g ScopeGit) (defaultBranch, error) {
	if head, ok := g.OriginHead(ctx); ok {
		sha, found, err := g.Lookup(ctx, head)
		if err != nil {
			return defaultBranch{}, err
		}
		if found {
			return defaultBranch{branchRef: branchRef{head, sha}}, nil
		}
	}
	var present []branchRef
	var names []string
	for _, name := range conventionalTargets {
		ref, sha, found, err := lookupBranch(ctx, g, name)
		if err != nil {
			return defaultBranch{}, err
		}
		if found {
			present = append(present, branchRef{ref, sha})
			names = append(names, name)
		}
	}
	switch len(present) {
	case 1:
		return defaultBranch{branchRef: present[0]}, nil
	case 0:
		return defaultBranch{why: "no target branch: pass --target, set target_branch in the config, or set origin/HEAD (git remote set-head origin -a)"}, nil
	default:
		return defaultBranch{
			why:       fmt.Sprintf("both %s exist and their configs name no common target_branch: pass --target or declare target_branch in both", strings.Join(names, " and ")),
			ambiguous: present,
		}, nil
	}
}

// lookupBranch resolves a branch name. A bare name is tried on the remote
// first, because the remote tip is the trusted one and a local branch of the
// same name may be stale.
func lookupBranch(ctx context.Context, g ScopeGit, name string) (ref, sha string, found bool, err error) {
	candidates := []string{name}
	if !strings.HasPrefix(name, remoteName+"/") && !strings.HasPrefix(name, "refs/") {
		candidates = []string{remoteName + "/" + name, name}
	}
	for _, candidate := range candidates {
		sha, found, err := g.Lookup(ctx, candidate)
		if err != nil || found {
			return candidate, sha, found, err
		}
	}
	return "", "", false, nil
}
