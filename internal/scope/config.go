package scope

// Config is the scope-resolution request: the analysis boundary, the diff mode,
// and the exclusions every walk shares.
type Config struct {
	// Root is the absolute path of the analysis boundary (ScanRoot). When
	// non-empty it overrides the git toplevel so archfit scans a subdirectory
	// of a monorepo. Empty means "use the git toplevel (or WorkDir as a
	// last resort)". Callers (cmd) are responsible for making this absolute.
	Root       string
	Base       string // git ref to diff against (empty = none)
	Full       bool   // if true, full-repo mode (no diff)
	Exclusions []string
	WorkDir    string // working directory for git commands; empty = process cwd
}
