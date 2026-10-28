package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexei-led/archfit/v3/internal/application"
	historygit "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// scopeGit adapts the git history package to the scope resolution port, for
// one directory.
type scopeGit struct {
	runner toolrun.Runner
	dir    string
}

var _ application.ScopeGit = scopeGit{}

func (g scopeGit) InRepository(ctx context.Context) bool {
	return historygit.InRepository(ctx, g.dir, g.runner)
}

func (g scopeGit) Lookup(ctx context.Context, ref string) (string, bool, error) {
	return historygit.Lookup(ctx, g.dir, ref, g.runner)
}

func (g scopeGit) OriginHead(ctx context.Context) (string, bool) {
	return historygit.OriginHead(ctx, g.dir, g.runner)
}

func (g scopeGit) MergeBase(ctx context.Context, a, b string) (string, bool, error) {
	return historygit.MergeBase(ctx, g.dir, a, b, g.runner)
}

func (g scopeGit) IsAncestor(ctx context.Context, a, b string) (bool, error) {
	return historygit.IsAncestor(ctx, g.dir, a, b, g.runner)
}

func (g scopeGit) FileAt(ctx context.Context, rev, path string) ([]byte, bool, error) {
	return historygit.FileAt(ctx, g.dir, rev, path, g.runner)
}

// policyFileNames are the protected policy files of a config bundle, in the
// order a result names them: the config, the baseline, the approved labels.
func policyFileNames(configPath string) []string {
	return []string{filepath.Base(configPath), defaultBaselinePath, defaultLabelsPath}
}

// policyFiles lists the bundle's policy files as the working tree holds them,
// with paths relative to the repository root. A bundle outside the repository
// has no target tip to compare with: it yields no files.
func policyFiles(ctx context.Context, g scopeGit, configPath string) ([]application.PolicyFile, error) {
	if !g.InRepository(ctx) {
		return nil, nil
	}
	gitRoot, err := historygit.RepoRoot(ctx, g.dir, g.runner)
	if err != nil {
		return nil, err
	}
	bundleDir, err := filepath.Abs(filepath.Dir(configPath))
	if err != nil {
		return nil, err
	}
	rel, err := pathInRepo(gitRoot, filepath.Join(bundleDir, filepath.Base(configPath)))
	if err != nil {
		return nil, nil //nolint:nilerr // an external policy directory is not tracked by this repository
	}
	relDir := filepath.Dir(rel)
	var out []application.PolicyFile
	for _, name := range policyFileNames(configPath) {
		content, readErr := os.ReadFile(filepath.Join(bundleDir, name)) //nolint:gosec // bundle paths come from --config
		out = append(out, application.PolicyFile{
			Path:    filepath.ToSlash(filepath.Join(relDir, name)),
			Content: content,
			Exists:  readErr == nil,
		})
	}
	return out, nil
}

// resolveCheckScope decides what a check compares with, from the flags and the
// config's target_branch. It sets req.baseRef to the resolved base, so every
// later stage sees one answer.
func resolveCheckScope(ctx context.Context, deps *appDeps, req *scanRequest, targetBranch string) (application.ScopeResolution, error) {
	dir := req.root
	if dir == "" {
		dir = filepath.Dir(req.configPath)
	}
	git := scopeGit{runner: deps.Runner, dir: dir}
	files, err := policyFiles(ctx, scopeGit{runner: deps.Runner, dir: filepath.Dir(req.configPath)}, req.configPath)
	if err != nil {
		return application.ScopeResolution{}, fmt.Errorf("read the policy files: %w", err)
	}
	res, err := application.ResolveScope(ctx, git, application.ScopeRequest{
		Target: req.target, ConfigTarget: targetBranch, Base: req.baseRef, Policy: files,
	})
	if err != nil {
		return res, fmt.Errorf("resolve the scope: %w", err)
	}
	req.baseRef = res.Base
	return res, nil
}

// materializePolicyAtTarget writes the target tip's policy files to a fresh
// directory, read as git objects, and returns it. ok is false when the target
// has no config to judge with.
func materializePolicyAtTarget(ctx context.Context, deps *appDeps, req scanRequest, res application.ScopeResolution) (dir string, ok bool, err error) {
	files, err := policyFiles(ctx, scopeGit{runner: deps.Runner, dir: filepath.Dir(req.configPath)}, req.configPath)
	if err != nil || len(files) == 0 {
		return "", false, err
	}
	g := scopeGit{runner: deps.Runner, dir: filepath.Dir(req.configPath)}
	tmp, err := os.MkdirTemp("", "archfit-target-policy-")
	if err != nil {
		return "", false, err
	}
	names := policyFileNames(req.configPath)
	haveConfig := false
	for i, file := range files {
		content, exists, err := g.FileAt(ctx, res.TargetSHA, file.Path)
		if err != nil {
			_ = os.RemoveAll(tmp)
			return "", false, err
		}
		if !exists {
			continue
		}
		if i == 0 {
			haveConfig = true
		}
		if err := os.WriteFile(filepath.Join(tmp, names[i]), content, 0o600); err != nil {
			_ = os.RemoveAll(tmp)
			return "", false, err
		}
	}
	if !haveConfig {
		_ = os.RemoveAll(tmp)
		return "", false, nil
	}
	return tmp, true, nil
}

// targetPolicyRequest re-aims a scan at the policy files in dir while keeping
// the tree: the same root and base, measured under the target's rules.
func targetPolicyRequest(req scanRequest, dir string) (scanRequest, error) {
	root := req.root
	if root == "" {
		root = filepath.Dir(req.configPath)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return req, err
	}
	treq := req
	treq.configPath = filepath.Join(dir, filepath.Base(req.configPath))
	treq.bundleDir = dir
	treq.root = root
	// The repair command names the real config, not the temporary copy.
	treq.validationConfig = req.configPath
	treq.progress, treq.quiet = progressNone, true
	return treq, nil
}
