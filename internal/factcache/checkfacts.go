package factcache

// CheckFactsAnalyzer is the cache directory of the facts the check reads from
// one tree (imports, literals, unknown items).
const CheckFactsAnalyzer = "checkfacts"

// Prefixes of the input component of a check-facts key. The base and the
// working tree are keyed by different hashes; a prefix keeps the two kinds
// from ever sharing a key.
const (
	gitTreeInput = "git-tree:"
	contentInput = "content:"
)

// TreeFactsKey is the key of the facts read from a git tree: the base of the
// check, and the head as git.WorkingTreeID names it. The tree id names the
// tree's content, so no file is read. toolVersion
// is the version of the extractors that read the facts and settingsHash the
// hash of the settings they read with; a change to either is a new key.
func TreeFactsKey(toolVersion, settingsHash, treeID string) string {
	return Key(CheckFactsAnalyzer, toolVersion, settingsHash, gitTreeInput+treeID)
}

// ContentFactsKey is the key of the facts read from the working tree: the
// content hash of relPaths under root (HashTree). Editing, adding or removing
// a file is a new key; the order of relPaths is not. The error of HashTree
// means the caller runs uncached.
func ContentFactsKey(toolVersion, settingsHash, root string, relPaths []string) (string, error) {
	tree, err := HashTree(root, relPaths)
	if err != nil {
		return "", err
	}
	return Key(CheckFactsAnalyzer, toolVersion, settingsHash, contentInput+tree), nil
}
