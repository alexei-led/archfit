package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// candidatesCfg declares three rules on the same import: one with an owner
// alternative that names a package a module owns, one whose alternative names
// nothing the policy owns, and one with no alternative.
func candidatesCfg(rules string) string {
	return `version: 2
modules:
  app:
    paths: ["internal/app/**"]
    depends_on: [store]
  store:
    paths: ["internal/store/**"]
    public: ["internal/store/api"]
    internal: ["internal/store/impl/**"]
  other:
    paths: ["internal/other/**"]
rules:
  - id: allowlists
    type: module_dependencies
    gate: fail
` + rules
}

const (
	candidatesFile = "internal/app/app.go"
	candidatesTo   = "internal/store/impl"
)

func candidatesRepo(t *testing.T, rules string) string {
	t.Helper()
	return writeAgreementFiles(t, t.TempDir(), map[string]string{
		markerGoMod:                   "module example.com/agree\n\ngo 1.21\n",
		defaultConfigPath:             candidatesCfg(rules),
		candidatesFile:                "package app\n\nimport impl \"example.com/agree/internal/store/impl\"\n\nvar _ = impl.Name\n",
		"internal/store/impl/impl.go": "package impl\n\nconst Name = \"impl\"\n",
		"internal/store/api/api.go":   "package api\n\nconst Name = \"api\"\n",
		"internal/other/other.go":     "package other\n\nconst Name = \"other\"\n",
	})
}

type candidatesAnswer struct {
	Answers []struct {
		Answer     string   `json:"answer"`
		Instead    []string `json:"instead"`
		Candidates []struct {
			Path        string `json:"path"`
			Module      string `json:"module"`
			PublicAPI   bool   `json:"public_api"`
			PolicyCheck string `json:"policy_check"`
			Basis       string `json:"basis"`
		} `json:"candidates"`
		CandidatesNote string `json:"candidates_note"`
	} `json:"answers"`
}

func canImportCandidates(t *testing.T, cfg string) candidatesAnswer {
	t.Helper()
	code, stdout, stderr := runArchfit(t, "policy", "can-import", "-c", cfg, candidatesFile, candidatesTo)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (denied)\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var doc candidatesAnswer
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	return doc
}

// TestCanImportNamesWhereToGoInstead: a denial whose rule declares an
// alternative yields the candidate the policy can judge; a rule without
// alternatives yields none.
func TestCanImportNamesWhereToGoInstead(t *testing.T) {
	t.Parallel()
	const deny = `  - id: app_no_store_impl
    type: forbidden_dependency
    from: internal/app/**
    to: internal/store/impl/**
    gate: fail
`
	for _, tc := range []struct {
		name           string
		extra          string
		wantInstead    int
		wantCandidates []string
	}{
		{name: "an alternative that names a public package", wantInstead: 1, wantCandidates: []string{"internal/store/api"},
			extra: `    alternatives:
      - "Call the store through internal/store/api, then the impl stays private"
`},
		{name: "an alternative that names nothing the policy owns", wantInstead: 1,
			extra: `    alternatives:
      - "Ask the store team for a query helper in third_party/queries"
`},
		{name: "a rule without alternatives", wantInstead: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := canImportCandidates(t, candidatesRepo(t, deny+tc.extra))
			a := got.Answers[0]
			if a.Answer != answerDenied || len(a.Instead) != tc.wantInstead {
				t.Fatalf("answer %q, instead %v, want denied with %d", a.Answer, a.Instead, tc.wantInstead)
			}
			var paths []string
			for _, c := range a.Candidates {
				paths = append(paths, c.Path)
				if c.Basis != "owner_alternative" || c.PolicyCheck != "allowed" || c.Module != "store" || !c.PublicAPI {
					t.Errorf("candidate = %+v, want an allowed public owner_alternative of module store", c)
				}
			}
			if strings.Join(paths, ",") != strings.Join(tc.wantCandidates, ",") {
				t.Errorf("candidates = %v, want %v", paths, tc.wantCandidates)
			}
			if (len(a.Candidates) > 0) != (a.CandidatesNote != "") {
				t.Errorf("candidates %d but note %q: the note travels with the candidates", len(a.Candidates), a.CandidatesNote)
			}
		})
	}
}

// TestCanImportJudgesEachCandidate: a declared alternative is advice, not
// permission. A candidate the rule pass denies is marked denied.
func TestCanImportJudgesEachCandidate(t *testing.T) {
	t.Parallel()
	const rules = `  - id: app_no_store_impl
    type: forbidden_dependency
    from: internal/app/**
    to: internal/store/impl/**
    gate: fail
    alternatives:
      - "Use internal/other instead"
  - id: app_no_other
    type: forbidden_dependency
    from: internal/app/**
    to: internal/other/**
    gate: fail
`
	got := canImportCandidates(t, candidatesRepo(t, rules))
	cs := got.Answers[0].Candidates
	if len(cs) != 1 || cs[0].Path != "internal/other" || cs[0].PolicyCheck != answerDenied || cs[0].PublicAPI {
		t.Fatalf("candidates = %+v, want internal/other marked denied", cs)
	}
}
