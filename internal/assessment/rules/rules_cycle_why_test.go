package rules_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alexei-led/archfit/v3/internal/assessment/rules"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

// maxConsumerWhy is the why length a report consumer accepts before it rejects
// the whole report. A cycle finding must stay under it however large the cycle.
const maxConsumerWhy = 500

func assertBoundedWhy(t *testing.T, why string) {
	t.Helper()
	if len(why) > maxConsumerWhy || utf8.RuneCountInString(why) > maxConsumerWhy {
		t.Errorf("why is %d bytes / %d runes, want <= %d: %s", len(why), utf8.RuneCountInString(why), maxConsumerWhy, why)
	}
}

// TestModuleCycle_WhyNamesThePairAndSizeOnly pins a bounded why on a large
// strongly-connected component: it names the pair and the cycle size, and the
// members stay in matched_by.cycle_modules.
func TestModuleCycle_WhyNamesThePairAndSizeOnly(t *testing.T) {
	const size = 30
	modules := make([]string, size)
	for i := range modules {
		modules[i] = fmt.Sprintf("capability-with-a-long-module-name-%02d", i)
	}
	edges := make([]moduleTestEdge, 0, size)
	for i, from := range modules {
		to := modules[(i+1)%size]
		edges = append(edges, moduleTestEdge{"file:" + from + "/a.go", "package:" + to, from, to, nil})
	}
	findings := checkProduction(newModuleCycleRule(t, "", modules...), moduleSet(edges...))
	if len(findings) != size {
		t.Fatalf("got %d findings, want %d", len(findings), size)
	}
	for _, f := range findings {
		assertBoundedWhy(t, f.Why)
		for _, want := range []string{f.Edge.From.Module, f.Edge.To.Module, strconv.Itoa(size)} {
			if !strings.Contains(f.Why, want) {
				t.Errorf("why lacks %q: %s", want, f.Why)
			}
		}
		if got := strings.Count(f.MatchedBy["cycle_modules"], ", ") + 1; got != size {
			t.Errorf("matched_by.cycle_modules lists %d members, want %d", got, size)
		}
	}
}

// TestCycleRule_WhyStaysBoundedOnALargeCycle pins the node-level cycle why: a
// small cycle lists every member, a large one lists the first members and how
// many more, and matched_by keeps the full list.
func TestCycleRule_WhyStaysBoundedOnALargeCycle(t *testing.T) {
	ring := func(size int) relationship.Set {
		edges := make([]testEdge, 0, size)
		for i := range size {
			from := fmt.Sprintf("file:src/features/a-rather-deep/directory/tree/component_%02d.ts", i)
			to := fmt.Sprintf("file:src/features/a-rather-deep/directory/tree/component_%02d.ts", (i+1)%size)
			edges = append(edges, testEdge{From: from, To: to, Kind: edgeKindImports})
		}
		return makeGraph(edges)
	}

	small := newCycleRule(t, "").Check(ring(2), rules.Evidence{})
	if len(small) != 1 {
		t.Fatalf("got %d findings, want 1", len(small))
	}
	if want := "Import cycle detected among 2 nodes: " + strings.ReplaceAll(small[0].MatchedBy["cycle_members"], ", ", " → "); small[0].Why != want {
		t.Errorf("small cycle why = %q, want every member: %q", small[0].Why, want)
	}

	large := newCycleRule(t, "").Check(ring(30), rules.Evidence{})
	if len(large) != 1 {
		t.Fatalf("got %d findings, want 1", len(large))
	}
	why := large[0].Why
	assertBoundedWhy(t, why)
	if !strings.Contains(why, "among 30 nodes") || !strings.Contains(why, "more") {
		t.Errorf("large cycle why = %q, want the size and how many members it omits", why)
	}
	if got := strings.Count(large[0].MatchedBy["cycle_members"], ", ") + 1; got != 30 {
		t.Errorf("matched_by.cycle_members lists %d members, want 30", got)
	}
}
