package policy3

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// The Archfit 3 files Convert reads besides the policy, relative to the
// repository root. Convert removes them after it writes the new policy.
const (
	LabelsFile   = ".archfit-labels.yaml"
	BaselineFile = ".archfit-baseline.json"
	AppFile      = ".archfit-app.yaml"
)

// archfit3Version is the policy version of Archfit 3.
const archfit3Version = 2

// draftFiles are the value drafts of Archfit 3's config enrich. Nobody
// approved them, so Convert does not read them, and it does not delete them:
// they may hold work nobody committed.
var draftFiles = []string{".archfit-volatility.yaml", ".archfit-subdomains.yaml", ".archfit-owners.yaml"}

// ErrNotArchfit3 is the error of Convert when the repository has no Archfit 3
// policy: no policy file, or one that is not version 2.
var ErrNotArchfit3 = errors.New("no Archfit 3 policy (.archfit.yaml version 2) to convert")

// archfit3 is what the Archfit 3 files of a repository hold. The decoders live
// here, not in the Archfit 3 packages, so those packages can go.
type archfit3 struct {
	policy policyV2
	// raw is the policy as written, maps in file order, so the conversion can
	// name every key it drops.
	raw yaml.MapSlice
	// app is nil when AppFile is absent.
	app *appV1
	// labels is nil when LabelsFile is absent.
	labels *labelsV1
	// baseline is nil when BaselineFile is absent.
	baseline *baselineV3
	// drafts are the draft files that exist.
	drafts []string
}

// policyV2 holds the keys of an Archfit 3 policy the conversion reads.
type policyV2 struct {
	Version int                 `yaml:"version"`
	Layers  []string            `yaml:"layers"`
	Modules map[string]moduleV2 `yaml:"modules"`
	Rules   []ruleV2            `yaml:"rules"`
	Waivers []waiverV2          `yaml:"waivers"`
}

type moduleV2 struct {
	Paths      []string `yaml:"paths"`
	Public     []string `yaml:"public"`
	Layer      string   `yaml:"layer"`
	Subdomain  string   `yaml:"subdomain"`
	Volatility string   `yaml:"volatility"`
	Role       string   `yaml:"role"`
	// DependsOn is nil when the key is absent: Archfit 3 then allowed any
	// module.
	DependsOn *[]string `yaml:"depends_on"`
}

type ruleV2 struct {
	ID           string   `yaml:"id"`
	Type         string   `yaml:"type"`
	Gate         string   `yaml:"gate"`
	From         string   `yaml:"from"`
	To           string   `yaml:"to"`
	FromModule   string   `yaml:"from_module"`
	ToModule     string   `yaml:"to_module"`
	Max          *int     `yaml:"max"`
	Guard        bool     `yaml:"guard"`
	Rationale    string   `yaml:"rationale"`
	Alternatives []string `yaml:"alternatives"`
	Docs         string   `yaml:"docs"`
}

type waiverV2 struct {
	Rule    string `yaml:"rule"`
	From    string `yaml:"from"`
	To      string `yaml:"to"`
	Reason  string `yaml:"reason"`
	Expires string `yaml:"expires"`
}

// appV1 is .archfit-app.yaml.
type appV1 struct {
	Owners []string `yaml:"owners"`
	Gate   string   `yaml:"gate"`
	raw    yaml.MapSlice
}

// labelsV1 is .archfit-labels.yaml: coupling strengths between module pairs.
type labelsV1 struct {
	Labels []struct {
		From     string `yaml:"from"`
		To       string `yaml:"to"`
		Strength string `yaml:"strength"`
	} `yaml:"labels"`
}

// baselineV3 is .archfit-baseline.json. An accepted entry is a fingerprint and
// a rule id: it names no place.
type baselineV3 struct {
	Accepted []struct {
		RuleID string `json:"rule_id"`
	} `json:"accepted"`
	Metrics map[string]json.RawMessage `json:"metrics"`
}

// readArchfit3 reads the Archfit 3 files of the repository at root. It
// returns ErrNotArchfit3 when the policy is missing or not version 2.
func readArchfit3(root string) (archfit3, error) {
	data, err := os.ReadFile(filepath.Join(root, PolicyFile)) //nolint:gosec // the repository's policy
	if errors.Is(err, fs.ErrNotExist) {
		return archfit3{}, ErrNotArchfit3
	}
	if err != nil {
		return archfit3{}, err
	}
	var version struct {
		Version int `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &version); err != nil {
		return archfit3{}, fmt.Errorf("%s: %w", PolicyFile, err)
	}
	if version.Version != archfit3Version {
		return archfit3{}, ErrNotArchfit3
	}
	var old archfit3
	if err := yaml.Unmarshal(data, &old.policy); err != nil {
		return archfit3{}, fmt.Errorf("%s: %w", PolicyFile, err)
	}
	if err := yaml.UnmarshalWithOptions(data, &old.raw, yaml.UseOrderedMap()); err != nil {
		return archfit3{}, fmt.Errorf("%s: %w", PolicyFile, err)
	}
	if old.app, err = readOptional(root, AppFile, func(b []byte, app *appV1) error {
		if err := yaml.Unmarshal(b, app); err != nil {
			return err
		}
		return yaml.UnmarshalWithOptions(b, &app.raw, yaml.UseOrderedMap())
	}); err != nil {
		return archfit3{}, err
	}
	if old.labels, err = readOptional(root, LabelsFile, func(b []byte, l *labelsV1) error { return yaml.Unmarshal(b, l) }); err != nil {
		return archfit3{}, err
	}
	if old.baseline, err = readOptional(root, BaselineFile, func(b []byte, bl *baselineV3) error { return json.Unmarshal(b, bl) }); err != nil {
		return archfit3{}, err
	}
	for _, d := range draftFiles {
		if _, err := os.Lstat(filepath.Join(root, d)); err == nil {
			old.drafts = append(old.drafts, d)
		}
	}
	return old, nil
}

// readOptional decodes the file name under root, or returns nil when it is
// absent.
func readOptional[T any](root, name string, decode func([]byte, *T) error) (*T, error) {
	data, err := os.ReadFile(filepath.Join(root, name)) //nolint:gosec // an Archfit 3 file of the repository
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var v T
	if err := decode(data, &v); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &v, nil
}
