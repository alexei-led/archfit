package report

// Repository states: the repository state of design 7.1.
const (
	RepositoryClean   = "clean"
	RepositoryDrift   = "drift"
	RepositoryExpired = "expired"
)

// RepositoryState is the state of the repository for owners (design 7.1 and
// 10.6): the whole head tree judged with the rules and the exceptions of a
// version 3 policy on the run's date. Only `archfit report` fills it.
//
// State does not cover Unread: missing evidence is not zero, so a reader that
// shows State shows Unread beside it.
type RepositoryState struct {
	// State is clean, drift or expired.
	State string `json:"state"`
	// Drift are the errors no exception covers, in record order.
	Drift []RepositoryError `json:"drift"`
	// Expired are the exceptions past their date that the code still breaks,
	// in policy order. The errors they cover are not in Drift.
	Expired []RepositoryError `json:"expired"`
	// Expiring are the exceptions within 14 days of their date that still
	// cover an error, in policy order.
	Expiring []ExpiringException `json:"expiring"`
	// NotChecked are the note rules of the policy, in policy order.
	NotChecked []NoteRule `json:"not_checked"`
	// Balances are the allowed dependencies with the balance of a use of the
	// target's public part, by from module, then to module.
	Balances []AllowedDependency `json:"balances"`
	// Unread are the places of the code the check could not read.
	Unread []UnreadPlace `json:"unread"`
	// Protected is false when no workflow uses the Archfit Action: the
	// result is a local guide (design 9.4).
	Protected bool `json:"protected"`
}

// RepositoryError is one error of the repository, in the fields of the
// record: where, which rule, what; then why, instead and balance. An expired
// exception has no line: its path is the policy file and Expires is its date.
type RepositoryError struct {
	Path    string             `json:"path"`
	Line    int                `json:"line,omitempty"`
	Rule    string             `json:"rule"`
	From    string             `json:"from,omitempty"`
	To      string             `json:"to,omitempty"`
	What    string             `json:"what"`
	Why     string             `json:"why,omitempty"`
	Instead string             `json:"instead,omitempty"`
	Balance *RepositoryBalance `json:"balance,omitempty"`
	Expires string             `json:"expires,omitempty"`
}

// RepositoryBalance is a balance as the record shows it: value/10 (min min).
type RepositoryBalance struct {
	Value int `json:"value"`
	Min   int `json:"min"`
}

// ExpiringException is an exception close to its date.
type ExpiringException struct {
	Rule string `json:"rule"`
	From string `json:"from"`
	To   string `json:"to,omitempty"`
	// Expires is the date with the days left: "2026-11-19 (9 days)".
	Expires string `json:"expires"`
	Why     string `json:"why,omitempty"`
}

// NoteRule is a rule of the policy that nothing checks.
type NoteRule struct {
	ID   string `json:"id"`
	Note string `json:"note"`
	Why  string `json:"why,omitempty"`
}

// AllowedDependency is a depends_on entry and its balance.
type AllowedDependency struct {
	From    string            `json:"from"`
	To      string            `json:"to"`
	Balance RepositoryBalance `json:"balance"`
}

// UnreadPlace is a place of the code the check could not read, and the step
// that restores it.
type UnreadPlace struct {
	Path   string `json:"path,omitempty"`
	Line   int    `json:"line,omitempty"`
	Reason string `json:"reason"`
	Fix    string `json:"fix,omitempty"`
}
