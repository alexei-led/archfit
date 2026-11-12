package policy3

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"

	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition/checkfacts"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// ConvertDeps are what Convert runs.
type ConvertDeps struct {
	// Runner runs git, to read the origin remote when the App file names no
	// owners.
	Runner toolrun.Runner
	// Facts returns the reader of the head facts for a configuration: the
	// languages and exclude of the converted policy.
	Facts func(checkfacts.Config) HeadFacts
}

// Convert converts the Archfit 3 files of the repository at root into one
// schema 3 policy (design 14): .archfit.yaml version 2 with its waivers,
// .archfit-labels.yaml, .archfit-baseline.json and .archfit-app.yaml. It
// writes the policy over the old one, writes the instruction block in
// AGENTS.md as Init does, and removes the other Archfit 3 files. Conversion
// lists what it did not convert and why.
//
// The policy takes the modules, public, depends_on, layers, subdomains and
// volatilities of the old policy, its forbidden dependencies as deny rules,
// the owners and gate of the App file, and the tests and generated code
// Init finds. A bare directory in a module's paths becomes dir/*: Archfit 3
// matched it as one package, so a directory below it stays out of the
// module. Every error the policy reports on the head gets an exception, as in
// Init, so the first check passes; a waiver keeps its date and reason.
//
// It returns ErrNotArchfit3, and writes nothing, when the repository has no
// Archfit 3 policy. A missing tool, or a policy that would not load or fit the
// tree, stops it before it writes anything.
func Convert(ctx context.Context, deps ConvertDeps, root string) (Conversion, error) {
	old, err := readArchfit3(root)
	if err != nil {
		return Conversion{}, err
	}
	policyPath := filepath.Join(root, PolicyFile)
	oldPolicy, err := os.ReadFile(policyPath) //nolint:gosec // the repository's policy
	if err != nil {
		return Conversion{}, err
	}
	agentsPath := filepath.Join(root, AgentsFile)
	agents, err := spliceFile(agentsPath)
	if err != nil {
		return Conversion{}, fmt.Errorf("%s: %w", AgentsFile, err)
	}
	owner := ""
	if old.app == nil || len(old.app.Owners) == 0 {
		if owner, err = originOwner(ctx, root, deps.Runner); err != nil {
			return Conversion{}, err
		}
	}
	cfg, err := factsConfig(old)
	if err != nil {
		return Conversion{}, err
	}
	facts, err := deps.Facts(cfg).Head(ctx, root)
	if err != nil {
		return Conversion{}, err
	}
	unknown, err := withoutMissingTools(facts.Unknown)
	if err != nil {
		return Conversion{}, err
	}
	gen, err := generated(root, facts.Sources)
	if err != nil {
		return Conversion{}, err
	}
	conv, data, err := convert(old, facts, gen, owner)
	if err != nil {
		return Conversion{}, err
	}
	conv.Unknown = unknown
	if err := replace(policyPath, data); err != nil {
		return Conversion{}, err
	}
	if err := os.WriteFile(agentsPath, agents, 0o644); err != nil { //nolint:gosec // an instruction file is world-readable by design
		// Without the old policy back, a second run finds no Archfit 3
		// policy and can never write the block.
		return Conversion{}, errors.Join(fmt.Errorf("%s: %w", AgentsFile, err), replace(policyPath, oldPolicy))
	}
	for _, name := range []string{AppFile, BaselineFile, LabelsFile} {
		err := os.Remove(filepath.Join(root, name))
		switch {
		case err == nil:
			conv.Removed = append(conv.Removed, name)
		case !errors.Is(err, fs.ErrNotExist):
			return conv, fmt.Errorf("the policy is converted, but %s is not removed: %w", name, err)
		}
	}
	return conv, nil
}

// factsConfig is the configuration of the check facts the converted policy
// gives: its languages and exclude, read through the schema 3 loader.
func factsConfig(old archfit3) (checkfacts.Config, error) {
	c := &converter{old: old}
	if err := c.settings(); err != nil {
		return checkfacts.Config{}, err
	}
	f := c.file
	f.Version, f.Owners = policyfile.Version, []string{"owner"}
	f.Modules = map[string]policyfile.Module{"all": {Paths: []string{"**"}}}
	data, err := yaml.Marshal(f)
	if err != nil {
		return checkfacts.Config{}, err
	}
	p, err := policyfile.Resolve(data)
	if err != nil {
		return checkfacts.Config{}, fmt.Errorf("%s: the language and analyzer keys do not convert:\n%w", PolicyFile, err)
	}
	return checkfacts.ConfigFrom(p), nil
}

// replace writes data to path through a file next to it, so path holds the
// old bytes or the new ones, never a part.
func replace(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".archfit-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after the rename; cleanup on error paths
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
