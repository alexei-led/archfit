package report

import (
	"strings"
)

// DirtySuffix marks a head tree digest measured with uncommitted edits.
const DirtySuffix = "+dirty"

// shortDigestLen is how much of a digest a human line shows.
const shortDigestLen = 12

// IdentitySchemaVersion identifies the result-identity block.
const IdentitySchemaVersion = "archfit.result-identity.v1"

// ResultIdentity names what a result was judged under, so a consumer can tell
// two results apart and the App can compare them with its own trusted values.
// A digest or SHA the run could not know is omitted: absent is not empty.
type ResultIdentity struct {
	// SchemaVersion is always archfit.result-identity.v1.
	SchemaVersion string `json:"schema_version"`
	// PolicyDigest is the sha256 of the policy file's bytes (.archfit.yaml).
	PolicyDigest string `json:"policy_digest,omitempty"`

	// PolicyRef is the ref and commit whose policy files were used, when the
	// run read them from a target branch instead of the working tree.
	PolicyRef *PolicyRef `json:"policy_ref,omitempty"`
	// BaseSHA is the commit the change was compared against.
	BaseSHA string `json:"base_sha,omitempty"`
	// HeadTreeDigest is the git tree id of the measured head. A working tree
	// with uncommitted edits has the tree id of HEAD plus a "+dirty" suffix.
	HeadTreeDigest string `json:"head_tree_digest,omitempty"`
	// EngineVersion is the archfit build that produced the result.
	EngineVersion string `json:"engine_version"`
}

// PolicyRef is a git ref and the commit it named when the policy was read.
type PolicyRef struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

// Line renders the identity as one short line for a footer or a hook message.
func (i ResultIdentity) Line() string {
	parts := []string{"engine " + i.EngineVersion}
	for _, d := range []struct{ name, digest string }{
		{"policy", i.PolicyDigest},
		{"tree", i.HeadTreeDigest},
	} {
		parts = append(parts, d.name+" "+shortDigest(d.digest))
	}
	if i.BaseSHA != "" {
		parts = append(parts, "base "+shortDigest(i.BaseSHA))
	}
	return strings.Join(parts, " · ")
}

func shortDigest(d string) string {
	if d == "" {
		return "absent"
	}
	dirty := strings.HasSuffix(d, DirtySuffix)
	d = strings.TrimSuffix(d, DirtySuffix)
	d = d[:min(shortDigestLen, len(d))]
	if dirty {
		d += DirtySuffix
	}
	return d
}
