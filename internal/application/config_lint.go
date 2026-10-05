package application

import (
	"context"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/policy"
)

// SourceInventoryReader is the config-lint evidence port: the rule-scope source
// inventory of the tree an analysis of root would scan.
type SourceInventoryReader interface {
	SourceInventory(ctx context.Context, root string) (evaluation.Observations, error)
}

// ConfigLintRequest names the policy to lint and the tree to lint it against.
// An empty Root resolves the way analysis resolves it.
type ConfigLintRequest struct {
	Policy policy.PolicySnapshot
	Root   string
}

// ConfigLintService reports configuration defects that loading accepts but that
// silently weaken the policy. It reads the source inventory and runs no
// analyzer. The vacuity decision is the one rule evaluation makes, over the
// same inventory build, so lint and a check of the same config agree about a
// dead selector, with two exceptions. Check has the Rust crate names cargo
// metadata supplies, so it can call a Rust-spelled selector dead that lint
// leaves undecided. `check --lang` turns on a language the config switches
// off, which changes the source in scope; lint reads the config as written.
type ConfigLintService struct {
	Inventory SourceInventoryReader
}

// Execute lints req.Policy against the source inventory of req.Root.
func (s ConfigLintService) Execute(ctx context.Context, req ConfigLintRequest) ([]evaluation.PolicyDiagnostic, error) {
	inventory, err := s.Inventory.SourceInventory(ctx, req.Root)
	if err != nil {
		return nil, err
	}
	return evaluation.LintPolicy(req.Policy, inventory), nil
}
