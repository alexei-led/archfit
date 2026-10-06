package main

import (
	"encoding/json"
	"strings"
	"testing"

	reporttest "github.com/alexei-led/archfit/internal/testutil/report"
)

const (
	ruleIDDomainNoHTTP = "domain_no_http"
	selectorRationale  = "Domain code stays free of I/O"
	selectorAlt        = "Depend on a port in the application layer"
	selectorDocs       = "docs/adr/003.md"
	selectorConfig     = `version: 2
layers: [domain, app]
modules:
  billing:
    paths: ["internal/billing/**"]
    layer: domain
    owner: team-b
  shipping:
    paths: ["internal/shipping/**"]
    layer: app
    owner: team-c
rules:
  - id: domain_no_http
    type: forbidden_dependency
    from_module: "layer:domain"
    to: net/http
    rationale: "Domain code stays free of I/O"
    alternatives: ["Depend on a port in the application layer"]
    docs: docs/adr/003.md
    gate: fail
`
	srcBillingHTTP = "package api\n\nimport \"net/http\"\n\nfunc Bill() int { return http.StatusOK }\n"
)

// selectorRepo writes a Go repository whose domain module imports net/http.
func selectorRepo(t *testing.T, config string) string {
	t.Helper()
	return writeRuleFixtureRepo(t, map[string]string{
		markerGoMod:        fixtureShopGoMod,
		fileBillingAPIGo:   srcBillingHTTP,
		fileShippingShipGo: goFileImportingBilling("app"),
		defaultConfigPath:  config,
	})
}

// TestRun_Check_ModuleSelectorCarriesTheRationale pins item 2.2 end to end: a
// layer selector blocks the domain module's import of net/http, and the rule's
// rationale, alternatives, and docs reach the finding, the repair task, and
// SARIF; a waiver on the module and the target releases it.
func TestRun_Check_ModuleSelectorCarriesTheRationale(t *testing.T) {
	t.Parallel()
	cfgPath := selectorRepo(t, selectorConfig)
	code, state := checkStateOf(t, cfgPath)
	if code != 1 {
		t.Fatalf("check exit = %d, want 1", code)
	}
	got := findingsOf(state, ruleIDDomainNoHTTP)
	if len(got) != 1 {
		t.Fatalf("findings = %+v, want one", got)
	}
	f := got[0]
	if f.Edge.From.Module != fixtureModBilling || f.Edge.To.Path != "net/http" || f.Edge.Kind != "module_dependency" {
		t.Errorf("edge = %+v, want module billing -> net/http", f.Edge)
	}
	if !strings.HasSuffix(f.Why, " — "+selectorRationale) || !strings.HasSuffix(f.Constraint, "(see "+selectorDocs+")") ||
		len(f.Alternatives) != 1 || f.Alternatives[0] != selectorAlt {
		t.Errorf("finding why/constraint/alternatives = %q / %q / %v", f.Why, f.Constraint, f.Alternatives)
	}
	var task []string
	for _, tk := range state.AgentTasks {
		if tk.FindingID == f.ID {
			task = tk.Constraints
		}
	}
	joined := strings.Join(task, "\n")
	for _, want := range []string{"rationale: " + selectorRationale, "allowed alternative: " + selectorAlt, selectorDocs} {
		if !strings.Contains(joined, want) {
			t.Errorf("task constraints %q miss %q", task, want)
		}
	}

	_, stdout, stderr := runArchfit(t, cmdCheck, "-c", cfgPath, "--format=sarif")
	var sarif struct {
		Runs []struct {
			Results []struct {
				RuleID  string `json:"ruleId"`
				Message struct {
					Text string `json:"text"`
				} `json:"message"`
				Properties map[string]any `json:"properties"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(stdout), &sarif); err != nil || len(sarif.Runs) != 1 {
		t.Fatalf("decode sarif: %v\nstderr:\n%s", err, stderr)
	}
	found := false
	for _, r := range sarif.Runs[0].Results {
		if r.RuleID == ruleIDDomainNoHTTP {
			found = true
			if !strings.Contains(r.Message.Text, selectorRationale) || r.Properties["allowed_alternatives"] == nil {
				t.Errorf("sarif result = %+v, want the rationale and the alternatives", r)
			}
		}
	}
	if !found {
		t.Error("sarif carries no domain_no_http result")
	}

	waived := selectorConfig + `waivers:
  - rule: domain_no_http
    from: billing
    to: net/http
    reason: staged migration
    approved_by: architecture-owner
    expires: "2099-01-01"
`
	if code, _ := checkStateOf(t, selectorRepo(t, waived)); code == 1 {
		t.Errorf("check exit = 1 with a waiver on billing -> net/http, want it released")
	}
}

// TestRun_Check_LongRationaleStaysWithinTheConsumerTextRules pins that a
// rationale longer than the App's free-text cap, with a control character,
// still yields a report the strict consumer accepts, and does not move the
// finding ID.
func TestRun_Check_LongRationaleStaysWithinTheConsumerTextRules(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("Domain code stays free of I/O. ", 25) + "\\u2028end"
	config := strings.Replace(selectorConfig, `rationale: "`+selectorRationale+`"`, `rationale: "`+long+`"`, 1)
	_, base := checkStateOf(t, selectorRepo(t, selectorConfig))
	code, stdout, stderr := runArchfit(t, cmdCheck, "-c", selectorRepo(t, config), "--format=json")
	if code != 1 {
		t.Fatalf("check exit = %d, want 1\nstderr:\n%s", code, stderr)
	}
	violations, err := reporttest.AppTextViolations([]byte(stdout))
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("published state breaks the consumer string rules:\n%s", strings.Join(violations, "\n"))
	}
	var state struct {
		Findings []struct {
			ID     string `json:"id"`
			RuleID string `json:"rule_id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(stdout), &state); err != nil {
		t.Fatal(err)
	}
	baseIDs := findingsOf(base, ruleIDDomainNoHTTP)
	for _, f := range state.Findings {
		if f.RuleID == ruleIDDomainNoHTTP && (len(baseIDs) != 1 || f.ID != baseIDs[0].ID) {
			t.Errorf("finding ID %s moved with the rationale text, want %v", f.ID, baseIDs)
		}
	}
}
