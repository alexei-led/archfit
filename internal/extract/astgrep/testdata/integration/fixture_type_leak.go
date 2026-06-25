//go:build ignore

// fixture_type_leak.go is excluded from Go compilation (build tag: ignore).
// ast-grep parses it to exercise the go-type-leak rule.
// This simulates an exported struct with a field whose type is a qualified
// (external-package) type — the Cat 5 public-API type-leak signal.

package fixture

// TypeLeakFixture exercises go-type-leak: an exported struct with a field
// typed by an external package selector (somepkg.Client).
// Ceiling: field-level export is not checked — unexported fields on exported
// structs are also captured (report-only; acceptable for structural signal).
type TypeLeakFixture struct {
	// Client uses a qualified type from an "external" package.
	Client somepkg.Client
}

// TypeLeakPointer exercises go-type-leak via a pointer-qualified type.
type TypeLeakPointer struct {
	Ctx *somepkg.Context
}
