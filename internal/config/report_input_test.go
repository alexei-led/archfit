package config

import (
	"strings"
	"testing"
)

func TestReportInputPreservesPolicyOrigins(t *testing.T) {
	t.Parallel()
	base := "version: 3\nowners: [owner]\nmodules:\n  app:\n    paths: [app/**]\n"
	for _, tc := range []struct {
		name, field       string
		raw               *string
		origin, effective string
	}{
		{"omitted", "", nil, "assumed", "high"},
		{"explicit high", "    volatility: high\n", new("high"), "asserted", "high"},
		{"derived supporting", "    subdomain: supporting\n", nil, "assessed", "low"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in, err := NewReportInput([]byte(base + tc.field))
			if err != nil {
				t.Fatal(err)
			}
			c := in.Components[0]
			if (c.Volatility.Raw == nil) != (tc.raw == nil) || tc.raw != nil && *c.Volatility.Raw != *tc.raw || string(c.Volatility.Origin) != tc.origin || c.Volatility.Effective == nil || *c.Volatility.Effective != tc.effective {
				t.Fatalf("volatility: %+v", c.Volatility)
			}
			if c.Public.Raw != nil || c.Public.Origin != "assumed" || c.Public.Effective == nil || len(*c.Public.Effective) != 1 {
				t.Fatalf("public default: %+v", c.Public)
			}
			copied := in.Bytes()
			copied[0] = 'X'
			if strings.HasPrefix(string(in.Bytes()), "X") {
				t.Fatal("consumed bytes were mutable")
			}
		})
	}
}

func TestReportInputDoesNotRelaxExplicitEmptyPublicValidation(t *testing.T) {
	t.Parallel()
	data := "version: 3\nowners: [owner]\nmodules:\n  app:\n    paths: [app/**]\n    public: []\n"
	if _, err := NewReportInput([]byte(data)); err == nil || !strings.Contains(err.Error(), "public") {
		t.Fatalf("explicit empty public accepted: %v", err)
	}
}
