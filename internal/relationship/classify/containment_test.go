package classify

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	nameSales   = "sales"
	nameCart    = "cart"
	nameInvoice = "invoice"
	nameStock   = "stock"
	globAll     = "a/**"
	fileBY      = "b/y.go"
	fileAX1     = "a/x.go"
	globSales   = "sales/**"
	globCart    = "sales/cart/**"
	nameMid     = "mid"
	nameTop     = "top"
	globB       = "b/**"
)

func mods(paths map[string][]string) map[string]policy.ModuleDef {
	out := make(map[string]policy.ModuleDef, len(paths))
	for name, p := range paths {
		out[name] = policy.ModuleDef{Paths: p}
	}
	return out
}

func TestContainment_ParentRules(t *testing.T) {
	tests := []struct {
		name  string
		paths map[string][]string
		want  map[string]string
	}{
		{
			name:  "recursive glob contains a nested module",
			paths: map[string][]string{nameSales: {globSales}, nameCart: {globCart}},
			want:  map[string]string{nameCart: nameSales, nameSales: ""},
		},
		{
			name:  "exact glob contains nothing",
			paths: map[string][]string{nameSales: {nameSales}, nameCart: {globCart}},
			want:  map[string]string{nameCart: "", nameSales: ""},
		},
		{
			name:  "single-level glob contains nothing",
			paths: map[string][]string{nameSales: {"sales/*"}, nameCart: {globCart}},
			want:  map[string]string{nameCart: ""},
		},
		{
			name:  "extension glob contains nothing",
			paths: map[string][]string{nameSales: {"sales/*.go"}, nameCart: {globCart}},
			want:  map[string]string{nameCart: ""},
		},
		{
			name:  "every root must be inside the parent",
			paths: map[string][]string{nameSales: {globSales}, "mixed": {"sales/a/**", "other/b/**"}},
			want:  map[string]string{"mixed": ""},
		},
		{
			name:  "longest container root wins",
			paths: map[string][]string{nameTop: {globAll}, nameMid: {"a/b/**"}, "leaf": {"a/b/c/**"}},
			want:  map[string]string{"leaf": nameMid, nameMid: nameTop, nameTop: ""},
		},
		{
			name:  "dotted roots for Python",
			paths: map[string][]string{"app": {"app.**"}, "billing": {"app.billing.**"}},
			want:  map[string]string{"billing": "app"},
		},
		{
			name:  "double-colon roots for Rust",
			paths: map[string][]string{subdomainCore: {"core::**"}, "net": {"core::net::**"}},
			want:  map[string]string{"net": subdomainCore},
		},
		{
			name:  "a prefix without a separator is not inside",
			paths: map[string][]string{nameSales: {globSales}, "salesforce": {"salesforce/**"}},
			want:  map[string]string{"salesforce": "", nameSales: ""},
		},
		{
			name:  "equal roots are not strictly inside",
			paths: map[string][]string{"a": {"x/**"}, "b": {"x/**"}},
			want:  map[string]string{"a": "", "b": ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := BuildContainment(mods(tt.paths))
			for module, want := range tt.want {
				if got := c.Parent(module); got != want {
					t.Errorf("Parent(%q) = %q, want %q", module, got, want)
				}
			}
		})
	}
}

func TestContainment_ContainerAndSpan(t *testing.T) {
	c := BuildContainment(mods(map[string][]string{
		nameSales:   {globSales},
		nameCart:    {globCart},
		nameInvoice: {"sales/invoice/**"},
		nameStock:   {"stock/**"},
		"lookup":    {"stock/lookup/**"},
	}))
	tests := []struct {
		a, b           string
		container      string
		crossings, sha int
	}{
		{nameCart, nameInvoice, nameSales, 2, 1},
		{nameCart, "lookup", "", 4, 0},
		{nameSales, nameCart, nameSales, 1, 1},
		{nameSales, nameStock, "", 2, 0},
	}
	for _, tt := range tests {
		t.Run(tt.a+"->"+tt.b, func(t *testing.T) {
			if got := c.Container(tt.a, tt.b); got != tt.container {
				t.Errorf("Container = %q, want %q", got, tt.container)
			}
			crossings, shared := c.Span(tt.a, tt.b)
			if crossings != tt.crossings || shared != tt.sha {
				t.Errorf("Span = (%d, %d), want (%d, %d)", crossings, shared, tt.crossings, tt.sha)
			}
		})
	}
}

// Renaming a module key must not change where it sits: the tree reads paths only.
func TestContainment_KeyRenameInvariant(t *testing.T) {
	a := BuildContainment(mods(map[string][]string{nameSales: {globSales}, nameCart: {globCart}}))
	b := BuildContainment(mods(map[string][]string{"deep/er/sales": {globSales}, "very/deep/cart": {globCart}}))
	ca, sa := a.Span(nameSales, nameCart)
	cb, sb := b.Span("deep/er/sales", "very/deep/cart")
	if ca != cb || sa != sb {
		t.Errorf("rename changed span: (%d,%d) vs (%d,%d)", ca, sa, cb, sb)
	}
}
