// Package policy3 writes the first schema 3 policy of a repository and the
// instruction block for agents (design 10.1 and 11.3).
//
// Init reads the head check facts, the facts archfit check reads, finds the
// modules in them, and writes .archfit.yaml: modules with paths, public (the
// packages other modules use today), depends_on (the dependencies between
// modules that exist), tests, generated, owners from the origin remote, an only
// rule for each package set that only some modules use, and an exception for
// each error the written policy reports on the head, so the first check
// passes. depends_on, public and the only rules come from the rules of
// internal/assessment/change, so the policy and the check agree by
// construction. Init then writes the five lines of design 11.3 between markers
// in AGENTS.md.
//
// Init never overwrites a policy file: converting an Archfit 3 policy is a
// separate case. A missing tool stops Init before it writes anything, because a
// policy written from missing evidence is wrong.
package policy3

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition/checkfacts"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// The files Init writes, relative to the repository root.
const (
	PolicyFile = ".archfit.yaml"
	AgentsFile = "AGENTS.md"
)

// ErrPolicyExists is the error of Init when the repository already has a
// policy file. Init writes nothing then.
var ErrPolicyExists = errors.New("a policy file exists: archfit init does not overwrite it")

// HeadFacts reads the check facts of the working tree; checkfacts.Acquirer is
// one, configured with FactsConfig.
type HeadFacts interface {
	Head(ctx context.Context, root string) (evidence.CheckFacts, error)
}

// Deps are what Init runs.
type Deps struct {
	// Runner runs git, to read the origin remote.
	Runner toolrun.Runner
	Facts  HeadFacts
}

// Result is what Init wrote and what it could not read.
type Result struct {
	// File is the policy written to PolicyFile.
	File policyfile.File
	// Unknown are the items of the head facts that are not a missing tool,
	// such as an import that does not resolve. Init writes the policy without
	// them; the caller shows them.
	Unknown []evidence.UnknownItem
}

// FactsConfig is the configuration of the check facts the written policy
// gives: it names no languages and no exclude, so the defaults apply, and the
// first check reads the facts Init read.
func FactsConfig() checkfacts.Config {
	return checkfacts.ConfigFrom(resolved.Policy{}.WithDefaults())
}

// Init writes the policy of the repository at root and the instruction block
// in its AGENTS.md. It returns ErrPolicyExists, and writes nothing, when the
// repository has a policy file.
func Init(ctx context.Context, deps Deps, root string) (Result, error) {
	policyPath := filepath.Join(root, PolicyFile)
	if err := absent(policyPath); err != nil {
		return Result{}, err
	}
	agentsPath := filepath.Join(root, AgentsFile)
	agents, err := spliceFile(agentsPath)
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w", AgentsFile, err)
	}
	owner, err := originOwner(ctx, root, deps.Runner)
	if err != nil {
		return Result{}, err
	}
	facts, err := deps.Facts.Head(ctx, root)
	if err != nil {
		return Result{}, err
	}
	unknown, err := withoutMissingTools(facts.Unknown)
	if err != nil {
		return Result{}, err
	}
	file, data, err := build(root, owner, facts)
	if err != nil {
		return Result{}, err
	}
	if err := create(policyPath, data); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(agentsPath, agents, 0o644); err != nil { //nolint:gosec // an instruction file is world-readable by design
		return Result{}, fmt.Errorf("%s: %w", AgentsFile, err)
	}
	return Result{File: file, Unknown: unknown}, nil
}

// absent returns ErrPolicyExists when path exists.
func absent(path string) error {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return fmt.Errorf("%s: %w", path, ErrPolicyExists)
	case errors.Is(err, fs.ErrNotExist):
		return nil
	default:
		return err
	}
}

// create writes a new file, and never one that exists.
func create(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // the policy is world-readable by design
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s: %w", path, ErrPolicyExists)
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// withoutMissingTools returns the unknown items that are not a missing or
// failing tool, or an error that names each such tool and its fix.
func withoutMissingTools(items []evidence.UnknownItem) ([]evidence.UnknownItem, error) {
	var rest []evidence.UnknownItem
	var missing []string
	for _, u := range items {
		if u.Cause == evidence.CauseTool {
			missing = append(missing, fmt.Sprintf("  %s\n  next: %s", u.Reason, u.Fix))
			continue
		}
		rest = append(rest, u)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("archfit init needs every fact of the code before it writes a policy:\n%s",
			strings.Join(missing, "\n"))
	}
	return rest, nil
}
