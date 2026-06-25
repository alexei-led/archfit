//go:build ignore

// fixture_type_leak.go is excluded from Go compilation (build tag: ignore).
// ast-grep parses it to exercise the go-type-leak and go-func-type-leak rules.
// This simulates exported struct fields and function returns whose types are
// qualified (external-package) types — the Cat 5 public-API type-leak signal.
//
// Verification:
//   - Exported fields on exported structs: matched (type_leak emitted).
//   - Unexported fields on exported structs: NOT matched (private impl detail).
//   - Exported func with single return qualified type: matched.
//   - Exported func with multi-result (pkg.Type, error): matched (the dominant idiom).
//   - Exported method with multi-result (*pkg.Type, error): matched.

package fixture

// TypeLeakFixture exercises go-type-leak: an exported struct with an exported
// field typed by an external package selector (somepkg.Client).
type TypeLeakFixture struct {
	// Client is exported — this SHOULD produce a type_leak fact.
	Client somepkg.Client
	// internal is unexported — this must NOT produce a type_leak fact.
	internal somepkg.Config
}

// TypeLeakPointer exercises go-type-leak via a pointer-qualified type.
type TypeLeakPointer struct {
	Ctx *somepkg.Context
}

// NewClient exercises go-func-type-leak: single external return type.
func NewClient() somepkg.Client {
	return somepkg.Client{}
}

// GetContext exercises go-func-type-leak: idiomatic multi-result (pkg.Type, error).
// This is the overwhelmingly common Go signature that the old rule missed.
func GetContext() (somepkg.Context, error) {
	return somepkg.Context{}, nil
}

// GetPointerContext exercises go-func-type-leak: multi-result (*pkg.Type, error).
func GetPointerContext() (*somepkg.Context, error) {
	return nil, nil
}
