package engine

import (
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/graph"
)

const unknownStrength = "unknown"

type semanticStrengthOverlay struct {
	byLanguage map[string]evidence.SemanticStrengthOverlayStats
}

func newSemanticStrengthOverlay() *semanticStrengthOverlay {
	return &semanticStrengthOverlay{byLanguage: make(map[string]evidence.SemanticStrengthOverlayStats)}
}

func (s *semanticStrengthOverlay) addCandidate(language, before string) {
	stats := s.statsFor(language)
	stats.CandidateEdges++
	stats.Before[strengthBucket(before)]++
	s.byLanguage[language] = stats
}

func (s *semanticStrengthOverlay) finishCandidate(language, after string, applied bool) {
	stats := s.statsFor(language)
	if applied {
		stats.Applied++
	} else {
		stats.Missed++
	}
	stats.After[strengthBucket(after)]++
	s.byLanguage[language] = stats
}

func (s *semanticStrengthOverlay) merge(other *semanticStrengthOverlay) {
	if other == nil {
		return
	}
	for language, src := range other.byLanguage {
		dst := s.statsFor(language)
		dst.CandidateEdges += src.CandidateEdges
		dst.Applied += src.Applied
		dst.Missed += src.Missed
		mergeCountMap(dst.Before, src.Before)
		mergeCountMap(dst.After, src.After)
		s.byLanguage[language] = dst
	}
}

func (s *semanticStrengthOverlay) report() *evidence.SemanticStrengthOverlay {
	if s == nil || len(s.byLanguage) == 0 {
		return nil
	}
	byLanguage := make(map[string]evidence.SemanticStrengthOverlayStats, len(s.byLanguage))
	for language, stats := range s.byLanguage {
		byLanguage[language] = evidence.SemanticStrengthOverlayStats{
			CandidateEdges: stats.CandidateEdges,
			Applied:        stats.Applied,
			Missed:         stats.Missed,
			Before:         copyCountMap(stats.Before),
			After:          copyCountMap(stats.After),
		}
	}
	return &evidence.SemanticStrengthOverlay{ByLanguage: byLanguage}
}

func (s *semanticStrengthOverlay) statsFor(language string) evidence.SemanticStrengthOverlayStats {
	stats := s.byLanguage[language]
	if stats.Before == nil {
		stats.Before = make(map[string]int)
	}
	if stats.After == nil {
		stats.After = make(map[string]int)
	}
	return stats
}

func mergeCountMap(dst, src map[string]int) {
	for k, v := range src {
		dst[k] += v
	}
}

func copyCountMap(src map[string]int) map[string]int {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func strengthBucket(strength string) string {
	if strength == "" {
		return unknownStrength
	}
	return strength
}

func tracksSemanticStrengthOverlay(cov evidence.Coverage) bool {
	switch cov.Status {
	case evidence.StatusOK, evidence.StatusPartial:
		return true
	default:
		return false
	}
}

func isSemanticOverlayLanguage(language string) bool {
	switch language {
	case graph.LangTypeScript, graph.LangPython, graph.LangRust:
		return true
	default:
		return false
	}
}
