package main

import (
	"context"
	"strings"

	"github.com/alexei-led/archfit/v4/internal/application"
	"github.com/alexei-led/archfit/v4/internal/config"
	"github.com/alexei-led/archfit/v4/internal/evidence/acquisition"
	historygit "github.com/alexei-led/archfit/v4/internal/history/git"
	"github.com/alexei-led/archfit/v4/internal/policy/resolved"
	"github.com/alexei-led/archfit/v4/internal/toolrun"
)

// reportPolicySource never rereads disk to redefine the consumed policy.
type reportPolicySource struct{ policy resolved.Policy }

func (s reportPolicySource) Policy(context.Context) (resolved.Policy, error) { return s.policy, nil }

func reportEvidenceService(file, root string, in config.ReportInput, deps *appDeps) *acquisition.Service {
	s := newEvidenceService(file, root, in.Config, deps)
	s.ConsumedPolicyDigest = strings.TrimPrefix(in.Policy.Digest, "sha256:")
	return s
}

// reportIdentityInputs binds the consumed bytes; Git remains a separate fact.
func reportIdentityInputs(ctx context.Context, runner toolrun.Runner, digest, root string) application.IdentityInputs {
	in := application.IdentityInputs{PolicyDigest: strings.TrimPrefix(digest, "sha256:"), EngineVersion: engineVersion()}
	if tree, err := historygit.TreeID(ctx, root, runner); err == nil {
		in.HeadTreeDigest = tree
		if dirty, err := worktreeDirty(ctx, runner, root); err == nil && dirty {
			in.HeadTreeDigest += dirtySuffix
		}
	}
	return in
}
