package scope

// Config is the scope-resolution request: the analysis boundary, the changed-file base,
// and the exclusions every walk shares.
type Config struct {
	// Root is the absolute path of the analysis boundary (ScanRoot). When
	// non-empty it overrides the git toplevel so archfit scans a subdirectory
	// of a monorepo. Empty means "use the git toplevel (or WorkDir as a
	// last resort)". Callers (cmd) are responsible for making this absolute.
	Root string
	Base string // git ref to diff against (empty = none)
	// AddedBase is the ref the added-file set is measured against when Base
	// is empty. check sets HEAD on the target branch, where the whole tree is
	// the change but uncommitted and untracked files are still new.
	AddedBase  string
	Exclusions []string
	WorkDir    string // working directory for git commands; empty = process cwd
}
