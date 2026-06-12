// Package scope resolves the analysis scope: the repo root, changed files
// (delta mode), and the mode (full vs delta) for a run.
package scope

import (
	"context"
	"fmt"
	"sort"

	"github.com/alexei-led/archfit/internal/config"
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

// Resolver supplies the version-control queries scope resolution needs.
// cmd wires the concrete git implementation; tests use an in-memory fake.
// scope itself stays free of process and tool dependencies.
type Resolver interface {
	// RepoRoot returns the absolute repository root.
	RepoRoot(ctx context.Context) (string, error)
	// HeadRef returns the resolved HEAD SHA.
	HeadRef(ctx context.Context) (string, error)
	// Changed returns the repo-relative files changed between base and head.
	Changed(ctx context.Context, base, head string) ([]string, error)
}

// Resolve determines the Scope for a run.
//
// It always resolves the repo root via the Resolver; a failing resolver is a
// hard error (exit 3). If cfg.Full is true the result has Mode=full and no
// Changed files. Otherwise HeadRef and Changed populate the delta. Changed
// files are sorted here so scope's determinism contract does not depend on
// resolver discipline.
func Resolve(ctx context.Context, cfg config.ScopeConfig, r Resolver) (Scope, error) {
	root, err := r.RepoRoot(ctx)
	if err != nil {
		return Scope{}, fmt.Errorf("scope: resolve repo root: %w", err)
	}

	if cfg.Full {
		return Scope{
			Root: root,
			Mode: ModeFull,
		}, nil
	}

	head, err := r.HeadRef(ctx)
	if err != nil {
		return Scope{}, fmt.Errorf("scope: resolve HEAD: %w", err)
	}

	changed, err := r.Changed(ctx, cfg.Base, head)
	if err != nil {
		return Scope{}, fmt.Errorf("scope: resolve changed files: %w", err)
	}
	sort.Strings(changed)

	return Scope{
		Base:    cfg.Base,
		Head:    head,
		Changed: changed,
		Root:    root,
		Mode:    ModeDelta,
	}, nil
}
