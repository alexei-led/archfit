package agenttask

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
)

const (
	advisoryTaskFileCap = 8
)

// BuildAdvisoryTasks converts grouped Balanced-Coupling advisories into a
// deterministic report-only work queue. Single edges stay as findings, and
// gate findings stay in agent_tasks[].
func BuildAdvisoryTasks(findings []finding.Finding, validation []string) []result.AdvisoryTask {
	tasks := make([]result.AdvisoryTask, 0)
	for _, f := range findings {
		if f.Kind != finding.KindAdvisory || f.RuleID != finding.RuleIDBCImbalanced {
			continue
		}
		groupCount, err := strconv.Atoi(f.MatchedBy["group_count"])
		if err != nil || groupCount <= 1 {
			continue
		}
		tasks = append(tasks, result.AdvisoryTask{
			FindingID: f.ID, RuleID: f.RuleID, Status: f.Status, Severity: f.Severity,
			GroupCount: groupCount, GroupMembers: splitGroupMembers(f.MatchedBy["group_members"]),
			Goal: advisoryTaskGoal(f, groupCount), CheapestMove: f.MatchedBy["cheapest_move"],
			ScoreValue: parseScoreValue(f.MatchedBy["score_value"]), TopFiles: advisoryTaskFiles(f),
			Constraints: advisoryTaskConstraints(f), Validation: append([]string(nil), validation...),
		})
	}
	return tasks
}

func advisoryTaskGoal(f finding.Finding, groupCount int) string {
	from, to := f.Edge.From.Module, f.Edge.To.Module
	if from == "" {
		from = f.Edge.From.Path
	}
	if to == "" {
		to = f.Edge.To.Path
	}
	if from == "" && to == "" {
		return fmt.Sprintf("Review %d same-shape Balanced-Coupling advisory edges and reduce the coupling risk without changing gate policy.", groupCount)
	}
	return fmt.Sprintf("Review %d same-shape Balanced-Coupling advisory edges from %s to %s and reduce the coupling risk without changing gate policy.", groupCount, from, to)
}

func advisoryTaskConstraints(f finding.Finding) []string {
	constraints := []string{
		"report-only advisory; do not promote to a gate unless coupling.gate policy changes",
		"keep agent_tasks[] reserved for active gate findings",
	}
	shape := advisoryTaskShape(f)
	if shape != "" {
		constraints = append(constraints, "preserve or improve coupling shape: "+shape)
	}
	if f.MatchedBy["cheapest_move"] != "" {
		constraints = append(constraints, "prefer cheapest_move: "+f.MatchedBy["cheapest_move"])
	}
	if strings.TrimSpace(f.Constraint) != "" {
		constraints = append(constraints, f.Constraint)
	}
	return constraints
}

func advisoryTaskShape(f finding.Finding) string {
	parts := make([]string, 0, 3)
	for _, key := range []string{"strength", "distance", "volatility"} {
		if value := f.MatchedBy[key]; value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	return strings.Join(parts, ", ")
}

func advisoryTaskFiles(f finding.Finding) []string {
	seen := map[string]struct{}{}
	files := make([]string, 0, advisoryTaskFileCap)
	add := func(file string) {
		file = strings.TrimSpace(file)
		if file == "" {
			return
		}
		if _, ok := seen[file]; ok {
			return
		}
		seen[file] = struct{}{}
		files = append(files, file)
	}
	locFiles := make([]string, 0, len(f.Locations))
	for _, loc := range f.Locations {
		if loc.File != "" {
			locFiles = append(locFiles, loc.File)
		}
	}
	sort.Strings(locFiles)
	for _, file := range locFiles {
		add(file)
	}
	if len(files) == 0 {
		add(f.Edge.From.Path)
		add(f.Edge.To.Path)
		sort.Strings(files)
	}
	if len(files) > advisoryTaskFileCap {
		return files[:advisoryTaskFileCap]
	}
	return files
}

func splitGroupMembers(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseScoreValue(raw string) int {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}
