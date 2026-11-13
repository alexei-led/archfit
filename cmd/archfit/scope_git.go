package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"

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

func (g scopeGit) TargetBranchAt(ctx context.Context, rev, path string) (string, error) {
	content, exists, err := g.FileAt(ctx, rev, path)
	if err != nil || !exists {
		return "", err
	}
	var doc struct {
		TargetBranch string `yaml:"target_branch"`
	}
	// A config that does not parse names no target; the config load reports it.
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return "", nil //nolint:nilerr // the config load reports an unreadable config
	}
	return strings.TrimSpace(doc.TargetBranch), nil
}

func (g scopeGit) FileAt(ctx context.Context, rev, path string) ([]byte, bool, error) {
	return historygit.FileAt(ctx, g.dir, rev, path, g.runner)
}

// policyFileNames are the protected policy files of a config bundle, in the
// order a result names them: the config, the baseline, the approved labels.
func policyFileNames(configPath string) []string {
	return []string{filepath.Base(configPath), defaultBaselinePath, defaultLabelsPath}
}

// sameRepository reports whether two directories are in one git repository.
func sameRepository(ctx context.Context, runner toolrun.Runner, a, b string) bool {
	rootA, errA := historygit.RepoRoot(ctx, a, runner)
	rootB, errB := historygit.RepoRoot(ctx, b, runner)
	if errA != nil || errB != nil {
		return false
	}
	realA, errA := filepath.EvalSymlinks(rootA)
	realB, errB := filepath.EvalSymlinks(rootB)
	return errA == nil && errB == nil && realA == realB
}

// policyFiles lists the bundle's policy files as the working tree holds them,
// with paths relative to the repository root. dir is the analysis root, whose
// repository the target is read from. A bundle in another repository, or in
// none, has no target tip to compare with: it yields no files.
func policyFiles(ctx context.Context, runner toolrun.Runner, dir, configPath string) ([]application.PolicyFile, error) {
	bundleDir, err := filepath.Abs(filepath.Dir(configPath))
	if err != nil {
		return nil, err
	}
	if !sameRepository(ctx, runner, dir, bundleDir) {
		return nil, nil
	}
	gitRoot, err := historygit.RepoRoot(ctx, bundleDir, runner)
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

// resolveScope fills in the policy files for the doctor scope report.
func resolveScope(ctx context.Context, runner toolrun.Runner, dir, configPath string, req application.ScopeRequest) (application.ScopeResolution, error) {
	files, err := policyFiles(ctx, runner, dir, configPath)
	if err != nil {
		return application.ScopeResolution{}, fmt.Errorf("read the policy files: %w", err)
	}
	req.Policy = files
	res, err := application.ResolveScope(ctx, scopeGit{runner: runner, dir: dir}, req)
	if err != nil {
		return res, fmt.Errorf("resolve the scope: %w", err)
	}
	return res, nil
}
