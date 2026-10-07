package acquisition

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/policy"
)

// ClassificationHash fingerprints the policy leaves that change the compared
// facts without being modules: the volatility cascade, the duplicated-knowledge
// mode, the declared external systems, the function-size threshold, and the
// metrics switched off with `enabled: false`.
//
// Comparability reads this hash, never the raw config bytes. Comments, waivers,
// rules, gates and `reviewed_at` decide findings and what blocks, not what is
// measured, so editing them must not make a stored reference non-comparable.
// Absent and explicit-default values hash the same, because the snapshot is
// already normalized. Every config leaf has one class; see internal/config/classification_test.go.
func ClassificationHash(p policy.PolicySnapshot) string {
	h := sha256.New()
	write := func(parts ...string) {
		_, _ = io.WriteString(h, strings.Join(parts, "\x01")+"\x00")
	}
	write("cascade", strconv.FormatBool(p.Relationship.VolatilityCascadeEnabled))
	write("duplicated_knowledge", string(policy.NormalizeDuplicatedKnowledgePolicy(p.Relationship.DuplicatedKnowledge)))
	write("function_loc_threshold", strconv.Itoa(p.Assessment.FunctionLOCThreshold))
	disabled := make([]string, 0, len(p.Gates.Metrics))
	for name, cfg := range p.Gates.Metrics {
		if cfg.Enabled != nil && !*cfg.Enabled {
			disabled = append(disabled, name)
		}
	}
	sort.Strings(disabled)
	write("disabled_metrics", strings.Join(disabled, "\x02"))
	names := make([]string, 0, len(p.Topology.ExternalSystems))
	for name := range p.Topology.ExternalSystems {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		def := p.Topology.ExternalSystems[name]
		volatility := def.Volatility
		if volatility == "" {
			volatility = "low" // the documented default
		}
		targets := slices.Clone(def.Targets)
		sort.Strings(targets)
		write("external", name, volatility, strings.Join(targets, "\x02"))
	}
	return hex.EncodeToString(h.Sum(nil))
}
