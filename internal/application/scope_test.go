package application_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/application"
	"github.com/alexei-led/archfit/v3/internal/model/report"
)

// fakeGit is a tiny commit graph: refs name commits, parents are the first
// parent chain, and files hold policy bytes per commit.
type fakeGit struct {
	inRepo     bool
	refs       map[string]string
	parent     map[string]string
	originHead string
	files      map[string]map[string]string // commit -> path -> content
}

func (f fakeGit) InRepository(context.Context) bool { return f.inRepo }
func (f fakeGit) Lookup(_ context.Context, ref string) (string, bool, error) {
	sha, ok := f.refs[ref]
	return sha, ok, nil
}
func (f fakeGit) OriginHead(context.Context) (string, bool) { return f.originHead, f.originHead != "" }
func (f fakeGit) IsAncestor(_ context.Context, a, b string) (bool, error) {
	for c := b; c != ""; c = f.parent[c] {
		if c == a {
			return true, nil
		}
	}
	return false, nil
}
func (f fakeGit) MergeBase(_ context.Context, a, b string) (string, bool, error) {
	seen := map[string]bool{}
	for c := a; c != ""; c = f.parent[c] {
		seen[c] = true
	}
	for c := b; c != ""; c = f.parent[c] {
		if seen[c] {
			return c, true, nil
		}
	}
	return "", false, nil
}
func (f fakeGit) TargetBranchAt(_ context.Context, rev, path string) (string, error) {
	for _, line := range strings.Split(f.files[rev][path], "\n") {
		if name, ok := strings.CutPrefix(line, "target_branch: "); ok {
			return strings.TrimSpace(name), nil
		}
	}
	return "", nil
}
func (f fakeGit) FileAt(_ context.Context, rev, path string) ([]byte, bool, error) {
	content, ok := f.files[rev][path]
	return []byte(content), ok, nil
}

const (
	policyPath    = ".archfit.yaml"
	labelsPath    = ".archfit-labels.yaml"
	headRef       = "HEAD"
	branchMain    = "main"
	originDev     = "origin/dev"
	originDevelop = "origin/develop"
	originMain    = "origin/main"
	explicitSHA   = "abc"
)

// branchy: m1 <- m2 (origin/main = main = m2); feature head f2 <- f1 <- m1.
func branchy() fakeGit {
	return fakeGit{
		inRepo: true,
		refs:   map[string]string{headRef: "f2", originMain: "m2", branchMain: "m1", originDev: "m2"},
		parent: map[string]string{"m2": "m1", "f2": "f1", "f1": "m1"},
		files:  map[string]map[string]string{"m1": {policyPath: "rules: a"}, "m2": {policyPath: "rules: a"}},
	}
}

func TestResolveScope(t *testing.T) {
	t.Parallel()
	same := []application.PolicyFile{{Path: policyPath, Content: []byte("rules: a"), Exists: true}}
	for _, tc := range []struct {
		name string
		git  func() fakeGit
		req  application.ScopeRequest
		want application.ScopeResolution
	}{
		{name: "not a repository", git: func() fakeGit { return fakeGit{} },
			want: application.ScopeResolution{Mode: report.ScopeModeNone}},
		{name: "unborn HEAD is the whole tree", git: func() fakeGit { g := branchy(); delete(g.refs, headRef); return g },
			want: application.ScopeResolution{Mode: report.ScopeModeNone}},
		{name: "feature branch compares from the merge-base with origin/main", git: branchy, req: application.ScopeRequest{Policy: same},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originMain, TargetSHA: "m2", Base: "m1"}},
		{name: "--target wins and a bare name prefers the remote", git: branchy, req: application.ScopeRequest{Target: "dev", ConfigTarget: branchMain, Policy: same},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originDev, TargetSHA: "m2", Base: "m1"}},
		{name: "a working-tree target_branch does not beat origin/HEAD", git: func() fakeGit { g := branchy(); g.originHead = originDev; return g },
			req:  application.ScopeRequest{ConfigTarget: branchMain, Policy: same},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originDev, TargetSHA: "m2", Base: "m1"}},
		{name: "origin/HEAD names the target", git: func() fakeGit { g := branchy(); g.originHead = originDev; return g }, req: application.ScopeRequest{Policy: same},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originDev, TargetSHA: "m2", Base: "m1"}},
		{name: "on the target branch the whole tree is the change", git: func() fakeGit { g := branchy(); g.refs[headRef] = "m2"; return g }, req: application.ScopeRequest{Policy: same},
			want: application.ScopeResolution{Mode: report.ScopeModeTargetBranch, Target: originMain, TargetSHA: "m2"}},
		{name: "a named target that does not exist is unresolved, not a fallback", git: branchy, req: application.ScopeRequest{Target: "release"},
			want: application.ScopeResolution{Mode: report.ScopeModeNone,
				Unresolved: `target "release" (--target) names no commit here: fetch it or fix the name`}},
		{name: "no target at all", git: func() fakeGit { g := branchy(); g.refs = map[string]string{headRef: "f2"}; return g },
			want: application.ScopeResolution{Mode: report.ScopeModeNone,
				Unresolved: "no target branch: pass --target, set target_branch in the config, or set origin/HEAD (git remote set-head origin -a)"}},
		{name: "main and master both exist", git: func() fakeGit { g := branchy(); g.refs["master"] = "m1"; return g },
			want: application.ScopeResolution{Mode: report.ScopeModeNone,
				Unresolved: "both main and master exist and their configs name no common target_branch: pass --target or declare target_branch in both"}},
		{name: "a shallow clone has no merge-base", git: func() fakeGit { g := branchy(); g.parent = map[string]string{"f2": "f1"}; return g },
			want: application.ScopeResolution{Mode: report.ScopeModeNone, Target: originMain, TargetSHA: "m2",
				Unresolved: "HEAD and origin/main share no history in this clone: fetch it (fetch-depth: 0) or pass --base"}},
		{name: "explicit base is kept", git: branchy, req: application.ScopeRequest{Base: branchMain, Policy: same},
			want: application.ScopeResolution{Mode: report.ScopeModeExplicit, Target: originMain, TargetSHA: "m2", Base: branchMain}},
		{name: "explicit base with no target guards against the base commit", git: func() fakeGit {
			g := branchy()
			g.refs = map[string]string{headRef: "f2", originMain: "m2"}
			delete(g.refs, originMain)
			g.refs[explicitSHA] = "m2"
			return g
		},
			req:  application.ScopeRequest{Base: explicitSHA, Policy: same},
			want: application.ScopeResolution{Mode: report.ScopeModeExplicit, Target: explicitSHA, TargetSHA: "m2", Base: explicitSHA}},
		{name: "explicit base that names no commit", git: branchy, req: application.ScopeRequest{Base: "nope"},
			want: application.ScopeResolution{Mode: report.ScopeModeNone, Target: originMain, TargetSHA: "m2", Unresolved: `base "nope" names no commit here`}},
		{name: "a changed policy file is reported", git: branchy,
			req:  application.ScopeRequest{Policy: []application.PolicyFile{{Path: policyPath, Content: []byte("rules: b"), Exists: true}}},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originMain, TargetSHA: "m2", Base: "m1", PolicyChanged: []string{policyPath}}},
		{name: "the trusted default's config names the target; the branch's own target_branch is ignored",
			git: func() fakeGit {
				g := branchy()
				g.refs["origin/release/2.0"] = "m2"
				g.files["m2"] = map[string]string{policyPath: "target_branch: release/2.0\nrules: a"}
				return g
			},
			req:  application.ScopeRequest{ConfigTarget: "feature", Policy: []application.PolicyFile{{Path: policyPath, Content: []byte("target_branch: feature\nrules: a"), Exists: true}}},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: "origin/release/2.0", TargetSHA: "m2", Base: "m1", PolicyChanged: []string{policyPath}}},
		{name: "a branch that points target_branch at itself still meets the default branch's policy",
			git: func() fakeGit {
				g := branchy()
				g.refs["origin/feature"] = "f2"
				g.files["f2"] = map[string]string{policyPath: "target_branch: feature"}
				return g
			},
			req:  application.ScopeRequest{ConfigTarget: "feature", Policy: []application.PolicyFile{{Path: policyPath, Content: []byte("target_branch: feature"), Exists: true}}},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originMain, TargetSHA: "m2", Base: "m1", PolicyChanged: []string{policyPath}}},
		{name: "without a trusted default the working-tree target_branch is used",
			git: func() fakeGit {
				g := branchy()
				g.refs = map[string]string{"HEAD": "f2", originDevelop: "m2"}
				return g
			},
			req:  application.ScopeRequest{ConfigTarget: "develop", Policy: same},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originDevelop, TargetSHA: "m2", Base: "m1"}},
		{name: "a config the repository does not track may name the target",
			git:  func() fakeGit { g := branchy(); g.refs[originDevelop] = "m2"; return g },
			req:  application.ScopeRequest{ConfigTarget: "develop"},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originDevelop, TargetSHA: "m2", Base: "m1"}},
		{name: "main and master agree on a target their configs declare",
			git: func() fakeGit {
				g := branchy()
				g.refs["master"], g.refs["origin/master"] = "m1", "m2"
				g.files["m2"] = map[string]string{policyPath: "target_branch: dev"}
				g.files["m1"] = map[string]string{policyPath: "target_branch: dev"}
				return g
			},
			req:  application.ScopeRequest{ConfigTarget: "master", Policy: []application.PolicyFile{{Path: policyPath, Content: []byte("target_branch: dev"), Exists: true}}},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originDev, TargetSHA: "m2", Base: "m1"}},
		{name: "with main and master both present, a branch's own target_branch decides nothing",
			git: func() fakeGit {
				g := branchy()
				g.refs["master"], g.refs["origin/master"] = "m1", "m2"
				return g
			},
			req: application.ScopeRequest{ConfigTarget: "master", Policy: same},
			want: application.ScopeResolution{Mode: report.ScopeModeNone,
				Unresolved: "both main and master exist and their configs name no common target_branch: pass --target or declare target_branch in both"}},
		{name: "a slash-named target is looked up on the remote first",
			git: func() fakeGit {
				g := branchy()
				g.refs["origin/release/2.0"] = "m2"
				g.refs["release/2.0"] = "m1"
				return g
			},
			req:  application.ScopeRequest{Target: "release/2.0", Policy: same},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: "origin/release/2.0", TargetSHA: "m2", Base: "m1"}},
		{name: "a policy the target moved after the fork is stale, not changed", git: func() fakeGit { g := branchy(); g.files["m2"] = map[string]string{policyPath: "rules: c"}; return g },
			req:  application.ScopeRequest{Policy: same},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originMain, TargetSHA: "m2", Base: "m1", PolicyStale: []string{policyPath}}},
		{name: "an edit on top of a moved target is changed, not stale", git: func() fakeGit { g := branchy(); g.files["m2"] = map[string]string{policyPath: "rules: c"}; return g },
			req:  application.ScopeRequest{Policy: []application.PolicyFile{{Path: policyPath, Content: []byte("rules: b"), Exists: true}}},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originMain, TargetSHA: "m2", Base: "m1", PolicyChanged: []string{policyPath}}},
		{name: "a target with no config has no policy to protect", git: func() fakeGit { g := branchy(); g.files = nil; return g },
			req:  application.ScopeRequest{Policy: same},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originMain, TargetSHA: "m2", Base: "m1"}},
		{name: "a deleted policy file is a change", git: branchy,
			req:  application.ScopeRequest{Policy: []application.PolicyFile{{Path: policyPath}}},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originMain, TargetSHA: "m2", Base: "m1", PolicyChanged: []string{policyPath}}},
		{name: "a file absent on both sides is not a change", git: branchy,
			req:  application.ScopeRequest{Policy: []application.PolicyFile{{Path: policyPath, Content: []byte("rules: a"), Exists: true}, {Path: labelsPath}}},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originMain, TargetSHA: "m2", Base: "m1"}},
		{name: "a new policy file is a change", git: branchy,
			req:  application.ScopeRequest{Policy: []application.PolicyFile{{Path: policyPath, Content: []byte("rules: a"), Exists: true}, {Path: labelsPath, Content: []byte("x"), Exists: true}}},
			want: application.ScopeResolution{Mode: report.ScopeModeMergeBase, Target: originMain, TargetSHA: "m2", Base: "m1", PolicyChanged: []string{labelsPath}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := application.ResolveScope(context.Background(), tc.git(), tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if got.Mode != tc.want.Mode || got.Target != tc.want.Target || got.TargetSHA != tc.want.TargetSHA ||
				got.Base != tc.want.Base || got.Unresolved != tc.want.Unresolved || !slices.Equal(got.PolicyChanged, tc.want.PolicyChanged) || !slices.Equal(got.PolicyStale, tc.want.PolicyStale) {
				t.Errorf("scope = %+v\nwant   %+v", got, tc.want)
			}
		})
	}
}
