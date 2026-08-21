package astgrep_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/alexei-led/archfit/internal/extract/astgrep"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
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
	if cov.Status != statusAbsentStr {
		t.Errorf("cov.Status = %q, want %q", cov.Status, statusAbsentStr)
	}
	// The syntax pass reports under its OWN coverage name. Sharing the pattern
	// pass's name would put two rows under one tool and make every row-pairing
	// consumer read an unpairable duplicate.
	if cov.Tool != toolAstGrepSyntax {
		t.Errorf("cov.Tool = %q, want %q", cov.Tool, toolAstGrepSyntax)
	}
}

func TestSyntax_DrainsStreamToEOF(t *testing.T) {
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("go-func", "pkg/svc/svc.go", "NewService", 10, 15),
	})
	output := append([]byte{}, entries...)
	output = append(output, ' ', '\n', '\t')
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: "sg", Path: "/usr/bin/sg"}, true
		},
		StreamFunc: func(_ context.Context, _ toolrun.ToolCmd, consume func(io.Reader) error) (toolrun.Output, error) {
			r := bytes.NewReader(output)
			if err := consume(r); err != nil {
				return toolrun.Output{}, err
			}
			if r.Len() != 0 {
				return toolrun.Output{}, fmt.Errorf("consumer left %d bytes unread", r.Len())
			}
			return toolrun.Output{}, nil
		},
	}
	a := astgrep.New(runner)

	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{"go"})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("len(facts) = %d, want 1", len(facts))
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
		{"pkg/repo/repo.go", kindInterfaceStr, "Repository", 4, true}, // 3+1
		{fileSvcGo, kindStructStr, "Service", 6, true},                // 5+1
		{fileSvcGo, kindFunctionStr, "NewService", 11, true},          // 10+1
		{fileSvcGo, kindMethodStr, "Save", 21, true},                  // 20+1
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
	const fileRouter = "internal/api/router.go"
	entries := marshalSyntaxEntries(t, []map[string]any{
		// Import signals: confirm gin and net/http are imported in this file.
		syntaxEntry("go-import-gin", fileRouter, `"github.com/gin-gonic/gin"`, 1, 1),
		syntaxEntry("go-import-net-http", fileRouter, `"net/http"`, 2, 2),
		// Route facts: will be emitted because imports are confirmed above.
		syntaxEntryWithPath("go-route-gin", fileRouter, pathUsers, 10),
		syntaxEntryWithPath("go-route-net-http", fileRouter, "/health", 20),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{"go"})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("len(facts) = %d, want 2 (signal entries must not appear as facts)", len(facts))
	}

	// sorted by StartLine: /users(11) then /health(21)
	gin := facts[0]
	if gin.Kind != kindRouteStr {
		t.Errorf("facts[0].Kind = %q, want route", gin.Kind)
	}
	if gin.Framework != "gin" {
		t.Errorf("facts[0].Framework = %q, want gin", gin.Framework)
	}
	if gin.Name != pathUsers {
		t.Errorf("facts[0].Name = %q, want /users", gin.Name)
	}
	if gin.Exported {
		t.Errorf("facts[0].Exported should be false for routes")
	}
	if !gin.FrameworkConfirmed {
		t.Errorf("facts[0].FrameworkConfirmed should be true for confirmed route")
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
	if facts[0].Kind != kindClassStr || facts[0].Name != "AuthService" {
		t.Errorf("facts[0] = {%s %s}, want {class AuthService}", facts[0].Kind, facts[0].Name)
	}
	if facts[1].Kind != kindInterfaceStr || facts[1].Name != "AuthPort" {
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
	if f.Kind != kindAnnotStr {
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
	const fileRouter = "src/api/router.ts"
	entries := marshalSyntaxEntries(t, []map[string]any{
		// Import signal: confirm express is imported in this file.
		syntaxEntry("ts-import-express", fileRouter, "express", 1, 1),
		// Route facts: emitted because import is confirmed above.
		syntaxEntryWithPath("ts-route-express", fileRouter, pathUsers, 10),
		syntaxEntryWithPath("ts-route-express", fileRouter, "/health", 20),
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
	if facts[0].Name != pathUsers {
		t.Errorf("facts[0].Name = %q, want /users", facts[0].Name)
	}
	if facts[1].Name != "/health" {
		t.Errorf("facts[1].Name = %q, want /health", facts[1].Name)
	}
}

func TestSyntax_TS_Route_Nest(t *testing.T) {
	const fileController = "src/api/user.controller.ts"
	entries := marshalSyntaxEntries(t, []map[string]any{
		// Import signal: confirm @nestjs/common is imported in this file.
		syntaxEntry("ts-import-nest", fileController, "@nestjs/common", 1, 1),
		// Route facts: emitted because import is confirmed above.
		syntaxEntryWithPath("ts-route-nest-controller", fileController, pathUsers, 5),
		syntaxEntryWithPath("ts-route-nest-method", fileController, "/", 10),
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

// --- Python adapter tests ---

const fileSvcPy = "pkg/svc/service.py"

func TestSyntax_Py_Kinds(t *testing.T) {
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("py-func", fileSvcPy, nameProcess, 5, 8),
		syntaxEntryWithName("py-class", fileSvcPy, "UserService", 10, 30),
	})

	a := astgrep.New(presentRunner(entries))
	facts, cov, err := a.Syntax(context.Background(), syntaxScope, []string{langPythonStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if cov.Status != "ok" {
		t.Errorf("cov.Status = %q, want ok", cov.Status)
	}
	if len(facts) != 2 {
		t.Fatalf("len(facts) = %d, want 2", len(facts))
	}
	// Sorted by (File, StartLine): process(6) < UserService(11).
	if facts[0].Kind != kindFunctionStr || facts[0].Name != nameProcess {
		t.Errorf("facts[0] = {%s %s}, want {function process}", facts[0].Kind, facts[0].Name)
	}
	if facts[1].Kind != "class" || facts[1].Name != "UserService" {
		t.Errorf("facts[1] = {%s %s}, want {class UserService}", facts[1].Kind, facts[1].Name)
	}
	for i, f := range facts {
		if f.Language != langPythonStr {
			t.Errorf("facts[%d].Language = %q, want python", i, f.Language)
		}
		if !f.Exported {
			t.Errorf("facts[%d] (%s %s) should be exported (public)", i, f.Kind, f.Name)
		}
	}
}

func TestSyntax_Py_PublicDetection(t *testing.T) {
	// Public names (no leading underscore) → Exported=true.
	// Private names (leading underscore) → Exported=false.
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("py-func", fileSvcPy, nameProcess, 1, 3),
		syntaxEntryWithName("py-func", fileSvcPy, "_internal", 5, 7),
		syntaxEntryWithName("py-class", fileSvcPy, "UserService", 10, 20),
		syntaxEntryWithName("py-class", fileSvcPy, "_Base", 22, 25),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langPythonStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 4 {
		t.Fatalf("len(facts) = %d, want 4", len(facts))
	}
	// Sorted by StartLine: process(2), _internal(6), UserService(11), _Base(23).
	cases := []struct {
		name     string
		exported bool
	}{
		{nameProcess, true},
		{"_internal", false},
		{"UserService", true},
		{"_Base", false},
	}
	for i, tc := range cases {
		if facts[i].Name != tc.name {
			t.Errorf("facts[%d].Name = %q, want %q", i, facts[i].Name, tc.name)
		}
		if facts[i].Exported != tc.exported {
			t.Errorf("facts[%d] (%s) Exported = %v, want %v", i, tc.name, facts[i].Exported, tc.exported)
		}
	}
}

func TestSyntax_Py_Decorator_NotExported(t *testing.T) {
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("py-decorator", fileSvcPy, "dataclass", 1, 1),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langPythonStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("len(facts) = %d, want 1", len(facts))
	}
	f := facts[0]
	if f.Kind != kindAnnotStr {
		t.Errorf("Kind = %q, want annotation", f.Kind)
	}
	if f.Name != "dataclass" {
		t.Errorf("Name = %q, want dataclass", f.Name)
	}
	if f.Exported {
		t.Errorf("decorator fact should not be exported")
	}
	if f.Framework != "" {
		t.Errorf("decorator fact should have no framework, got %q", f.Framework)
	}
	if f.Language != langPythonStr {
		t.Errorf("Language = %q, want python", f.Language)
	}
}

func TestSyntax_Py_Route_Decorator(t *testing.T) {
	// fastapi/flask/starlette/aiohttp all share the same @recv.verb(path) shape.
	// Labelled "fastapi" as the canonical representative.
	const fileRoutes = "src/api/routes.py"
	entries := marshalSyntaxEntries(t, []map[string]any{
		// Import signal: confirm fastapi is imported in this file.
		syntaxEntry("py-import-fastapi", fileRoutes, "fastapi", 1, 1),
		// Route facts: emitted because import is confirmed above.
		syntaxEntryWithPath("py-route-decorator", fileRoutes, pathUsers, 5),
		syntaxEntryWithPath("py-route-decorator", fileRoutes, "/items", 10),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langPythonStr})
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
		if f.Framework != "fastapi" {
			t.Errorf("facts[%d].Framework = %q, want fastapi", i, f.Framework)
		}
		if f.Exported {
			t.Errorf("facts[%d] route should not be exported", i)
		}
		if f.Language != langPythonStr {
			t.Errorf("facts[%d].Language = %q, want python", i, f.Language)
		}
	}
	// Sorted by StartLine: /users(6) < /items(11).
	if facts[0].Name != pathUsers {
		t.Errorf("facts[0].Name = %q, want /users", facts[0].Name)
	}
	if facts[1].Name != "/items" {
		t.Errorf("facts[1].Name = %q, want /items", facts[1].Name)
	}
}

func TestSyntax_Py_Route_Django(t *testing.T) {
	const fileURLs = "myapp/urls.py"
	entries := marshalSyntaxEntries(t, []map[string]any{
		// Import signal: confirm django.urls is imported in this file.
		syntaxEntry("py-import-django", fileURLs, "django", 1, 1),
		// Route facts: emitted because import is confirmed above.
		syntaxEntryWithPath("py-route-django-path", fileURLs, "/users/", 3),
		syntaxEntryWithPath("py-route-django-repath", fileURLs, `^/items/\d+/$`, 4),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langPythonStr})
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
		if f.Framework != "django" {
			t.Errorf("facts[%d].Framework = %q, want django", i, f.Framework)
		}
		if f.Exported {
			t.Errorf("facts[%d] django route should not be exported", i)
		}
		if f.Language != langPythonStr {
			t.Errorf("facts[%d].Language = %q, want python", i, f.Language)
		}
	}
	// Sorted by StartLine: /users/(4) < ^/items/\d+/$(5).
	if facts[0].Name != "/users/" {
		t.Errorf("facts[0].Name = %q, want /users/", facts[0].Name)
	}
}

// --- Rust adapter tests ---

const fileSvcRs = "src/svc/service.rs"

func TestSyntax_Rust_Kinds(t *testing.T) {
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("rs-func", fileSvcRs, "new_service", 5, 10),
		syntaxEntryWithName("rs-struct", fileSvcRs, "Service", 15, 20),
		syntaxEntryWithName("rs-enum", fileSvcRs, "Status", 22, 26),
		syntaxEntryWithName("rs-trait", "src/svc/port.rs", "Repository", 3, 8),
		syntaxEntryWithName("rs-impl", fileSvcRs, "save", 30, 35),
		syntaxEntryWithName("rs-mod", fileSvcRs, "handlers", 40, 50),
	})

	a := astgrep.New(presentRunner(entries))
	facts, cov, err := a.Syntax(context.Background(), syntaxScope, []string{langRustStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if cov.Status != "ok" {
		t.Errorf("cov.Status = %q, want ok", cov.Status)
	}
	if len(facts) != 6 {
		t.Fatalf("len(facts) = %d, want 6", len(facts))
	}
	// All declaration facts must have Language=rust and Exported=true (pub filtered by YAML).
	for i, f := range facts {
		if f.Language != langRustStr {
			t.Errorf("facts[%d].Language = %q, want rust", i, f.Language)
		}
		if !f.Exported {
			t.Errorf("facts[%d] (%s %s) should be exported (pub)", i, f.Kind, f.Name)
		}
	}
}

func TestSyntax_Rust_KindMapping(t *testing.T) {
	// Verify each ruleId maps to the correct Kind.
	cases := []struct {
		ruleID string
		name   string
		want   string
	}{
		{"rs-func", nameProcess, "function"},
		{"rs-struct", "Config", "struct"},
		{"rs-enum", "Color", "enum"},
		{"rs-trait", "Storage", "interface"},
		{"rs-impl", "handle", "method"},
		{"rs-mod", "api", "struct"},
	}

	for _, tc := range cases {
		entries := marshalSyntaxEntries(t, []map[string]any{
			syntaxEntryWithName(tc.ruleID, fileSvcRs, tc.name, 1, 5),
		})
		a := astgrep.New(presentRunner(entries))
		facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langRustStr})
		if err != nil {
			t.Fatalf("ruleID=%s Syntax: %v", tc.ruleID, err)
		}
		if len(facts) != 1 {
			t.Fatalf("ruleID=%s len(facts) = %d, want 1", tc.ruleID, len(facts))
		}
		if facts[0].Kind != tc.want {
			t.Errorf("ruleID=%s Kind = %q, want %q", tc.ruleID, facts[0].Kind, tc.want)
		}
	}
}

func TestSyntax_Rust_PubExported(t *testing.T) {
	// The YAML rule already requires visibility_modifier; isExported always returns
	// true for declaration ruleIds. Verify the Exported flag is set correctly.
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("rs-func", fileSvcRs, "public_fn", 1, 3),
		syntaxEntryWithName("rs-struct", fileSvcRs, "PubStruct", 5, 8),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langRustStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("len(facts) = %d, want 2", len(facts))
	}
	for i, f := range facts {
		if !f.Exported {
			t.Errorf("facts[%d] (%s %s) should be exported", i, f.Kind, f.Name)
		}
	}
}

func TestSyntax_Rust_Attribute_NotExported(t *testing.T) {
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("rs-attribute", fileSvcRs, "derive", 1, 1),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langRustStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("len(facts) = %d, want 1", len(facts))
	}
	f := facts[0]
	if f.Kind != kindAnnotStr {
		t.Errorf("Kind = %q, want annotation", f.Kind)
	}
	if f.Name != "derive" {
		t.Errorf("Name = %q, want derive", f.Name)
	}
	if f.Exported {
		t.Errorf("attribute fact should not be exported")
	}
	if f.Framework != "" {
		t.Errorf("attribute fact should have no framework, got %q", f.Framework)
	}
	if f.Language != langRustStr {
		t.Errorf("Language = %q, want rust", f.Language)
	}
}

func TestSyntax_Rust_Route_Actix(t *testing.T) {
	// actix/rocket share the same #[get(...)]/etc. attribute shape; labelled "actix".
	const fileHandlers = "src/api/handlers.rs"
	entries := marshalSyntaxEntries(t, []map[string]any{
		// Import signal: confirm actix_web is imported in this file.
		syntaxEntry("rs-import-actix", fileHandlers, "actix_web", 1, 1),
		// Route facts: emitted because import is confirmed above.
		syntaxEntryWithPath("rs-route-actix-rocket", fileHandlers, pathUsers, 10),
		syntaxEntryWithPath("rs-route-actix-rocket", fileHandlers, "/health", 20),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langRustStr})
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
		if f.Framework != "actix" {
			t.Errorf("facts[%d].Framework = %q, want actix", i, f.Framework)
		}
		if f.Exported {
			t.Errorf("facts[%d] route should not be exported", i)
		}
		if f.Language != langRustStr {
			t.Errorf("facts[%d].Language = %q, want rust", i, f.Language)
		}
	}
	// Sorted by StartLine: /users(11) < /health(21).
	if facts[0].Name != pathUsers {
		t.Errorf("facts[0].Name = %q, want /users", facts[0].Name)
	}
	if facts[1].Name != "/health" {
		t.Errorf("facts[1].Name = %q, want /health", facts[1].Name)
	}
}

func TestSyntax_Rust_Route_Axum(t *testing.T) {
	// axum/warp share the .route(path, handler) builder shape; labelled "axum".
	const fileRouter = "src/api/router.rs"
	entries := marshalSyntaxEntries(t, []map[string]any{
		// Import signal: confirm axum is imported in this file.
		syntaxEntry("rs-import-axum", fileRouter, "axum", 1, 1),
		// Route facts: emitted because import is confirmed above.
		syntaxEntryWithPath("rs-route-axum-warp", fileRouter, pathUsers, 5),
		syntaxEntryWithPath("rs-route-axum-warp", fileRouter, "/items", 8),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langRustStr})
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
		if f.Framework != "axum" {
			t.Errorf("facts[%d].Framework = %q, want axum", i, f.Framework)
		}
		if f.Exported {
			t.Errorf("facts[%d] route should not be exported", i)
		}
		if f.Language != langRustStr {
			t.Errorf("facts[%d].Language = %q, want rust", i, f.Language)
		}
	}
	// Sorted by StartLine: /users(6) < /items(9).
	if facts[0].Name != pathUsers {
		t.Errorf("facts[0].Name = %q, want /users", facts[0].Name)
	}
	if facts[1].Name != "/items" {
		t.Errorf("facts[1].Name = %q, want /items", facts[1].Name)
	}
}

// --- Import-gating false-positive regression tests ---
// Each test reproduces a validated FP from the field and asserts it is dropped.

// TestSyntax_FP_Go_NoImport_RouteDropped reproduces the pumba false positive:
// cntr.Delete(ctx, ...) matches go-route-chi/go-route-fiber patterns, but the
// file has NO chi/fiber import. With import-gating the route facts must be dropped.
func TestSyntax_FP_Go_NoImport_RouteDropped(t *testing.T) {
	const fileCntr = "container/container.go"
	entries := marshalSyntaxEntries(t, []map[string]any{
		// No import-signal entry: file has no chi/fiber import.
		// Route match: cntr.Delete(ctx, ...) matches go-route-chi pattern.
		syntaxEntryWithPath("go-route-chi", fileCntr, "ctx", 42),
		syntaxEntryWithPath("go-route-fiber", fileCntr, "ctx", 42),
		// A declaration in the same file (must still be emitted).
		syntaxEntryWithName("go-func", fileCntr, "Delete", 40, 50),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{"go"})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	// Route facts must be dropped; only the function declaration survives.
	if len(facts) != 1 {
		t.Fatalf("len(facts) = %d, want 1 (route FPs must be dropped)", len(facts))
	}
	if facts[0].Kind != kindFunctionStr {
		t.Errorf("remaining fact Kind = %q, want function", facts[0].Kind)
	}
	if facts[0].Name != "Delete" {
		t.Errorf("remaining fact Name = %q, want Delete", facts[0].Name)
	}
}

// TestSyntax_FP_TS_NoImport_RouteDropped reproduces the codegraph false positive:
// https.get(url, cb) matches ts-route-express pattern, but the file has NO express import.
// With import-gating the route fact must be dropped.
func TestSyntax_FP_TS_NoImport_RouteDropped(t *testing.T) {
	const fileClient = "src/client/http.ts"
	entries := marshalSyntaxEntries(t, []map[string]any{
		// No import-signal entry: file has no express import.
		// Route match: https.get(url, cb) matches ts-route-express pattern.
		syntaxEntryWithPath("ts-route-express", fileClient, "url", 10),
		// A declaration in the same file (must still be emitted).
		syntaxEntryWithName("ts-func", fileClient, "fetchData", 8, 15),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langTypeScriptStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	// Route fact must be dropped; only the function declaration survives.
	if len(facts) != 1 {
		t.Fatalf("len(facts) = %d, want 1 (route FP must be dropped)", len(facts))
	}
	if facts[0].Kind != kindFunctionStr {
		t.Errorf("remaining fact Kind = %q, want function", facts[0].Kind)
	}
}

// TestSyntax_FP_Py_VerbSet_RouteDropped reproduces the ccgram false positives:
// @topic_state.register("topic") and @pytest.fixture(autouse=True) matched the
// old py-route-decorator rule. With verb-set constraint they must not match at all
// (the sg output will not contain them), so this test verifies that even if a
// py-route-decorator match somehow appears in sg output without an import signal,
// it is gated out by the import-gating logic.
//
// The verb-set itself is enforced in the YAML by sg; unit tests cannot re-invoke sg.
// What this test verifies: a py-route-decorator without py-import-fastapi → dropped.
func TestSyntax_FP_Py_NoImport_RouteDropped(t *testing.T) {
	const fileApp = "app/tasks.py"
	entries := marshalSyntaxEntries(t, []map[string]any{
		// No import-signal entry: file has no fastapi/aiohttp/flask import.
		// If a decorator somehow matched (e.g. aiohttp not in signal list), it is dropped.
		syntaxEntryWithPath("py-route-decorator", fileApp, "topic", 5),
		// A declaration in the same file (must still be emitted).
		syntaxEntryWithName("py-func", fileApp, "process_message", 3, 8),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langPythonStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	// Route fact must be dropped; only the function declaration survives.
	if len(facts) != 1 {
		t.Fatalf("len(facts) = %d, want 1 (route FP must be dropped)", len(facts))
	}
	if facts[0].Kind != kindFunctionStr {
		t.Errorf("remaining fact Kind = %q, want function", facts[0].Kind)
	}
}

// TestSyntax_Confirmed_Route_Emitted verifies the happy path: a route fact WITH
// a matching import signal is emitted with FrameworkConfirmed=true.
func TestSyntax_Confirmed_Route_Emitted(t *testing.T) {
	const fileRouter = "src/routes/api.ts"
	entries := marshalSyntaxEntries(t, []map[string]any{
		// Import signal: file imports express.
		syntaxEntry("ts-import-express", fileRouter, "express", 1, 1),
		// Route fact: should be emitted confirmed.
		syntaxEntryWithPath("ts-route-express", fileRouter, pathUsers, 10),
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
	if f.Kind != kindRouteStr {
		t.Errorf("Kind = %q, want route", f.Kind)
	}
	if f.Framework != "express" {
		t.Errorf("Framework = %q, want express", f.Framework)
	}
	if !f.FrameworkConfirmed {
		t.Errorf("FrameworkConfirmed = false, want true for confirmed route")
	}
}

func TestSyntax_Py_LazyImport_Facts(t *testing.T) {
	// lazy_import facts use $NAME from the import statement; they are never exported.
	// py-lazy-import-module: `import X` inside a function → Name is module name.
	// py-lazy-import-from: `from X import Y` inside a function → Name is source module.
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("py-lazy-import-module", fileSvcPy, "os", 5, 5),
		syntaxEntryWithName("py-lazy-import-from", fileSvcPy, "pathlib", 8, 8),
	})

	a := astgrep.New(presentRunner(entries))
	facts, cov, err := a.Syntax(context.Background(), syntaxScope, []string{langPythonStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	if cov.Status != "ok" {
		t.Errorf("cov.Status = %q, want ok", cov.Status)
	}
	if len(facts) != 2 {
		t.Fatalf("len(facts) = %d, want 2", len(facts))
	}
	for i, f := range facts {
		if f.Kind != kindLazyImportStr {
			t.Errorf("facts[%d].Kind = %q, want %q", i, f.Kind, kindLazyImportStr)
		}
		if f.Exported {
			t.Errorf("facts[%d] lazy_import should not be exported", i)
		}
		if f.Name == "" {
			t.Errorf("facts[%d] should have a non-empty Name ($NAME from module field)", i)
		}
		if f.Language != langPythonStr {
			t.Errorf("facts[%d].Language = %q, want python", i, f.Language)
		}
	}
	// Verify specific names are preserved.
	wantNames := []string{"os", "pathlib"}
	for i, want := range wantNames {
		if facts[i].Name != want {
			t.Errorf("facts[%d].Name = %q, want %q", i, facts[i].Name, want)
		}
	}
}

func TestSyntax_Py_LazyImport_Empty(t *testing.T) {
	// No lazy-import entries → no lazy_import facts; other kinds still work.
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("py-func", fileSvcPy, nameProcess, 1, 3),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langPythonStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	for _, f := range facts {
		if f.Kind == kindLazyImportStr {
			t.Errorf("unexpected lazy_import fact when none were injected: %+v", f)
		}
	}
}

func TestSyntax_Py_LazyImport_TopLevelNotMatched(t *testing.T) {
	// Note: this test exercises adapter fact-mapping using a mock runner.
	// It does NOT test the YAML `inside:` guard — the integration test
	// (syntax_integration_test.go, fixture.py top-level imports) covers that.
	//
	// Here we inject a py-func fact alongside lazy-import facts to confirm that
	// top-level imports (represented as py-func, py-class, etc.) do not get
	// mislabelled as lazy_import by the adapter mapping logic.
	entries := marshalSyntaxEntries(t, []map[string]any{
		syntaxEntryWithName("py-func", fileSvcPy, "top_level_module", 1, 1),
		syntaxEntryWithName("py-lazy-import-module", fileSvcPy, "requests", 5, 5),
	})

	a := astgrep.New(presentRunner(entries))
	facts, _, err := a.Syntax(context.Background(), syntaxScope, []string{langPythonStr})
	if err != nil {
		t.Fatalf("Syntax: %v", err)
	}
	lazyCount := 0
	for _, f := range facts {
		if f.Kind == kindLazyImportStr {
			lazyCount++
		}
	}
	// Only the explicitly injected lazy-import fact should appear (not the py-func).
	if lazyCount != 1 {
		t.Errorf("lazyCount = %d, want 1 (only injected lazy-import fact)", lazyCount)
	}
}
