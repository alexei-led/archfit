package initcfg

import (
	"regexp"
	"strconv"
	"strings"
)

// MigrationSchemaVersion identifies the migration report contract.
const MigrationSchemaVersion = "archfit.config-migration.v1"

// TargetSchemaVersion is the config schema this migration writes.
//
// It restates config.SchemaVersion rather than importing it: initcfg is a
// support-layer YAML editor and the architecture forbids it from importing the
// config lifecycle (the `*_no_config` rules in .archfit.yaml). The two are
// pinned together by TestMigrationTargetsCurrentSchema in cmd, which is a
// composition root and may see both.
const TargetSchemaVersion = 2

// Migration statuses.
const (
	// MigrationRequired means the file is not yet on the current schema.
	MigrationRequired = "migration_required"
	// MigrationAlreadyCurrent means nothing needs to change.
	MigrationAlreadyCurrent = "already_current"
)

// MigrationChange is one deterministic edit the migration makes.
type MigrationChange struct {
	Key    string `json:"key"`
	Action string `json:"action"` // set | removed | inserted
	Detail string `json:"detail"`
}

// Migration edit actions.
const (
	migrationSet      = "set"
	migrationRemoved  = "removed"
	migrationInserted = "inserted"
)

// MigrationResult is the outcome of migrating one config file.
//
// Output is always the full file: unchanged when nothing needed migrating, so a
// caller can write it back unconditionally and get byte-identical results on a
// second run.
type MigrationResult struct {
	SchemaVersion string            `json:"schema_version"`
	Status        string            `json:"status"`
	FromVersion   int               `json:"from_version"`
	ToVersion     int               `json:"to_version"`
	Changes       []MigrationChange `json:"changes"`
	// PolicyChanges name semantic shifts the edits cause. A version bump is
	// mechanical; retiring a gate is not, and the report must say so rather
	// than let a silent edit change what blocks a build.
	PolicyChanges []string `json:"policy_changes"`
	Output        []byte   `json:"-"`
}

// Changed reports whether the migration produced different bytes.
func (r MigrationResult) Changed() bool { return r.Status == MigrationRequired }

var (
	// topLevelVersion matches the root `version:` key: column zero only, so a
	// nested `version:` under some other stanza is never rewritten.
	topLevelVersion = regexp.MustCompile(`^version:\s*(\d+)\s*(#.*)?$`)
	// retiredGateKey matches an indented retired coupling-gate knob. The value
	// is deliberately unconstrained: a valueless `min_band:` is still the
	// retired key, and leaving it behind ships a "migrated" file that the
	// current schema still rejects.
	retiredGateKey = regexp.MustCompile(`^(\s+)(min_band|max_drop):`)
	// existingStanza matches an already-authored distributed_monolith key, so a
	// hybrid config (a retired knob beside a hand-written replacement) is not
	// given a second copy of it. It matches block AND flow style — over-matching
	// only skips the insert, which a reader can see and fix, while under-matching
	// splices a duplicate YAML key that makes the file unloadable.
	existingStanza = regexp.MustCompile(`^\s+distributed_monolith:`)
)

// MigrateToV2 rewrites a v1 config onto schema v2.
//
// It is a line transform, not a YAML round-trip, for one reason: a round-trip
// through a marshaller reflows the whole file and discards every comment the
// author wrote. The migration touches exactly the keys it names and leaves the
// rest of the file byte-identical.
//
// The edits are:
//
//   - the root `version:` becomes the current schema version;
//   - the retired `coupling.gate.min_band` / `max_drop` knobs are removed, with
//     the comment lines immediately above them (a comment directly above a key
//     documents that key, and leaving it behind would document a key that is
//     gone);
//   - a `distributed_monolith` stanza in warn mode replaces them in place —
//     unless the file already declares one, in which case the author's stanza
//     stands and only the retired keys go.
//
// It never infers `mode: fail`. Fail blocks a build, so switching it on stays an
// owner decision taken after a report-only run against a comparable reference.
//
// Running it twice is byte-idempotent: after the first pass the version is
// current and the retired keys are gone, so the second pass finds nothing.
func MigrateToV2(src []byte) MigrationResult {
	out := MigrationResult{
		SchemaVersion: MigrationSchemaVersion,
		Status:        MigrationAlreadyCurrent,
		ToVersion:     TargetSchemaVersion,
		Changes:       []MigrationChange{},
		PolicyChanges: []string{},
		Output:        src,
	}
	lines := strings.Split(string(src), "\n")
	hasStanza := false
	for _, line := range lines {
		if existingStanza.MatchString(line) {
			hasStanza = true
			break
		}
	}
	kept := make([]string, 0, len(lines)+6)
	gateIndent := ""
	// insertAt is where the FIRST retired key stood. The replacement stanza
	// goes exactly there, so it lands inside the gate block it replaces rather
	// than beside some later stanza that happens to share the indentation.
	insertAt := -1

	for _, line := range lines {
		if m := topLevelVersion.FindStringSubmatch(line); m != nil {
			from, _ := strconv.Atoi(m[1])
			out.FromVersion = from
			// Only ever move the version forward. A file declaring a NEWER
			// schema than this build understands is "upgrade archfit", which
			// validateSchemaVersion says; silently rewriting it to 2 would
			// answer a question the migration was never asked.
			if from >= TargetSchemaVersion {
				kept = append(kept, line)
				continue
			}
			kept = append(kept, "version: "+strconv.Itoa(TargetSchemaVersion)+trailingComment(m[2]))
			out.Changes = append(out.Changes, MigrationChange{Key: "version", Action: migrationSet,
				Detail: strconv.Itoa(from) + " → " + strconv.Itoa(TargetSchemaVersion)})
			continue
		}
		if m := retiredGateKey.FindStringSubmatch(line); m != nil {
			kept = dropLeadingComments(kept, m[1])
			if insertAt < 0 {
				gateIndent, insertAt = m[1], len(kept)
			}
			out.Changes = append(out.Changes, MigrationChange{Key: "coupling.gate." + m[2],
				Action: migrationRemoved, Detail: "retired in schema v" + strconv.Itoa(TargetSchemaVersion)})
			if len(out.PolicyChanges) == 0 {
				out.PolicyChanges = append(out.PolicyChanges,
					"the repository coupling scalar no longer gates the verdict; the replacement rule counts "+
						"newly introduced distributed-monolith seams and is written in warn mode, so this "+
						"migration cannot make a previously passing build fail")
			}
			continue
		}
		kept = append(kept, line)
	}

	// A later retired key at a shallower indent can let dropLeadingComments pop
	// lines from BELOW the recorded position, leaving insertAt past the end.
	// Clamp rather than slice out of range: the stanza then lands at the end of
	// the block instead of panicking on a file the user cannot currently load.
	if insertAt > len(kept) {
		insertAt = len(kept)
	}
	if insertAt >= 0 && !hasStanza {
		kept = insertDistributedMonolith(kept, gateIndent, insertAt)
		out.Changes = append(out.Changes, MigrationChange{Key: "coupling.gate.distributed_monolith",
			Action: migrationInserted, Detail: "mode: warn, max_new_seams: 0"})
	}
	if len(out.Changes) == 0 {
		return out
	}
	out.Status = MigrationRequired
	out.Output = []byte(strings.Join(kept, "\n"))
	return out
}

// insertDistributedMonolith splices the replacement stanza in at the position
// the first retired key occupied.
func insertDistributedMonolith(lines []string, indent string, at int) []string {
	stanza := []string{
		indent + "distributed_monolith:",
		indent + "  # warn is diagnostic. fail blocks only on seams newly introduced against a",
		indent + "  # comparable reference, so switch it on only after a report-only run shows",
		indent + "  # the seam count you expect.",
		indent + "  mode: warn",
		indent + "  max_new_seams: 0",
	}
	out := make([]string, 0, len(lines)+len(stanza))
	out = append(out, lines[:at]...)
	out = append(out, stanza...)
	return append(out, lines[at:]...)
}

// dropLeadingComments removes the run of comment lines immediately above a
// removed key at the same indentation. They document the key being removed.
func dropLeadingComments(kept []string, indent string) []string {
	for len(kept) > 0 {
		last := kept[len(kept)-1]
		if !strings.HasPrefix(last, indent+"#") {
			break
		}
		kept = kept[:len(kept)-1]
	}
	return kept
}

// trailingComment preserves an inline comment on the version line.
func trailingComment(c string) string {
	if c == "" {
		return ""
	}
	return " " + c
}
