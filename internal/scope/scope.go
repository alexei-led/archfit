// Package scope resolves the analysis scope: the repo root, changed files
// (delta mode), and the mode (full vs delta) for a run.
package scope

import (
	"context"
	"fmt"

	"github.com/alexei-led/archfit/internal/config"
	git "github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// ScopeMode distinguishes full-repo analysis from delta (diff-based) analysis.
// The name is intentionally ScopeMode (not Mode) to match the design contract
// shared across packages; the stutter is acceptable here.
//
//nolint:revive // ScopeMode is the design-specified name; used as scope.ScopeMode across packages intentionally
type ScopeMode string

const (
	// ModeDelta analyses only files changed between Base and Head.
	ModeDelta ScopeMode = "delta"
	// ModeFull analyses the entire repository.
	ModeFull ScopeMode = "full"
)

// Scope carries the resolved analysis scope for a single run.
type Scope struct {
	// Base is the git ref used as the diff base in delta mode; empty in full mode.
	Base string
	// Head is the resolved HEAD SHA in delta mode; empty in full mode.
	Head string
	// Changed is the sorted list of repo-relative files changed since Base.
	// Nil/empty in full mode.
	Changed []string
	// Root is the absolute path to the repository root.
	Root string
	// Mode indicates whether this is a delta or full-repo run.
	Mode ScopeMode
}

// Resolve determines the Scope for a run.
//
// It always resolves the repo root via git; a missing git binary is a hard
// error (exit 3). If cfg.Full is true the result has Mode=full and no Changed
// files. Otherwise it calls git.HeadRef and git.Changed to populate the delta.
func Resolve(ctx context.Context, cfg config.ScopeConfig, runner toolrun.Runner) (Scope, error) {
	root, err := git.RepoRoot(ctx, runner)
	if err != nil {
		return Scope{}, fmt.Errorf("scope: resolve repo root: %w", err)
	}

	if cfg.Full {
		return Scope{
			Root: root,
			Mode: ModeFull,
		}, nil
	}

	head, err := git.HeadRef(ctx, runner)
	if err != nil {
		return Scope{}, fmt.Errorf("scope: resolve HEAD: %w", err)
	}

	cs, err := git.Changed(ctx, cfg.Base, head, runner)
	if err != nil {
		return Scope{}, fmt.Errorf("scope: resolve changed files: %w", err)
	}

	return Scope{
		Base:    cfg.Base,
		Head:    head,
		Changed: cs.Files,
		Root:    root,
		Mode:    ModeDelta,
	}, nil
}
