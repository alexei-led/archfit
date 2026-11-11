// Package golang extracts Go imports on two paths.
//
// The typed loader (GoExtractor) uses golang.org/x/tools/go/packages. It is an
// in-process adapter that satisfies the engine.Extractor interface, needs the
// Go toolchain and resolvable module dependencies, and adds strength hints
// from go/types.
//
// The parser path (ParseImports) reads file-level import facts with go/parser
// and go/build only: the members from go.work and go.mod, the files the go
// command would read, and each import with its line, placed on a first-party
// package or called external. It runs no tool. `policy can-import`
// (QueryEdge) reads files and places targets with the same decisions, so it
// answers for the files the check reads. A standard-library import is always
// external, even beside a first-party package of the same name.
//
// Without a Go tool the parser path cannot see whether cgo is on when
// CGO_ENABLED is unset, so a file that only one cgo setting reads is Unknown,
// never guessed. The check counts an unknown only where the change adds it, so
// a cgo file present on both base and head does not block a change.
package golang
