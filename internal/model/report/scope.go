package report

// Scope resolution modes: how a check decided what the change is.
const (
	// ScopeModeNone: no git repository, or no commit yet. The whole tree is the
	// change.
	ScopeModeNone = "none"
	// ScopeModeExplicit: the caller named the base (--base <ref>).
	ScopeModeExplicit = "explicit"
	// ScopeModeTargetBranch: HEAD is on the target branch. The whole tree is
	// the change, as for a push to the default branch.
	ScopeModeTargetBranch = "target_branch"
	// ScopeModeMergeBase: the change is what HEAD adds after its merge-base with
	// the target branch.
	ScopeModeMergeBase = "merge_base"
)

// CheckScope is how a check resolved what the change is and which policy
// judges it. It is run context, not part of the state wire contract: the agent
// result and the check text render it.
type CheckScope struct {
	// Mode is one of the ScopeMode constants.
	Mode string
	// Target and TargetSHA name the target branch and the commit it names.
	Target    string
	TargetSHA string
	// Base is the commit or ref the change was compared against; empty means
	// the whole tree.
	Base string
	// Unresolved says why the target or the base could not be resolved. A scope
	// with a reason is never a pass.
	Unresolved string
	// PolicyChanged lists the policy files the branch edited that differ from
	// the target tip.
	PolicyChanged []string
	// PolicyStale lists the policy files the branch left alone that differ from
	// the target tip because the target changed them after the fork.
	PolicyStale []string
	// TargetPolicy is the same tree measured under the target tip's policy
	// files. It is set only when PolicyChanged or PolicyStale is not empty.
	TargetPolicy *Document
}
