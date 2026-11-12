package policyfile

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// settingKeys are the top-level keys whose values reach the policy model only
// as flat settings: the model does not read them, and Compare reports their
// changes.
var settingKeys = []string{"languages", "analyzers", "coverage"}

// Resolve reads a schema 3 policy and returns its model: Decode, then the
// defaults of design 6.1, then the checks across keys (resolved.Policy.Check).
// The digest is that of data. Checks against the file tree are
// resolved.Policy.CheckTree. A version 2 file fails with ErrVersion2.
func Resolve(data []byte) (resolved.Policy, error) {
	f, err := Decode(data)
	if err != nil {
		return resolved.Policy{}, err
	}
	settings, err := flatSettings(data)
	if err != nil {
		return resolved.Policy{}, err
	}
	p := f.project()
	p.Settings = settings
	p = p.WithDefaults()
	p.Digest = resolved.Digest(data)
	if err := p.Check(); err != nil {
		return resolved.Policy{}, err
	}
	return p, nil
}

// project maps the file to the model key by key. An absent key stays zero;
// WithDefaults gives it its meaning.
func (f File) project() resolved.Policy {
	p := resolved.Policy{
		Owners:    f.Owners,
		Gate:      resolved.Gate(f.Gate),
		Layers:    f.Layers,
		Modules:   make(map[string]resolved.Module, len(f.Modules)),
		Generated: f.Generated,
		Tests:     f.Tests,
		Exclude:   f.Exclude,
		Builtin: resolved.Builtin{
			Graph:       resolved.Mode(f.Builtin.Graph),
			Coupling:    resolved.Mode(f.Builtin.Coupling),
			Duplication: resolved.Mode(f.Builtin.Duplication),
			Cycle:       resolved.Mode(f.Builtin.Cycle),
			Layers:      resolved.Mode(f.Builtin.Layers),
			Unowned:     resolved.Mode(f.Builtin.Unowned),
		},
	}
	if f.Builtin.MinBalance != nil {
		p.Builtin.MinBalance = *f.Builtin.MinBalance
	}
	for name, m := range f.Modules {
		p.Modules[name] = resolved.Module{
			Paths:      m.Paths,
			Public:     m.Public,
			Volatility: resolved.Volatility(m.Volatility),
			Subdomain:  resolved.Subdomain(m.Subdomain),
			Layer:      m.Layer,
			DependsOn:  m.DependsOn,
		}
	}
	for _, r := range f.Rules {
		rule := resolved.Rule{ID: r.ID, Note: r.Note, Why: r.Why, Instead: r.Instead}
		switch {
		case r.Deny != nil:
			rule.Form, rule.From, rule.To = resolved.FormDeny, r.Deny.From, r.Deny.To
		case r.Only != nil:
			rule.Form, rule.Use, rule.In = resolved.FormOnly, r.Only.Use, r.Only.In
		default:
			rule.Form = resolved.FormNote
		}
		p.Rules = append(p.Rules, rule)
	}
	for _, e := range f.Exceptions {
		exception := resolved.Exception{Rule: e.Rule, From: e.From, To: e.To, Why: e.Why}
		if e.Expires != "" {
			// Decode has checked the date.
			exception.Expires, _ = time.Parse(dateLayout, e.Expires)
		}
		p.Exceptions = append(p.Exceptions, exception)
	}
	return p
}

// flatSettings returns the values under settingKeys as dotted keys, such as
// "languages.go.enabled" -> "true" and "coverage.sources[0].path" -> "c.out".
func flatSettings(data []byte) (map[string]string, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, key := range settingKeys {
		if v, ok := raw[key]; ok {
			flatten(out, key, v)
		}
	}
	return out, nil
}

func flatten(out map[string]string, key string, v any) {
	switch v := v.(type) {
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(v)) {
			flatten(out, key+"."+k, v[k])
		}
	case []any:
		for i, item := range v {
			flatten(out, fmt.Sprintf("%s[%d]", key, i), item)
		}
	default:
		out[key] = fmt.Sprint(v)
	}
}
