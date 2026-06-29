// Package fileclass defines the FileClass type used to categorise source files
// as Production, Test, Generated, or Vendor. This type lives in the model layer
// (stdlib-only) so every part of the system — core ring, adapters, cmd — can
// reference it without introducing a forbidden import.
package fileclass

// FileClass categorises a source file for metric-filtering purposes.
//
// Policy (segregate, not hide): production-health metrics exclude Test,
// Generated, and Vendor from their score but report the excluded count as
// evidence. Test density deliberately retains Test files.
type FileClass string

const (
	// Production is the default for any file not matched by the other classes.
	Production FileClass = "production"
	// Test matches language-convention test files and test helper patterns.
	Test FileClass = "test"
	// Generated matches code-generated files: proto/grpc stubs, moq fakes,
	// files with the standard "Code generated … DO NOT EDIT" header, and
	// filename patterns like *_gen.go, *.pb.go, *_pb2.py, mock_*.go, *_mock.go.
	Generated FileClass = "generated"
	// Vendor matches third-party code vendored into the repository.
	// Currently derived from directory name ("vendor/") — not from file
	// content. Files in vendor/ directories are already excluded by the
	// loc walk's skipDirs list, so this class exists for completeness and
	// for any callers that scan vendor trees directly.
	Vendor FileClass = "vendor"
)

// IsProduction reports whether fc is Production. Use this instead of
// `fc != Test && fc != Generated` to ensure Vendor files are also excluded
// from production-health metrics.
func IsProduction(fc FileClass) bool { return fc == Production }
