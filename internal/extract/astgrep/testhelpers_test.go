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

	// Coverage status strings.
	statusAbsentStr = "absent"

	// Repeated file paths used in table-driven tests.
	fileSvcGo = "pkg/svc/svc.go"

	// Repeated kind and language strings used across syntax tests.
	kindFunctionStr    = "function"
	kindMethodStr      = "method"
	kindStructStr      = "struct"
	kindInterfaceStr   = "interface"
	kindClassStr       = "class"
	kindEnumStr        = "enum"
	kindAnnotStr       = "annotation"
	kindRouteStr       = "route"
	kindTestImportStr  = "test_import"
	kindUnsafeOpStr    = "unsafe_op"
	kindStructFieldStr = "struct_field"
	kindPanicOpStr     = "panic_op"
	langTypeScriptStr  = "typescript"
	langPythonStr      = "python"
	langRustStr        = "rust"

	// Repeated name strings used across syntax tests.
	nameProcess = "process"

	// Repeated route path strings used across syntax tests.
	pathUsers = "/users"
)
