// Package policyfile is the policy file contract of Archfit 4: the types of a
// schema 3 `.archfit.yaml` and a strict decoder.
//
// The types mirror the file. An absent key keeps its zero value (nil slice, empty
// string, nil pointer), so the policy model can tell an absent key from a set one
// and apply the defaults of design section 6.1. The doc comments become the
// descriptions of the published archfit.schema.json and state those defaults.
//
// Decode enforces exactly what the schema states: keys, types, closed values,
// ranges, dates, required keys and the one form of a rule. Checks that need more
// than one key or the file tree (an unknown module, a file in two modules, a glob
// that matches nothing) belong to the policy model.
package policyfile

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/goccy/go-yaml"
)

// Version is the only policy schema version this package reads.
const Version = 3

// File is an Archfit policy: modules, allowed dependencies, rules and exceptions.
type File struct {
	// The policy schema version. Always 3.
	Version int `yaml:"version" jsonschema:"required"`
	// GitHub logins of the people who write the policy and answer requests.
	Owners []string `yaml:"owners" jsonschema:"required,minItems=1"`
	// Whether the check blocks a pull request. Absent: advisory, so the check reports and does not block.
	Gate Gate `yaml:"gate,omitempty" jsonschema:"default=advisory"`
	// Layer names, inner first. Absent: no layer rule.
	Layers []string `yaml:"layers,omitempty" jsonschema:"uniqueItems=true"`
	// The modules: named sets of paths, keyed by module name.
	Modules map[string]Module `yaml:"modules" jsonschema:"required"`
	// Changes the modes of the built-in rules. Absent: the defaults of each rule apply.
	Builtin Builtin `yaml:"builtin,omitempty"`
	// The custom rules. Absent: none; the built-in rules still apply.
	Rules []Rule `yaml:"rules,omitempty"`
	// Permissions to break one rule in one place. Absent: none.
	Exceptions []Exception `yaml:"exceptions,omitempty"`
	// Globs of generated code, which the rules skip. Absent: only listed paths are out; a file name that looks generated is not enough.
	Generated []string `yaml:"generated,omitempty"`
	// Globs of test code, which the rules skip. Absent: only listed paths are out.
	Tests []string `yaml:"tests,omitempty"`
	// Globs of paths that no extractor reads, such as vendored or fixture trees. Absent: only the built-in exclusions apply.
	Exclude []string `yaml:"exclude,omitempty"`
	// The settings of each language extractor.
	Languages Languages `yaml:"languages,omitempty"`
	// The settings of the optional analyzers.
	Analyzers Analyzers `yaml:"analyzers,omitempty"`
	// Coverage files that your CI writes. The report reads them; Archfit never runs tests.
	Coverage Coverage `yaml:"coverage,omitempty"`
}

// Module is a named set of paths with a public part, a volatility and the modules it may depend on.
type Module struct {
	// Globs of the files that belong to the module.
	Paths []string `yaml:"paths" jsonschema:"required,minItems=1"`
	// Globs other modules may use. Absent: the whole module is public. Add public to protect its inside.
	Public []string `yaml:"public,omitempty" jsonschema:"minItems=1"`
	// How often the module is expected to change. Absent: taken from subdomain (core is high; supporting and generic are low), otherwise high.
	Volatility Volatility `yaml:"volatility,omitempty"`
	// The business role of the module. It sets the volatility only when volatility is absent. Absent: no business role is recorded.
	Subdomain Subdomain `yaml:"subdomain,omitempty"`
	// A name from layers. Absent: the module has no layer.
	Layer string `yaml:"layer,omitempty"`
	// The modules this module may depend on. Absent: the module may depend on no other module.
	DependsOn []string `yaml:"depends_on,omitempty"`
}

// Builtin holds the modes of the built-in rules and the minimum balance.
type Builtin struct {
	// A dependency between two modules must be an allowed dependency. Absent: ask.
	Graph Mode `yaml:"graph,omitempty" jsonschema:"default=ask"`
	// New knowledge across a module boundary needs a balance of at least min_balance. Absent: deny.
	Coupling Mode `yaml:"coupling,omitempty" jsonschema:"default=deny"`
	// Knowledge that another module already owns is not copied. Absent: ask.
	Duplication Mode `yaml:"duplication,omitempty" jsonschema:"default=ask"`
	// No new cycle between modules. Absent: deny.
	Cycle Mode `yaml:"cycle,omitempty" jsonschema:"default=deny"`
	// Dependencies follow the layer order. Absent: deny.
	Layers Mode `yaml:"layers,omitempty" jsonschema:"default=deny"`
	// New production code belongs to a module. Absent: deny.
	Unowned Mode `yaml:"unowned,omitempty" jsonschema:"default=deny"`
	// The lowest balance the coupling rule accepts, from 1 to 10. Absent: 7.
	MinBalance *int `yaml:"min_balance,omitempty" jsonschema:"minimum=1,maximum=10,default=7"`
}

// Rule is a custom rule: exactly one of deny, only or note, with its reason.
type Rule struct {
	// The rule's short name, as in a linter. It appears in every error.
	ID string `yaml:"id" jsonschema:"required,minLength=1"`
	// Forbids a dependency from these modules or paths to those. It needs instead.
	Deny *Deny `yaml:"deny,omitempty" jsonschema:"oneof_required=deny"`
	// Keeps a library, or a named package set (network, process, database), in the listed modules.
	Only *Only `yaml:"only,omitempty" jsonschema:"oneof_required=only"`
	// A rule nothing checks today, written down. It shows as "not checked".
	Note string `yaml:"note,omitempty" jsonschema:"oneof_required=note,minLength=1"`
	// The reason for the rule.
	Why string `yaml:"why" jsonschema:"required,minLength=1"`
	// What to do instead. Required for deny.
	Instead string `yaml:"instead,omitempty" jsonschema:"minLength=1"`
}

// Deny names the source and the target of a forbidden dependency.
type Deny struct {
	// A module name or a path glob, or a list of them.
	From Names `yaml:"from" jsonschema:"required"`
	// A module name or a path glob, or a list of them.
	To Names `yaml:"to" jsonschema:"required"`
}

// Only names a library or package set and the modules that may use it.
type Only struct {
	// A library import path or a package set: network, process or database.
	Use string `yaml:"use" jsonschema:"required,minLength=1"`
	// The module, or a list of modules, that may use it.
	In Names `yaml:"in" jsonschema:"required"`
}

// Exception permits errors of one rule in one place, with a reason and usually a date.
type Exception struct {
	// The id of a built-in or custom rule.
	Rule string `yaml:"rule" jsonschema:"required,minLength=1"`
	// The module the errors come from. For unowned, the glob of the code no module owns.
	From string `yaml:"from" jsonschema:"required,minLength=1"`
	// The module the errors reach. Required for every rule except unowned and a custom only rule, which have no target: an only rule names its library itself.
	To string `yaml:"to,omitempty" jsonschema:"minLength=1"`
	// The last day the exception holds, as YYYY-MM-DD. After it, while the code still breaks the rule, every change fails. Absent: no date.
	Expires string `yaml:"expires,omitempty" jsonschema:"format=date,pattern=^[0-9]{4}-[0-9]{2}-[0-9]{2}$"`
	// The reason for the exception.
	Why string `yaml:"why" jsonschema:"required,minLength=1"`
}

// Languages holds one block per language extractor.
type Languages struct {
	// The Go extractor.
	Go GoLanguage `yaml:"go,omitempty"`
	// The TypeScript and JavaScript extractor.
	TypeScript Language `yaml:"typescript,omitempty"`
	// The Python extractor.
	Python PythonLanguage `yaml:"python,omitempty"`
	// The Rust extractor.
	Rust RustLanguage `yaml:"rust,omitempty"`
	// The .NET extractor.
	DotNet DotNetLanguage `yaml:"dotnet,omitempty"`
}

// Language is a language extractor with no settings besides enabled.
type Language struct {
	// Turns the extractor on or off. Absent: auto, which reads the language when its files exist.
	Enabled Switch `yaml:"enabled,omitempty"`
}

// GoLanguage configures the Go extractor.
type GoLanguage struct {
	// Turns the extractor on or off. Absent: auto, which reads the language when its files exist.
	Enabled Switch `yaml:"enabled,omitempty"`
	// Selects go.work members. Absent: every member.
	Modules GoModules `yaml:"modules,omitempty"`
}

// GoModules selects go.work members by globs relative to the repository root.
type GoModules struct {
	// Keeps only the members that match. Absent: every member.
	Include []string `yaml:"include,omitempty"`
	// Drops the members that match, after include. Absent: none.
	Exclude []string `yaml:"exclude,omitempty"`
}

// PythonLanguage configures the Python extractor.
type PythonLanguage struct {
	// Turns the extractor on or off. Absent: auto, which reads the language when its files exist.
	Enabled Switch `yaml:"enabled,omitempty"`
	// The top-level importable package, such as myapp. Absent: found from the layout, src first.
	Package string `yaml:"package,omitempty"`
}

// RustLanguage configures the Rust extractor.
type RustLanguage struct {
	// Turns the extractor on or off. Absent: auto, which reads the language when its files exist.
	Enabled Switch `yaml:"enabled,omitempty"`
	// The path of Cargo.toml. Absent: the root manifest.
	Manifest string `yaml:"manifest,omitempty"`
	// The cargo features to turn on. Absent: the default features.
	Features []string `yaml:"features,omitempty"`
	// Adds dev-dependencies as crate dependencies. Absent: false.
	IncludeDevDeps bool `yaml:"include_dev_deps,omitempty" jsonschema:"default=false"`
}

// DotNetLanguage configures the .NET extractor.
type DotNetLanguage struct {
	// Turns the extractor on or off. Absent: auto, which reads the language when its files exist.
	Enabled Switch `yaml:"enabled,omitempty"`
	// The .slnx, .sln, .slnf or .csproj files to read, relative to the repository root. Absent: the first .slnx or .sln at the root, else every .csproj outside obj/, bin/ and excluded paths.
	Projects []string `yaml:"projects,omitempty"`
}

// Analyzers holds the optional analyzers.
type Analyzers struct {
	// The ast-grep syntax pass. The report uses it for function size. Absent: off.
	Syntax Analyzer `yaml:"syntax,omitempty"`
	// Reads the module graph inside Rust crates. It compiles the crate. Absent: off.
	CargoModules Analyzer `yaml:"cargo_modules,omitempty"`
	// Reads the .NET type graph with the Roslyn helper. Absent: on where the helper is installed.
	Roslyn TimedAnalyzer `yaml:"roslyn,omitempty"`
}

// Analyzer is an optional analyzer with no settings besides enabled.
type Analyzer struct {
	// Turns the analyzer on (true) or off (false). Only true turns on an analyzer that is off by default.
	Enabled Switch `yaml:"enabled,omitempty"`
}

// TimedAnalyzer is an optional analyzer with a time limit.
type TimedAnalyzer struct {
	// Turns the analyzer on (true) or off (false).
	Enabled Switch `yaml:"enabled,omitempty"`
	// A Go duration, such as 5m. Absent or zero: the analyzer's own limit.
	Timeout string `yaml:"timeout,omitempty"`
}

// Coverage names coverage files for the report.
type Coverage struct {
	// Makes the report read the sources. It needs at least one source. Absent: false.
	Enabled bool `yaml:"enabled,omitempty" jsonschema:"default=false"`
	// The coverage files.
	Sources []CoverageSource `yaml:"sources,omitempty"`
}

// CoverageSource is one coverage file.
type CoverageSource struct {
	// The path of the coverage file, relative to the repository root.
	Path string `yaml:"path" jsonschema:"required,minLength=1"`
	// The format of the file. Absent: auto, found from the content.
	Format CoverageFormat `yaml:"format,omitempty" jsonschema:"default=auto"`
	// The file that records the source hashes. Absent: <path>.sidecar.json.
	SidecarPath string `yaml:"sidecar_path,omitempty"`
	// The largest file the report reads. Absent: 67108864 (64 MiB).
	MaxBytes *int64 `yaml:"max_bytes,omitempty" jsonschema:"minimum=1,default=67108864"`
	// The largest number of coverage facts the report keeps. Absent: 1000000.
	MaxFacts *int `yaml:"max_facts,omitempty" jsonschema:"minimum=1,default=1000000"`
}

// Gate says whether the check blocks a pull request.
type Gate string

// Gate values.
const (
	GateAdvisory Gate = "advisory"
	GateRequired Gate = "required"
)

// Gates are the accepted gate values.
var Gates = []Gate{GateAdvisory, GateRequired}

// Mode is how a built-in rule treats an error: deny (fix the code), ask (an owner may approve) or off.
type Mode string

// Mode values.
const (
	ModeDeny Mode = "deny"
	ModeAsk  Mode = "ask"
	ModeOff  Mode = "off"
)

// Modes are the accepted rule modes.
var Modes = []Mode{ModeDeny, ModeAsk, ModeOff}

// Volatility is how often a module is expected to change.
type Volatility string

// Volatility values.
const (
	VolatilityLow    Volatility = "low"
	VolatilityMedium Volatility = "medium"
	VolatilityHigh   Volatility = "high"
)

// Volatilities are the accepted volatility values.
var Volatilities = []Volatility{VolatilityLow, VolatilityMedium, VolatilityHigh}

// Subdomain is the business role of a module.
type Subdomain string

// Subdomain values.
const (
	SubdomainCore       Subdomain = "core"
	SubdomainSupporting Subdomain = "supporting"
	SubdomainGeneric    Subdomain = "generic"
)

// Subdomains are the accepted subdomain values.
var Subdomains = []Subdomain{SubdomainCore, SubdomainSupporting, SubdomainGeneric}

// CoverageFormat is the format of a coverage file.
type CoverageFormat string

// CoverageFormats are the accepted coverage formats, auto first.
var CoverageFormats = []CoverageFormat{"auto", "go-coverprofile", "lcov", "coverage-py-json", "llvm-cov-json", "cobertura"}

// Switch turns an extractor or analyzer on or off: true, false or "auto".
type Switch string

// Switch values. The empty Switch means the key is absent.
const (
	SwitchOn   Switch = "true"
	SwitchOff  Switch = "false"
	SwitchAuto Switch = "auto"
)

// Switches are the accepted values of a set Switch.
var Switches = []Switch{SwitchOn, SwitchOff, SwitchAuto}

// UnmarshalYAML keeps a YAML boolean as true or false, the string auto as auto,
// and any other scalar as written, quoted if a string. Decode then rejects a
// value outside Switches with its key path.
func (s *Switch) UnmarshalYAML(data []byte) error {
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return err
	}
	switch v := v.(type) {
	case bool:
		*s = SwitchOff
		if v {
			*s = SwitchOn
		}
	case string:
		// Only the bare word auto is a value; a quoted "true" is a string the
		// schema rejects, so it must not read as SwitchOn.
		*s = SwitchAuto
		if v != string(SwitchAuto) {
			*s = Switch(strconv.Quote(v))
		}
	case uint64, int64, float64:
		*s = Switch(fmt.Sprint(v))
	default:
		return errors.New(`enabled takes true, false or "auto"`)
	}
	return nil
}

// MarshalYAML writes true and false as YAML booleans and auto as the word, the
// forms UnmarshalYAML reads back as the same Switch. A value outside Switches
// is written as its text.
func (s Switch) MarshalYAML() (any, error) {
	switch s {
	case SwitchOn:
		return true, nil
	case SwitchOff:
		return false, nil
	default:
		return string(s), nil
	}
}

// Names is one name or a list of names. A YAML string decodes to a list of one.
type Names []string

// UnmarshalYAML accepts one string or a list of strings; Decode then rejects an
// empty list or an empty name with its key path.
func (n *Names) UnmarshalYAML(data []byte) error {
	var one string
	if err := yaml.Unmarshal(data, &one); err == nil {
		*n = Names{one}
		return nil
	}
	var list []string
	if err := yaml.Unmarshal(data, &list); err != nil {
		return errors.New("want one name or a list of names")
	}
	*n = Names(list)
	if *n == nil {
		*n = Names{}
	}
	return nil
}
