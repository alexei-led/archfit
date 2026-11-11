package policyfile

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// RuleUnowned is the built-in rule whose exceptions name a path, not a module pair.
const RuleUnowned = "unowned"

const dateLayout = "2006-01-02"

// DurationPattern is the shape of a timeout: a Go duration without a sign, such
// as 90s or 1m30s. The schema publishes the same pattern.
const DurationPattern = `^([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$`

var durationRE = regexp.MustCompile(DurationPattern)

// Decode reads a schema 3 policy. Unknown keys, wrong types, values outside their
// list or range, bad dates, missing required keys and a rule without exactly one
// form are errors. A version 2 file fails with a message that names archfit init.
// When the checks fail, the error lists every problem, one per line.
func Decode(data []byte) (File, error) {
	if err := checkVersion(data); err != nil {
		return File{}, err
	}
	var f File
	if err := yaml.NewDecoder(bytes.NewReader(data), yaml.DisallowUnknownField()).Decode(&f); err != nil {
		return File{}, err
	}
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return File{}, err
	}
	var kinds problems
	checkKinds(&kinds, raw, reflect.TypeFor[File](), "")
	if err := kinds.err(); err != nil {
		return File{}, err
	}
	if err := f.check(); err != nil {
		return File{}, err
	}
	return f, nil
}

// checkVersion reads only the version, before the strict decode, so a version 2
// file gets the conversion message instead of its first unknown key.
func checkVersion(data []byte) error {
	var header struct {
		Version any `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return err
	}
	version := header.Version
	// As in JSON Schema, a number with no fraction is an integer: 3.0 is 3.
	if f, ok := version.(float64); ok && f >= 0 && f == math.Trunc(f) {
		version = uint64(f)
	}
	switch v := version.(type) {
	case nil:
		return errors.New(`version: missing: add "version: 3"`)
	case uint64:
		switch v {
		case Version:
			return nil
		case 2:
			return errors.New(`version 2 is the Archfit 3 policy format: run "archfit init" to convert it to version 3`)
		}
	}
	return fmt.Errorf("version %v: this engine reads version 3", yamlValue(header.Version))
}

// yamlValue prints a value as the file wrote it, quoting strings.
func yamlValue(v any) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprint(v)
}

// problems collects path-qualified messages in file order.
type problems []string

func (p *problems) add(path, format string, args ...any) {
	*p = append(*p, path+": "+fmt.Sprintf(format, args...))
}

func (p problems) err() error {
	if len(p) == 0 {
		return nil
	}
	return errors.New(strings.Join(p, "\n"))
}

func (f File) check() error {
	var p problems
	if len(f.Owners) == 0 {
		p.add("owners", "required: list at least one GitHub login")
	}
	checkEnum(&p, "gate", f.Gate, Gates)
	seen := map[string]bool{}
	for _, layer := range f.Layers {
		if seen[layer] {
			p.add("layers", "%q is listed twice", layer)
		}
		seen[layer] = true
	}
	if len(f.Modules) == 0 {
		p.add("modules", "required: declare at least one module")
	}
	for _, name := range slices.Sorted(maps.Keys(f.Modules)) {
		f.Modules[name].check(&p, "modules."+name)
	}
	f.Builtin.check(&p)
	for i, r := range f.Rules {
		r.check(&p, fmt.Sprintf("rules[%d]", i))
	}
	for i, e := range f.Exceptions {
		e.check(&p, fmt.Sprintf("exceptions[%d]", i))
	}
	f.checkSwitches(&p)
	if t := f.Analyzers.Roslyn.Timeout; t != "" {
		if !durationRE.MatchString(t) {
			p.add("analyzers.roslyn.timeout", "%q is not a Go duration, such as 5m", t)
		}
	}
	f.Coverage.check(&p)
	return p.err()
}

func (m Module) check(p *problems, path string) {
	if len(m.Paths) == 0 {
		p.add(path+".paths", "required: list at least one glob")
	}
	if m.Public != nil && len(m.Public) == 0 {
		p.add(path+".public", "empty: list at least one glob, or remove the key to make the whole module public")
	}
	checkEnum(p, path+".volatility", m.Volatility, Volatilities)
	checkEnum(p, path+".subdomain", m.Subdomain, Subdomains)
}

func (b Builtin) check(p *problems) {
	for _, mode := range []struct {
		key  string
		mode Mode
	}{
		{"graph", b.Graph}, {"coupling", b.Coupling}, {"duplication", b.Duplication},
		{"cycle", b.Cycle}, {"layers", b.Layers}, {"unowned", b.Unowned},
	} {
		checkEnum(p, "builtin."+mode.key, mode.mode, Modes)
	}
	if b.MinBalance != nil && (*b.MinBalance < 1 || *b.MinBalance > 10) {
		p.add("builtin.min_balance", "%d is outside 1 to 10", *b.MinBalance)
	}
}

func (r Rule) check(p *problems, path string) {
	if r.ID == "" {
		p.add(path+".id", "required: give the rule a short name")
	} else {
		path += " (" + r.ID + ")"
	}
	var forms []string
	if r.Deny != nil {
		forms = append(forms, "deny")
	}
	if r.Only != nil {
		forms = append(forms, "only")
	}
	if r.Note != "" {
		forms = append(forms, "note")
	}
	switch len(forms) {
	case 0:
		p.add(path, "needs one of deny, only or note")
	case 1:
	default:
		p.add(path, "has %s: a rule has exactly one of deny, only or note", strings.Join(forms, " and "))
	}
	if r.Why == "" {
		p.add(path+".why", "required: give the reason for the rule")
	}
	if r.Deny != nil {
		checkNames(p, path+".deny.from", r.Deny.From, "name a module or a path glob")
		checkNames(p, path+".deny.to", r.Deny.To, "name a module or a path glob")
		if r.Instead == "" {
			p.add(path+".instead", "required with deny: say what to do instead")
		}
	}
	if r.Only != nil {
		if r.Only.Use == "" {
			p.add(path+".only.use", "required: name a library or a package set (network, process, database)")
		}
		checkNames(p, path+".only.in", r.Only.In, "name the modules that may use it")
	}
}

func (e Exception) check(p *problems, path string) {
	if e.Rule == "" {
		p.add(path+".rule", "required: name the rule")
	}
	if e.From == "" {
		p.add(path+".from", "required: name the module, or for unowned the path glob")
	}
	switch {
	case e.Rule == RuleUnowned && e.To != "":
		p.add(path+".to", "unowned has no target module: remove to")
	case e.Rule != RuleUnowned && e.To == "":
		p.add(path+".to", "required: name the module the errors reach")
	}
	if e.Expires != "" {
		if _, err := time.Parse(dateLayout, e.Expires); err != nil {
			p.add(path+".expires", "%q is not a date: use YYYY-MM-DD", e.Expires)
		}
	}
	if e.Why == "" {
		p.add(path+".why", "required: give the reason for the exception")
	}
}

func (c Coverage) check(p *problems) {
	if c.Enabled && len(c.Sources) == 0 {
		p.add("coverage.sources", "required when coverage is enabled: list at least one file")
	}
	for i, s := range c.Sources {
		path := fmt.Sprintf("coverage.sources[%d]", i)
		if s.Path == "" {
			p.add(path+".path", "required: name the coverage file")
		}
		checkEnum(p, path+".format", s.Format, CoverageFormats)
		if s.MaxBytes != nil && *s.MaxBytes < 1 {
			p.add(path+".max_bytes", "%d is below 1", *s.MaxBytes)
		}
		if s.MaxFacts != nil && *s.MaxFacts < 1 {
			p.add(path+".max_facts", "%d is below 1", *s.MaxFacts)
		}
	}
}

func (f File) checkSwitches(p *problems) {
	for _, s := range []struct {
		path string
		v    Switch
	}{
		{"languages.go.enabled", f.Languages.Go.Enabled},
		{"languages.typescript.enabled", f.Languages.TypeScript.Enabled},
		{"languages.python.enabled", f.Languages.Python.Enabled},
		{"languages.rust.enabled", f.Languages.Rust.Enabled},
		{"languages.dotnet.enabled", f.Languages.DotNet.Enabled},
		{"analyzers.syntax.enabled", f.Analyzers.Syntax.Enabled},
		{"analyzers.cargo_modules.enabled", f.Analyzers.CargoModules.Enabled},
		{"analyzers.roslyn.enabled", f.Analyzers.Roslyn.Enabled},
	} {
		if s.v != "" && !slices.Contains(Switches, s.v) {
			p.add(s.path, `%s is not true, false or "auto"`, s.v)
		}
	}
}

// checkNames requires a set, non-empty list. checkKinds already rejected empty names.
func checkNames(p *problems, path string, n Names, hint string) {
	switch {
	case n == nil:
		p.add(path, "required: %s", hint)
	case len(n) == 0:
		p.add(path, "empty: %s", hint)
	}
}

// checkEnum accepts an absent value or one from the list.
func checkEnum[T ~string](p *problems, path string, v T, allowed []T) {
	if v == "" || slices.Contains(allowed, v) {
		return
	}
	words := make([]string, len(allowed))
	for i, a := range allowed {
		words[i] = string(a)
	}
	p.add(path, "%q is not one of %s", string(v), strings.Join(words, ", "))
}
