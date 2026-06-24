package syntax_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/syntax"
)

// kind literal constants used across test cases (goconst threshold = 3).
const (
	kindFunction   = "function"
	kindStruct     = "struct"
	kindClass      = "class"
	kindAnnotation = "annotation"
	kindInterface  = "interface"
	kindRoute      = "route"
)

// evidence prefix constants.
const (
	evDecorator  = "decorator"
	evNameSuffix = "name suffix"
)

// name constants reused across cases.
const (
	nameUserHandler    = "UserHandler"
	nameUserRepository = "UserRepository"
)

// file path constants reused across cases.
const (
	fileInternalDBUser = "internal/db/user.go"
)

func TestDeriveRoles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     diagnostic.SyntaxFact
		wantRole  string
		wantConf  string
		wantEvPfx string // prefix of Evidence string (empty = no check)
	}{
		// ---- Tier 1: route kind → handler high ----
		{
			name:      "kind=route gives handler high",
			input:     diagnostic.SyntaxFact{Kind: kindRoute, Name: "GetUsers", File: "pkg/api/users.go"},
			wantRole:  syntax.RoleHandler,
			wantConf:  syntax.ConfHigh,
			wantEvPfx: "route registration",
		},
		// ---- Tier 1: non-empty Framework → handler high ----
		{
			name:      "framework gin gives handler high",
			input:     diagnostic.SyntaxFact{Kind: kindFunction, Name: "ListOrders", File: "cmd/server.go", Framework: "gin"},
			wantRole:  syntax.RoleHandler,
			wantConf:  syntax.ConfHigh,
			wantEvPfx: "framework gin",
		},
		{
			name:      "framework express gives handler high",
			input:     diagnostic.SyntaxFact{Kind: kindFunction, Name: "handler", File: "src/routes.ts", Framework: "express"},
			wantRole:  syntax.RoleHandler,
			wantConf:  syntax.ConfHigh,
			wantEvPfx: "framework express",
		},
		// ---- Tier 1: annotation + name contains controller → handler high ----
		{
			name:      "annotation @Controller → handler high",
			input:     diagnostic.SyntaxFact{Kind: kindAnnotation, Name: "Controller", File: "src/user.ts"},
			wantRole:  syntax.RoleHandler,
			wantConf:  syntax.ConfHigh,
			wantEvPfx: evDecorator,
		},
		{
			name:      "annotation @Service → service high",
			input:     diagnostic.SyntaxFact{Kind: kindAnnotation, Name: "Service", File: "src/billing.ts"},
			wantRole:  syntax.RoleService,
			wantConf:  syntax.ConfHigh,
			wantEvPfx: evDecorator,
		},
		{
			name:      "annotation @Repository → repository high",
			input:     diagnostic.SyntaxFact{Kind: kindAnnotation, Name: "Repository", File: "src/user_repo.ts"},
			wantRole:  syntax.RoleRepository,
			wantConf:  syntax.ConfHigh,
			wantEvPfx: evDecorator,
		},
		{
			name:      "annotation @Entity → domain high",
			input:     diagnostic.SyntaxFact{Kind: kindAnnotation, Name: "Entity", File: "src/order.ts"},
			wantRole:  syntax.RoleDomain,
			wantConf:  syntax.ConfHigh,
			wantEvPfx: evDecorator,
		},
		// ---- Tier 2: name suffix → medium ----
		{
			name:      "name suffix Handler → handler medium",
			input:     diagnostic.SyntaxFact{Kind: kindClass, Name: nameUserHandler, File: "pkg/web/user.go"},
			wantRole:  syntax.RoleHandler,
			wantConf:  syntax.ConfMedium,
			wantEvPfx: evNameSuffix,
		},
		{
			name:      "name suffix Controller → handler medium",
			input:     diagnostic.SyntaxFact{Kind: kindStruct, Name: "OrderController", File: "internal/api/order.go"},
			wantRole:  syntax.RoleHandler,
			wantConf:  syntax.ConfMedium,
			wantEvPfx: evNameSuffix,
		},
		{
			name:      "name suffix Repository → repository medium",
			input:     diagnostic.SyntaxFact{Kind: kindStruct, Name: nameUserRepository, File: fileInternalDBUser},
			wantRole:  syntax.RoleRepository,
			wantConf:  syntax.ConfMedium,
			wantEvPfx: evNameSuffix,
		},
		{
			name:      "name suffix Repo → repository medium",
			input:     diagnostic.SyntaxFact{Kind: kindInterface, Name: "UserRepo", File: "internal/ports/user.go"},
			wantRole:  syntax.RoleRepository,
			wantConf:  syntax.ConfMedium,
			wantEvPfx: evNameSuffix,
		},
		{
			name:      "name suffix Storage → repository medium",
			input:     diagnostic.SyntaxFact{Kind: kindStruct, Name: "FileStorage", File: "internal/infra/fs.go"},
			wantRole:  syntax.RoleRepository,
			wantConf:  syntax.ConfMedium,
			wantEvPfx: evNameSuffix,
		},
		{
			name:      "name suffix Service → service medium",
			input:     diagnostic.SyntaxFact{Kind: kindStruct, Name: "BillingService", File: "internal/billing/billing.go"},
			wantRole:  syntax.RoleService,
			wantConf:  syntax.ConfMedium,
			wantEvPfx: evNameSuffix,
		},
		{
			name:      "name suffix UseCase → service medium",
			input:     diagnostic.SyntaxFact{Kind: kindStruct, Name: "CreateOrderUseCase", File: "internal/order/create.go"},
			wantRole:  syntax.RoleService,
			wantConf:  syntax.ConfMedium,
			wantEvPfx: evNameSuffix,
		},
		{
			name:      "name suffix Domain → domain medium",
			input:     diagnostic.SyntaxFact{Kind: kindStruct, Name: "OrderDomain", File: "internal/order/model.go"},
			wantRole:  syntax.RoleDomain,
			wantConf:  syntax.ConfMedium,
			wantEvPfx: evNameSuffix,
		},
		{
			name:      "name suffix Entity → domain medium",
			input:     diagnostic.SyntaxFact{Kind: kindClass, Name: "UserEntity", File: "src/user.ts"},
			wantRole:  syntax.RoleDomain,
			wantConf:  syntax.ConfMedium,
			wantEvPfx: evNameSuffix,
		},
		{
			name:      "name suffix Model → domain medium",
			input:     diagnostic.SyntaxFact{Kind: kindStruct, Name: "UserModel", File: fileInternalDBUser},
			wantRole:  syntax.RoleDomain,
			wantConf:  syntax.ConfMedium,
			wantEvPfx: evNameSuffix,
		},
		// ---- Tier 3: path substring → low ----
		{
			name:      "path contains handler → handler low",
			input:     diagnostic.SyntaxFact{Kind: kindFunction, Name: "processRequest", File: "pkg/handler/request.go"},
			wantRole:  syntax.RoleHandler,
			wantConf:  syntax.ConfLow,
			wantEvPfx: "path contains handler",
		},
		{
			name:      "path contains controller → handler low",
			input:     diagnostic.SyntaxFact{Kind: kindFunction, Name: "index", File: "app/controller/home.go"},
			wantRole:  syntax.RoleHandler,
			wantConf:  syntax.ConfLow,
			wantEvPfx: "path contains controller",
		},
		{
			name:      "path contains routes → handler low",
			input:     diagnostic.SyntaxFact{Kind: kindFunction, Name: "setupRoutes", File: "internal/routes/setup.go"},
			wantRole:  syntax.RoleHandler,
			wantConf:  syntax.ConfLow,
			wantEvPfx: "path contains routes",
		},
		{
			name:      "path contains repository → repository low",
			input:     diagnostic.SyntaxFact{Kind: kindFunction, Name: "findByID", File: "internal/repository/user.go"},
			wantRole:  syntax.RoleRepository,
			wantConf:  syntax.ConfLow,
			wantEvPfx: "path contains repository",
		},
		{
			name:      "path contains service → service low",
			input:     diagnostic.SyntaxFact{Kind: kindFunction, Name: "calculate", File: "internal/service/pricing.go"},
			wantRole:  syntax.RoleService,
			wantConf:  syntax.ConfLow,
			wantEvPfx: "path contains service",
		},
		{
			name:      "path contains usecase → service low",
			input:     diagnostic.SyntaxFact{Kind: kindFunction, Name: "execute", File: "internal/usecase/order.go"},
			wantRole:  syntax.RoleService,
			wantConf:  syntax.ConfLow,
			wantEvPfx: "path contains usecase",
		},
		{
			name:      "path contains domain → domain low",
			input:     diagnostic.SyntaxFact{Kind: kindStruct, Name: "Price", File: "internal/domain/pricing.go"},
			wantRole:  syntax.RoleDomain,
			wantConf:  syntax.ConfLow,
			wantEvPfx: "path contains domain",
		},
		{
			name:      "path contains model → domain low",
			input:     diagnostic.SyntaxFact{Kind: kindStruct, Name: "User", File: "internal/model/user.go"},
			wantRole:  syntax.RoleDomain,
			wantConf:  syntax.ConfLow,
			wantEvPfx: "path contains model",
		},
		// ---- No match ----
		{
			name:     "no match — role stays empty",
			input:    diagnostic.SyntaxFact{Kind: kindFunction, Name: "parse", File: "internal/config/loader.go"},
			wantRole: "",
			wantConf: "",
		},
		{
			name:     "no match — utility file",
			input:    diagnostic.SyntaxFact{Kind: kindFunction, Name: "formatDate", File: "pkg/util/time.go"},
			wantRole: "",
			wantConf: "",
		},
		// ---- Precedence: tier 1 beats tier 2 ----
		{
			name: "route+name-suffix: tier 1 (route) wins over tier 2 (name suffix Repository)",
			input: diagnostic.SyntaxFact{
				Kind: kindRoute, Name: nameUserRepository, File: "internal/repository/user.go",
			},
			wantRole:  syntax.RoleHandler, // route wins
			wantConf:  syntax.ConfHigh,
			wantEvPfx: "route registration",
		},
		// ---- Precedence: tier 2 beats tier 3 ----
		{
			name: "name suffix wins over path: UserHandler in service/ path",
			input: diagnostic.SyntaxFact{
				Kind: kindStruct, Name: nameUserHandler, File: "internal/service/user.go",
			},
			wantRole:  syntax.RoleHandler, // name suffix beats path
			wantConf:  syntax.ConfMedium,
			wantEvPfx: evNameSuffix,
		},
		// ---- Existing role overwritten ----
		{
			name: "pre-set role is overwritten by DeriveRoles",
			input: diagnostic.SyntaxFact{
				Kind: kindStruct, Name: nameUserHandler, File: "pkg/web/user.go",
				Role: "domain", RoleConf: "low",
			},
			wantRole:  syntax.RoleHandler, // derivation rewrites
			wantConf:  syntax.ConfMedium,
			wantEvPfx: evNameSuffix,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := syntax.DeriveRoles([]diagnostic.SyntaxFact{tc.input})
			if len(got) != 1 {
				t.Fatalf("DeriveRoles returned %d facts, want 1", len(got))
			}
			f := got[0]
			if f.Role != tc.wantRole {
				t.Errorf("Role = %q, want %q", f.Role, tc.wantRole)
			}
			if f.RoleConf != tc.wantConf {
				t.Errorf("RoleConf = %q, want %q", f.RoleConf, tc.wantConf)
			}
			if tc.wantEvPfx != "" && !hasPrefix(f.Evidence, tc.wantEvPfx) {
				t.Errorf("Evidence = %q, want prefix %q", f.Evidence, tc.wantEvPfx)
			}
			if tc.wantRole == "" && f.Evidence != "" {
				t.Errorf("Evidence = %q, want empty (no match)", f.Evidence)
			}
		})
	}
}

// TestDeriveRoles_InputUnchanged verifies that DeriveRoles does not mutate its
// input slice — it returns a new slice.
func TestDeriveRoles_InputUnchanged(t *testing.T) {
	t.Parallel()
	orig := diagnostic.SyntaxFact{Kind: kindStruct, Name: nameUserRepository, File: fileInternalDBUser}
	input := []diagnostic.SyntaxFact{orig}
	_ = syntax.DeriveRoles(input)
	if input[0].Role != "" {
		t.Errorf("DeriveRoles mutated input: Role = %q", input[0].Role)
	}
}

// TestDeriveRoles_Empty verifies that an empty input returns an empty slice.
func TestDeriveRoles_Empty(t *testing.T) {
	t.Parallel()
	got := syntax.DeriveRoles(nil)
	if len(got) != 0 {
		t.Errorf("DeriveRoles(nil) = %d facts, want 0", len(got))
	}
	got = syntax.DeriveRoles([]diagnostic.SyntaxFact{})
	if len(got) != 0 {
		t.Errorf("DeriveRoles([]) = %d facts, want 0", len(got))
	}
}

// TestConfidenceMeets exercises the ConfidenceMeets helper directly.
func TestConfidenceMeets(t *testing.T) {
	t.Parallel()
	const confBogus = "bogus"
	cases := []struct {
		conf, min string
		want      bool
	}{
		{syntax.ConfHigh, syntax.ConfHigh, true},
		{syntax.ConfHigh, syntax.ConfMedium, true},
		{syntax.ConfHigh, syntax.ConfLow, true},
		{syntax.ConfMedium, syntax.ConfHigh, false},
		{syntax.ConfMedium, syntax.ConfMedium, true},
		{syntax.ConfMedium, syntax.ConfLow, true},
		{syntax.ConfLow, syntax.ConfHigh, false},
		{syntax.ConfLow, syntax.ConfMedium, false},
		{syntax.ConfLow, syntax.ConfLow, true},
		// Unknown strings return false regardless of the other operand.
		{"", syntax.ConfHigh, false},
		{syntax.ConfHigh, "", false},
		{confBogus, syntax.ConfHigh, false},
		{syntax.ConfHigh, confBogus, false},
	}
	for _, tc := range cases {
		got := syntax.ConfidenceMeets(tc.conf, tc.min)
		if got != tc.want {
			t.Errorf("ConfidenceMeets(%q, %q) = %v, want %v", tc.conf, tc.min, got, tc.want)
		}
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
