package astgrep_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alexei-led/archfit/internal/extract/astgrep"
	"github.com/alexei-led/archfit/internal/scope"
)

// syntaxEntry builds a minimal sgSyntaxMatch-shaped JSON object for use in
// canned Syntax() test fixtures. Fields match sg 0.44 --json=compact output.
func syntaxEntry(ruleID, file, name string, startLine, endLine int) map[string]any {
	e := map[string]any{
		jsonKeyText: name,
		"ruleId":    ruleID,
		"file":      file,
		"range": map[string]any{
			"start": map[string]any{jsonKeyLine: startLine, jsonKeyColumn: 0},
			"end":   map[string]any{jsonKeyLine: endLine, jsonKeyColumn: 0},
		},
		"metaVariables": map[string]any{
			jsonKeySingle: map[string]any{},
			jsonKeyMulti:  map[string]any{},
		},
	}
	return e
}

// syntaxEntryWithName adds a NAME metavariable (exported declaration rules).
func syntaxEntryWithName(ruleID, file, name string, startLine, endLine int) map[string]any {
	e := syntaxEntry(ruleID, file, name, startLine, endLine)
	e["metaVariables"] = map[string]any{
		"single": map[string]any{
			"NAME": map[string]any{"text": name},
		},
		"multi": map[string]any{},
	}
	return e
}

// syntaxEntryWithPath adds a PATH metavariable (route rules).
func syntaxEntryWithPath(ruleID, file, path string, startLine int) map[string]any {
	e := syntaxEntry(ruleID, file, path, startLine, startLine)
	e["metaVariables"] = map[string]any{
		"single": map[string]any{
			"PATH": map[string]any{"text": `"` + path + `"`},
		},
		"multi": map[string]any{},
	}
	return e
}

func marshalSyntaxEntries(t *testing.T, entries []map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal entries: %v", err)
	}
	return b
}

var syntaxScope = scope.Scope{Root: "/repo", Mode: scope.ModeFull}

func TestSyntax_AbsentTool_ReturnsAbsentCoverageNoError(t *testing.T) {
	a := astgrep.New(absentRunner())

	facts, cov, err := a.Syntax(context.Background(), syntaxScope, []string{"go"})
	if err != nil {
		t.Fatalf("Syntax with absent sg must not error: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 facts for absent tool, got %d", len(facts))
	}
	if cov.Status != "absent" {
		t.Errorf("cov.Status = %q, want %q", cov.Status, "absent")
	}
	if cov.Tool != "ast-grep" {
		t.Errorf("cov.Tool = %q, want %q", cov.Tool, "ast-grep")
	}
}

func TestSyntax_Go_Kinds(t *testing.T) {
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("go-func", "pkg/svc/svc.go", "NewService", 10, 15),
		syntaxEntryWithName("go-method", "pkg/svc/svc.go", "Save", 20, 22),
		syntaxEntryWithName("go-struct", "pkg/svc/svc.go", "Service", 5, 8),
		syntaxEntryWithName("go-interface", "pkg/repo/repo.go", "Repository", 3, 7),
	})

	a := astgrep.New(presentRunner(entries))
	facts, cov, err := a.Syntax(context.Background(), syntaxScope, []string{"go"})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if cov.Status != "ok" {
		t.Errorf("cov.Status = %q, want ok", cov.Status)
	}
	if len(facts) != 4 {
		t.Fatalf("len(facts) = %d, want 4", len(facts))
	}

	// Sorted by (File, StartLine, Kind, Name):
	// pkg/repo/repo.go:3 interface Repository
	// pkg/svc/svc.go:5 struct Service
	// pkg/svc/svc.go:10 function NewService
	// pkg/svc/svc.go:20 method Save
	cases := []struct {
		file      string
		kind      string
		name      string
		startLine int
		exported  bool
	}{
		{"pkg/repo/repo.go", "interface", "Repository", 4, true}, // 3+1
		{fileSvcGo, "struct", "Service", 6, true},                // 5+1
		{fileSvcGo, "function", "NewService", 11, true},          // 10+1
		{fileSvcGo, "method", "Save", 21, true},                  // 20+1
	}
	for i, tc := range cases {
		f := facts[i]
		if f.File != tc.file {
			t.Errorf("facts[%d].File = %q, want %q", i, f.File, tc.file)
		}
		if f.Kind != tc.kind {
			t.Errorf("facts[%d].Kind = %q, want %q", i, f.Kind, tc.kind)
		}
		if f.Name != tc.name {
			t.Errorf("facts[%d].Name = %q, want %q", i, f.Name, tc.name)
		}
		if f.StartLine != tc.startLine {
			t.Errorf("facts[%d].StartLine = %d, want %d", i, f.StartLine, tc.startLine)
		}
		if f.Exported != tc.exported {
			t.Errorf("facts[%d].Exported = %v, want %v", i, f.Exported, tc.exported)
		}
		if f.Language != "go" {
			t.Errorf("facts[%d].Language = %q, want go", i, f.Language)
		}
	}
}

func TestSyntax_Go_Exported_Detection(t *testing.T) {
	// Mix of exported (uppercase) and unexported (lowercase) names.
	// The ruleIds for func/method already filter by regex in the YAML, but the
	// adapter also derives Exported from the name — verify the flag is correct.
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("go-func", "pkg/a/a.go", "Exported", 1, 1),
		syntaxEntryWithName("go-method", "pkg/a/a.go", "unexported", 2, 2),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{"go"})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("len(facts) = %d, want 2", len(facts))
	}
	if !facts[0].Exported {
		t.Errorf("facts[0] (Exported) should be exported")
	}
	if facts[1].Exported {
		t.Errorf("facts[1] (unexported) should not be exported")
	}
}

func TestSyntax_Go_TypeAlias(t *testing.T) {
	// go-type-alias has no NAME metavar; name comes from text "type ID = int".
	entries := []map[string]any{
		syntaxEntry("go-type-alias", "pkg/model/model.go", "type ID = int", 5, 5),
	}
	b := marshalSyntaxEntries(t, entries)

	a := astgrep.New(presentRunner(b))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{"go"})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("len(facts) = %d, want 1", len(facts))
	}
	if facts[0].Name != "ID" {
		t.Errorf("facts[0].Name = %q, want ID", facts[0].Name)
	}
	if facts[0].Kind != "type_alias" {
		t.Errorf("facts[0].Kind = %q, want type_alias", facts[0].Kind)
	}
	if !facts[0].Exported {
		t.Errorf("facts[0].Exported should be true for uppercase ID")
	}
}

func TestSyntax_Go_RouteAndFramework(t *testing.T) {
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithPath("go-route-gin", "internal/api/router.go", "/users", 10),
		syntaxEntryWithPath("go-route-net-http", "internal/api/router.go", "/health", 20),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{"go"})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("len(facts) = %d, want 2", len(facts))
	}

	// sorted by StartLine: /health (20+1=21) < /users (10+1=11) — wait, /users is line 10
	// sorted: StartLine 11 (/users, gin) then 21 (/health, net/http)
	gin := facts[0]
	if gin.Kind != kindRouteStr {
		t.Errorf("facts[0].Kind = %q, want route", gin.Kind)
	}
	if gin.Framework != "gin" {
		t.Errorf("facts[0].Framework = %q, want gin", gin.Framework)
	}
	if gin.Name != "/users" {
		t.Errorf("facts[0].Name = %q, want /users", gin.Name)
	}
	if gin.Exported {
		t.Errorf("facts[0].Exported should be false for routes")
	}

	http := facts[1]
	if http.Framework != "net/http" {
		t.Errorf("facts[1].Framework = %q, want net/http", http.Framework)
	}
}

func TestSyntax_UnknownLanguage_Skipped(t *testing.T) {
	// A language with no embedded rules must not error and return empty facts.
	// Use "cobol" — never a supported language in archfit.
	a := astgrep.New(presentRunner([]byte("[]")))
	facts, cov, err := a.Syntax(context.Background(), syntaxScope, []string{"cobol"})
	if err != nil {
		t.Fatalf("Syntax with unknown lang must not error: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 facts for lang with no rules, got %d", len(facts))
	}
	// Runner was never called for a lang with no rules, but the tool was detected
	// as present (presentRunner), so status is ok.
	if cov.Status != "ok" {
		t.Errorf("cov.Status = %q, want ok", cov.Status)
	}
}

func TestSyntax_MalformedJSON_ReturnsError(t *testing.T) {
	bad := []byte(`not-json`)
	a := astgrep.New(presentRunner(bad))
	_, _, err := a.Syntax(context.Background(), syntaxScope, []string{"go"})
	if err == nil {
		t.Fatal("expected error for malformed JSON output, got nil")
	}
}

func TestSyntax_SortedOutput(t *testing.T) {
	// Provide facts in reverse file+line order; output must be sorted.
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("go-func", "pkg/z/z.go", "ZFunc", 5, 5),
		syntaxEntryWithName("go-func", "pkg/a/a.go", "AFunc", 10, 10),
		syntaxEntryWithName("go-func", "pkg/a/a.go", "BFunc", 3, 3),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{"go"})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 3 {
		t.Fatalf("len(facts) = %d, want 3", len(facts))
	}
	// Expected order: pkg/a/a.go:4(BFunc), pkg/a/a.go:11(AFunc), pkg/z/z.go:6(ZFunc)
	if facts[0].Name != "BFunc" {
		t.Errorf("facts[0].Name = %q, want BFunc", facts[0].Name)
	}
	if facts[1].Name != "AFunc" {
		t.Errorf("facts[1].Name = %q, want AFunc", facts[1].Name)
	}
	if facts[2].Name != "ZFunc" {
		t.Errorf("facts[2].Name = %q, want ZFunc", facts[2].Name)
	}
}

// --- TypeScript adapter tests ---

const fileSvcTS = "src/svc/service.ts"

func TestSyntax_TS_Kinds(t *testing.T) {
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("ts-func", fileSvcTS, "createUser", 5, 10),
		syntaxEntryWithName("ts-class", fileSvcTS, "UserService", 15, 40),
		syntaxEntryWithName("ts-interface", "src/svc/types.ts", "UserRepository", 3, 8),
		syntaxEntryWithName("ts-enum", "src/svc/types.ts", "UserRole", 10, 14),
		syntaxEntryWithName("ts-type-alias", "src/svc/types.ts", "UserId", 16, 16),
		syntaxEntryWithName("ts-method", fileSvcTS, "save", 20, 22),
	})

	a := astgrep.New(presentRunner(entries))
	facts, cov, err := a.Syntax(context.Background(), syntaxScope, []string{langTypeScriptStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if cov.Status != "ok" {
		t.Errorf("cov.Status = %q, want ok", cov.Status)
	}
	if len(facts) != 6 {
		t.Fatalf("len(facts) = %d, want 6", len(facts))
	}
	// All declaration facts must have Language=langTypeScriptStr and Exported=true.
	for i, f := range facts {
		if f.Language != langTypeScriptStr {
			t.Errorf("facts[%d].Language = %q, want typescript", i, f.Language)
		}
		if !f.Exported {
			t.Errorf("facts[%d] (%s %s) should be exported", i, f.Kind, f.Name)
		}
	}
}

func TestSyntax_TS_ClassAndInterface_Kinds(t *testing.T) {
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("ts-class", fileSvcTS, "AuthService", 1, 30),
		syntaxEntryWithName("ts-interface", fileSvcTS, "AuthPort", 32, 36),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langTypeScriptStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("len(facts) = %d, want 2", len(facts))
	}
	// Sorted: AuthService (line 1) < AuthPort (line 32) — same file, StartLine order.
	if facts[0].Kind != "class" || facts[0].Name != "AuthService" {
		t.Errorf("facts[0] = {%s %s}, want {class AuthService}", facts[0].Kind, facts[0].Name)
	}
	if facts[1].Kind != "interface" || facts[1].Name != "AuthPort" {
		t.Errorf("facts[1] = {%s %s}, want {interface AuthPort}", facts[1].Kind, facts[1].Name)
	}
}

func TestSyntax_TS_Decorator_NotExported(t *testing.T) {
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("ts-decorator", fileSvcTS, "Injectable", 1, 1),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langTypeScriptStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("len(facts) = %d, want 1", len(facts))
	}
	f := facts[0]
	if f.Kind != "annotation" {
		t.Errorf("Kind = %q, want annotation", f.Kind)
	}
	if f.Name != "Injectable" {
		t.Errorf("Name = %q, want Injectable", f.Name)
	}
	if f.Exported {
		t.Errorf("decorator fact should not be exported")
	}
	if f.Framework != "" {
		t.Errorf("decorator fact should have no framework, got %q", f.Framework)
	}
}

func TestSyntax_TS_Route_Express(t *testing.T) {
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithPath("ts-route-express", "src/api/router.ts", "/users", 10),
		syntaxEntryWithPath("ts-route-express", "src/api/router.ts", "/health", 20),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langTypeScriptStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("len(facts) = %d, want 2", len(facts))
	}
	for i, f := range facts {
		if f.Kind != kindRouteStr {
			t.Errorf("facts[%d].Kind = %q, want route", i, f.Kind)
		}
		if f.Framework != "express" {
			t.Errorf("facts[%d].Framework = %q, want express", i, f.Framework)
		}
		if f.Exported {
			t.Errorf("facts[%d] route should not be exported", i)
		}
	}
	// Sorted by StartLine: /users(11) < /health(21)
	if facts[0].Name != "/users" {
		t.Errorf("facts[0].Name = %q, want /users", facts[0].Name)
	}
	if facts[1].Name != "/health" {
		t.Errorf("facts[1].Name = %q, want /health", facts[1].Name)
	}
}

func TestSyntax_TS_Route_Nest(t *testing.T) {
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithPath("ts-route-nest-controller", "src/api/user.controller.ts", "/users", 5),
		syntaxEntryWithPath("ts-route-nest-method", "src/api/user.controller.ts", "/", 10),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langTypeScriptStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("len(facts) = %d, want 2", len(facts))
	}
	for i, f := range facts {
		if f.Kind != kindRouteStr {
			t.Errorf("facts[%d].Kind = %q, want route", i, f.Kind)
		}
		if f.Framework != "nest" {
			t.Errorf("facts[%d].Framework = %q, want nest", i, f.Framework)
		}
		if f.Exported {
			t.Errorf("facts[%d] nest route should not be exported", i)
		}
	}
}
