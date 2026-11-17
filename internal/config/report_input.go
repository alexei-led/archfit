package config

import (
	"slices"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/alexei-led/archfit/v4/internal/config/policyfile"
	"github.com/alexei-led/archfit/v4/internal/model/evidence"
	"github.com/alexei-led/archfit/v4/internal/policy/resolved"
)

// ReportInput owns a private copy of the first consumed policy bytes and all
// report views derived from them. It is never persisted.
type ReportInput struct {
	data       []byte
	Config     Config
	Policy     resolved.Policy
	Components []evidence.ReportComponent
}

// NewReportInput derives report configuration and declarations from one byte copy.
func NewReportInput(data []byte) (ReportInput, error) {
	data = slices.Clone(data)
	p, err := policyfile.Resolve(data)
	if err != nil {
		return ReportInput{}, err
	}
	f, err := policyfile.Decode(data)
	if err != nil {
		return ReportInput{}, err
	}
	// Decode preserves list omission, but scalar zero values cannot establish
	// lexical presence. Extract only these typed declarations from the same bytes.
	var lexical struct {
		Modules map[string]struct {
			Volatility *string `yaml:"volatility"`
			Subdomain  *string `yaml:"subdomain"`
		} `yaml:"modules"`
	}
	if err := yaml.Unmarshal(data, &lexical); err != nil {
		return ReportInput{}, err
	}
	in := ReportInput{data: data, Config: reportConfig(f, p), Policy: p}
	names := make([]string, 0, len(f.Modules))
	for name := range f.Modules {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw, effective := f.Modules[name], p.Modules[name]
		scalar := lexical.Modules[name]
		ref := func(key string) []evidence.ReportRef {
			digest := p.Digest
			return []evidence.ReportRef{{SHA256: &digest, Selector: "/modules/" + strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1") + "/" + key, Status: "captured"}}
		}
		public := evidence.ReportPathsFact{Effective: new(slices.Clone(effective.Public)), Origin: evidence.ReportAsserted, Refs: ref("public")}
		if raw.Public == nil {
			public.Origin = evidence.ReportAssumed
			public.Derivation = "omitted public defaults to the whole module"
			public.Refs = ref("paths")
		} else {
			public.Raw = new(slices.Clone(raw.Public))
		}
		subdomain := evidence.ReportStringFact{Raw: scalar.Subdomain, Origin: evidence.ReportUnknown, Refs: []evidence.ReportRef{}}
		if scalar.Subdomain != nil {
			subdomain.Effective = new(string(effective.Subdomain))
			subdomain.Origin = evidence.ReportAsserted
			subdomain.Refs = ref("subdomain")
		}
		volatility := evidence.ReportStringFact{Raw: scalar.Volatility, Effective: new(string(effective.Volatility)), Origin: evidence.ReportAssumed, Refs: []evidence.ReportRef{}}
		switch {
		case raw.Volatility != "":
			volatility.Origin = evidence.ReportAsserted
			volatility.Refs = ref("volatility")
		case raw.Subdomain != "":
			volatility.Origin = evidence.ReportAssessed
			volatility.Derivation = "policy default mapping: core=high, supporting/generic=low"
			volatility.Refs = ref("subdomain")
		default:
			volatility.Derivation = "omitted or empty volatility defaults conservatively to high"
		}
		unknowns := []evidence.ReportUnknownFact{}
		if raw.Subdomain == "" {
			unknowns = append(unknowns, evidence.ReportUnknownFact{Fact: "subdomain", Reason: "no nonempty subdomain assertion", Owner: "policy"})
		}
		in.Components = append(in.Components, evidence.ReportComponent{Name: name, Declared: true, Public: public, Subdomain: subdomain, Volatility: volatility, Unknowns: unknowns})
	}
	return in, nil
}

// Bytes returns a copy so callers cannot redefine consumed policy identity.
func (in ReportInput) Bytes() []byte { return slices.Clone(in.data) }
