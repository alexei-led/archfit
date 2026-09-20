package decision

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

// CompareMeasurementProfiles returns the named reasons a delta is inadmissible.
// External versions must match exactly until an equivalence is verified here.
func CompareMeasurementProfiles(head, base *evidence.MeasurementProfile) []string {
	reasons := append(profileUnknowns("head", head), profileUnknowns("reference", base)...)
	if len(reasons) != 0 {
		return reasons
	}
	if head.SettingsHash != base.SettingsHash {
		reasons = append(reasons, "measurement_profile.settings_hash differs between the two runs")
	}
	h, b := profileProducers(head), profileProducers(base)
	tools := make(map[string]struct{}, len(h)+len(b))
	for tool := range h {
		tools[tool] = struct{}{}
	}
	for tool := range b {
		tools[tool] = struct{}{}
	}
	names := make([]string, 0, len(tools))
	for tool := range tools {
		names = append(names, tool)
	}
	sort.Strings(names)
	for _, tool := range names {
		hp, hok := h[tool]
		bp, bok := b[tool]
		switch {
		case !hok || !bok:
			reasons = append(reasons, fmt.Sprintf("measurement_profile producer %s is missing from one run", tool))
		case hp.Status != bp.Status:
			reasons = append(reasons, fmt.Sprintf("measurement_profile producer %s availability differs (%s vs %s)", tool, hp.Status, bp.Status))
		case hp.PartialBasis != bp.PartialBasis:
			reasons = append(reasons, fmt.Sprintf("measurement_profile producer %s partial evidence basis differs", tool))
		case hp.SemanticsVersion != bp.SemanticsVersion:
			reasons = append(reasons, fmt.Sprintf("measurement_profile producer %s semantics_version differs", tool))
		case hp.ToolVersion != bp.ToolVersion:
			reasons = append(reasons, fmt.Sprintf("measurement_profile producer %s tool_version differs (%s vs %s)", tool, hp.ToolVersion, bp.ToolVersion))
		}
	}
	return reasons
}

func profileUnknowns(side string, p *evidence.MeasurementProfile) []string {
	if p == nil {
		return []string{"measurement_profile is missing from " + side}
	}
	var reasons []string
	if p.Version != evidence.MeasurementProfileVersion {
		reasons = append(reasons, fmt.Sprintf("measurement_profile version %q is unsupported in %s", p.Version, side))
	}
	if p.SettingsHash == "" {
		reasons = append(reasons, "measurement_profile.settings_hash is missing from "+side)
	}
	if len(p.Producers) == 0 {
		reasons = append(reasons, "measurement_profile producers are missing from "+side)
	}
	seen := make(map[string]bool)
	for _, producer := range p.Producers {
		semantics, external := evidence.MeasurementContract(producer.Tool)
		if semantics == "" || producer.SemanticsVersion != semantics || seen[producer.Tool] {
			reasons = append(reasons, fmt.Sprintf("measurement_profile producer %q is incomplete or duplicated in %s", producer.Tool, side))
		}
		if external && (producer.Status == evidence.StatusOK || producer.Status == evidence.StatusPartial) && !knownProducerVersion(producer.Tool, producer.ToolVersion) {
			reasons = append(reasons, fmt.Sprintf("measurement_profile producer %s tool_version is unknown in %s", producer.Tool, side))
		}
		seen[producer.Tool] = true
		switch producer.Status {
		case evidence.StatusOK, evidence.StatusAbsent, evidence.StatusDisabled:
			if producer.PartialBasis != "" {
				reasons = append(reasons, fmt.Sprintf("measurement_profile producer %s has a partial basis without partial status in %s", producer.Tool, side))
			}
		case evidence.StatusPartial:
			if !comparablePartialProducer(producer) {
				reasons = append(reasons, fmt.Sprintf("measurement_profile producer %s has incomplete evidence (%s) in %s", producer.Tool, producer.Status, side))
			}
		default:
			reasons = append(reasons, fmt.Sprintf("measurement_profile producer %s has incomplete evidence (%s) in %s", producer.Tool, producer.Status, side))
		}
	}
	for _, unknown := range p.Unknowns {
		reasons = append(reasons, fmt.Sprintf("measurement_profile %s: %s", side, strings.TrimSpace(unknown)))
	}
	return reasons
}

func comparablePartialProducer(p evidence.MeasurementProducer) bool {
	switch p.PartialBasis {
	case evidence.PartialUnresolvedSpecifiers:
		return p.Tool == toolDepCruiser || p.Tool == toolGrimp
	case evidence.PartialDegradedPrecision:
		return p.Tool == "go/packages"
	default:
		return false
	}
}

func knownProducerVersion(tool, version string) bool {
	if strings.TrimSpace(version) == "" || version == "unknown" {
		return false
	}
	if tool == "scip" || tool == "scip-symbols" {
		return strings.Contains(version, " ")
	}
	if tool == "grimp" {
		return strings.HasPrefix(version, "grimp ") && strings.Contains(version, "; python ")
	}
	return true
}

func profileProducers(p *evidence.MeasurementProfile) map[string]evidence.MeasurementProducer {
	out := make(map[string]evidence.MeasurementProducer, len(p.Producers))
	for _, producer := range p.Producers {
		out[producer.Tool] = producer
	}
	return out
}
