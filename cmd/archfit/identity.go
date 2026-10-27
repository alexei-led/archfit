package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/alexei-led/archfit/v3/internal/application"
	historygit "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/model/report"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// dirtySuffix marks a head tree digest measured with uncommitted edits.
const dirtySuffix = report.DirtySuffix

// identityInputs reads the facts a result's identity block needs: the sha256
// of the three policy files as they are on disk, the commit the change is
// compared against, and the tree that was measured. A fact that cannot be read
// (no git, no baseline file) stays empty rather than guessed.
//
// The policy digests follow the config bundle. The git facts follow the
// analysis root (--root, else the config's directory): an external policy
// directory may belong to another repository or to none.
func identityInputs(ctx context.Context, runner toolrun.Runner, configPath, root, bundleDir, baseRef string) application.IdentityInputs {
	in := application.IdentityInputs{
		PolicyDigest:   fileDigest(configPath),
		BaselineDigest: fileDigest(filepath.Join(bundleDir, defaultBaselinePath)),
		LabelsDigest:   fileDigest(filepath.Join(bundleDir, defaultLabelsPath)),
		EngineVersion:  version,
	}
	dir := root
	if dir == "" {
		dir = filepath.Dir(configPath)
	}
	if baseRef != "" {
		if sha, err := historygit.ResolveCommit(ctx, dir, baseRef, runner); err == nil {
			in.BaseSHA = sha
		}
	}
	if tree, err := historygit.TreeID(ctx, dir, runner); err == nil {
		in.HeadTreeDigest = tree
		if dirty, err := worktreeDirty(ctx, runner, dir); err == nil && dirty {
			in.HeadTreeDigest += dirtySuffix
		}
	}
	return in
}

// fileDigest returns the sha256 of a file's bytes, or "" when it cannot be read.
func fileDigest(path string) string {
	raw, err := os.ReadFile(path) //#nosec G304 -- the policy bundle paths come from --config
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
