package astgrep_test

// Shared constants for JSON key strings that appear across multiple test files
// in this package. goconst requires constants when a string literal appears 3+
// times in the same package.
const (
	// JSON field keys used in sg match fixtures.
	jsonKeyText   = "text"
	jsonKeyLine   = "line"
	jsonKeyColumn = "column"
	jsonKeySingle = "single"
	jsonKeyMulti  = "multi"

	// Repeated file paths used in table-driven tests.
	fileSvcGo = "pkg/svc/svc.go"

	// Repeated kind and language strings used across syntax tests.
	kindRouteStr      = "route"
	langTypeScriptStr = "typescript"
)
