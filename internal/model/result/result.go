// Package result is the result contract of Archfit 4: the JSON that
// `archfit check` and `archfit can` print with `--format json`, and that the
// upload to the App carries unchanged.
//
// Two documents share one schema file, archfit.result.schema.json:
//
//   - Result, the verdict for a change (`check`), with Schema "archfit.result.v1";
//   - CanAnswer, the answer to `can <from> <to>`, with Schema "archfit.can.v1".
//
// The fields follow the text record of design section 8: where, which rule,
// what; then why, instead, balance, next. The JSON adds what a machine needs
// and a worker does not read: keys, digests, commits, the engine version.
//
// The package holds data only and imports no project package. The schema is
// generated from these types by internal/reportschema; `make schema` rewrites
// it and a no-drift test pins it. Lists are never null: a producer writes an
// empty list.
package result

// Document identifiers, the value of each document's schema field.
const (
	// SchemaVersion identifies a Result.
	SchemaVersion = "archfit.result.v1"
	// CanSchemaVersion identifies a CanAnswer.
	CanSchemaVersion = "archfit.can.v1"
)

// Status is the status of a change.
type Status string

// The four statuses of a change (design 7.1).
const (
	// StatusPass: the change adds nothing a rule forbids.
	StatusPass Status = "pass"
	// StatusFail: the change adds an error, or an exception has expired.
	StatusFail Status = "fail"
	// StatusWaiting: the errors are covered by an open request. Set by the App
	// only; the engine never emits it.
	StatusWaiting Status = "waiting"
	// StatusUnknown: no verdict, because evidence is missing or the local
	// policy differs from the protected one.
	StatusUnknown Status = "unknown"
)

// NextKind says what kind of step Next.Text is.
type NextKind string

// The kinds of next step.
const (
	// NextFix: change the code ("fix the code", or what `instead` says).
	NextFix NextKind = "fix"
	// NextCommand: run the command in the text, such as `archfit request`.
	NextCommand NextKind = "command"
	// NextWait: wait for an owner to answer a request.
	NextWait NextKind = "wait"
	// NextRestore: restore what the text names (the policy file, the merge
	// base, a tool), then run the check again.
	NextRestore NextKind = "restore"
)

// Cause is the reason a check gave no verdict. The record prints it where an
// error prints its rule.
type Cause string

// The causes of an unknown status.
const (
	// CausePolicy: the local policy differs from the target's, or is invalid.
	CausePolicy Cause = "policy"
	// CauseBase: there is no target branch, no merge base, or no evidence for
	// the base.
	CauseBase Cause = "base"
	// CauseTool: a tool the check needs is missing.
	CauseTool Cause = "tool"
	// CauseImport: a first-party import cannot be resolved.
	CauseImport Cause = "import"
)

// Reply is the answer of `archfit can`.
type Reply string

// The answers of `archfit can`.
const (
	// ReplyYes: the dependency is allowed.
	ReplyYes Reply = "yes"
	// ReplyNo: a rule forbids the dependency.
	ReplyNo Reply = "no"
	// ReplyUnknown: no answer, for example for a path in no module.
	ReplyUnknown Reply = "unknown"
)

// Result is the verdict of `archfit check` for one change.
type Result struct {
	// Schema is always archfit.result.v1.
	Schema string `json:"schema"`
	// Status is the status of the change.
	Status Status `json:"status"`
	// Summary is the status line of the record, such as
	// "archfit: fail, 1 error (not from this change)".
	Summary string `json:"summary"`
	// Errors are the errors of the change, in record order. An expired
	// exception is listed for every change, with FromChange false.
	Errors []Error `json:"errors"`
	// Unknown are the reasons for an unknown status, in record order.
	Unknown []Unknown `json:"unknown"`
	// Policy names the policy that judged the change.
	Policy Policy `json:"policy"`
	// Base is the merge-base commit. Absent when there is no merge base.
	Base string `json:"base,omitempty"`
	// Head is the HEAD commit of the checked tree.
	Head string `json:"head,omitempty"`
	// Date is the date the exceptions were judged on (YYYY-MM-DD). It is an
	// input: the same tree, policy, engine and date give the same result.
	Date string `json:"date"`
	// Engine is the version of the archfit build that produced the result.
	Engine string `json:"engine"`
}

// Error is one place where the change breaks a rule.
type Error struct {
	// Path is the file, relative to the repository root. An error of the
	// policy itself, such as an expired exception, has the path .archfit.yaml.
	Path string `json:"path"`
	// Line is the line in Path. Absent for an error of the policy file.
	Line int `json:"line,omitempty"`
	// Rule is the rule id: a built-in rule (graph, coupling, duplication,
	// cycle, layers, unowned), a custom rule id, or "exception" for an
	// exception past its date.
	Rule string `json:"rule"`
	// What says what is wrong.
	What string `json:"what"`
	// Why is the reason the rule exists.
	Why string `json:"why,omitempty"`
	// Instead is what the owner says to do, when the rule has it.
	Instead string `json:"instead,omitempty"`
	// Balance is the balance of the dependency, when the rule is about
	// coupling.
	Balance *Balance `json:"balance,omitempty"`
	// Next is the one step to take.
	Next Next `json:"next"`
	// From is the module the dependency starts in.
	From string `json:"from,omitempty"`
	// To is the module the dependency reaches, or the library for an only
	// rule.
	To string `json:"to,omitempty"`
	// Key identifies the error: its rule, modules, and symbol or literal.
	// Equal keys name the same error in two results of one engine version.
	// Consumers compare keys and do not parse them.
	Key string `json:"key"`
	// FromChange is false only for an expired exception that the change did
	// not add to.
	FromChange bool `json:"from_change"`
	// Exception names the policy exception that has expired. Set only when
	// Rule is "exception".
	Exception *Exception `json:"exception,omitempty"`
}

// Balance is the coupling number of a dependency, printed as
// "Value/10 (min Min)".
type Balance struct {
	// Value is the balance of the dependency, 1 to 10.
	Value int `json:"value"`
	// Min is the policy's min_balance, 1 to 10.
	Min int `json:"min"`
}

// Next is the one step a record names.
type Next struct {
	// Kind is the kind of step.
	Kind NextKind `json:"kind"`
	// Text is the step as the record prints it: "fix the code", a command, or
	// "wait for an owner: #431".
	Text string `json:"text"`
	// Note is a short hint the record prints in parentheses after Text, such
	// as "do not copy the code".
	Note string `json:"note,omitempty"`
}

// Exception identifies an exception of the policy by its rule; From and To of
// the error hold its modules.
type Exception struct {
	// Rule is the rule the exception covers.
	Rule string `json:"rule"`
	// Expires is the date the exception ended (YYYY-MM-DD).
	Expires string `json:"expires"`
}

// Unknown is one reason the check gave no verdict.
type Unknown struct {
	// Path is the file the reason is about, when there is one.
	Path string `json:"path,omitempty"`
	// Line is the line in Path, when there is one.
	Line int `json:"line,omitempty"`
	// Cause is what is missing or wrong.
	Cause Cause `json:"cause"`
	// What says what is missing or wrong.
	What string `json:"what"`
	// Why is the reason this stops the verdict.
	Why string `json:"why,omitempty"`
	// Next is the one step that restores a verdict.
	Next Next `json:"next"`
}

// Policy names the policy that judged a change.
type Policy struct {
	// Path is the policy file, relative to the repository root.
	Path string `json:"path"`
	// Digest is "sha256:" and the hex SHA-256 of the file's bytes.
	Digest string `json:"digest"`
	// Protected is true when a workflow runs the check on pull requests.
	Protected bool `json:"protected"`
	// Differs is true when the local policy differs from the target's.
	Differs bool `json:"differs"`
	// Target is the ref the policy was compared with, such as origin/main.
	// Absent when no comparison was made.
	Target string `json:"target,omitempty"`
}

// CanAnswer is the answer of `archfit can <from> <to>`: may this dependency
// or library be used.
type CanAnswer struct {
	// Schema is always archfit.can.v1.
	Schema string `json:"schema"`
	// From is the module of the first argument, or the argument as given
	// when it is in no module.
	From string `json:"from"`
	// To is the module or library of the second argument, or the argument as
	// given when it is in no module.
	To string `json:"to"`
	// Answer is the answer.
	Answer Reply `json:"answer"`
	// Rule is the rule that decides a no or an unknown.
	Rule string `json:"rule,omitempty"`
	// What says why the answer is not yes.
	What string `json:"what,omitempty"`
	// Why is the reason the rule exists.
	Why string `json:"why,omitempty"`
	// Instead is what the owner says to do, when the rule has it.
	Instead string `json:"instead,omitempty"`
	// Balance is the balance the dependency would have.
	Balance *Balance `json:"balance,omitempty"`
	// Next is the one step to take, when the answer is not yes.
	Next *Next `json:"next,omitempty"`
	// Policy names the policy that gave the answer.
	Policy Policy `json:"policy"`
	// Head is the HEAD commit when the question was asked.
	Head string `json:"head,omitempty"`
	// Date is the date of the answer (YYYY-MM-DD).
	Date string `json:"date"`
	// Engine is the version of the archfit build that answered.
	Engine string `json:"engine"`
}
