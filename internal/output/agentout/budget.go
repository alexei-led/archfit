package agentout

// The size budget keeps the result small enough for an agent's context and a
// hook's stderr. No ID, path, edge, or validation command is ever shortened:
// they are identity material an agent copies verbatim. Free text is shortened,
// and over budget the lists are capped; every dropped entry is counted, in
// Omitted or on its repair.
const (
	// budgetBytes is the most the encoded result may take.
	budgetBytes = 8 * 1024
	// textRunes bounds every free-text string, the same bound the report uses
	// for its free text, so the App limit (500) always holds.
	textRunes = 400
	// tightRunes bounds free text once the result is over budget.
	tightRunes = 200
	// tightListLen bounds a repair's locations, files, and constraints once
	// the result is over budget. A module-pair repair can name fifty import
	// sites.
	tightListLen = 5
)

// fit encodes the result within the budget. It cuts free text first, then
// moves tail repairs and tail unevaluated rules into Omitted. The header and
// the first repair always stay, so a result whose header and first repair
// alone exceed the budget (paths thousands of characters long) is returned
// over it. The next action and the summary were decided on the full result,
// so the budget can never change them.
func fit(r Result) ([]byte, error) {
	r = bound(r, textRunes, 0)
	out, err := encode(r)
	if err != nil || len(out) <= budgetBytes {
		return out, err
	}
	r = bound(r, tightRunes, tightListLen)
	r.Truncated = true
	for {
		out, err = encode(r)
		if err != nil || len(out) <= budgetBytes {
			return out, err
		}
		switch {
		case len(r.Repairs) > 1:
			r.Repairs = r.Repairs[:len(r.Repairs)-1]
			r.Omitted.Repairs++
		case len(r.UnevaluatedRules) > 0:
			r.UnevaluatedRules = r.UnevaluatedRules[:len(r.UnevaluatedRules)-1]
			r.Omitted.UnevaluatedRules++
		default:
			return out, nil
		}
	}
}

// bound cuts free text to maxRunes and, when maxList is positive, a repair's
// locations, files, and constraints to maxList entries, counting the rest on
// the repair. It sets Truncated when it cut
// anything. It copies every slice it changes, so the caller's result is
// unchanged.
func bound(r Result, maxRunes, maxList int) Result {
	cut := func(s string) string {
		out := cutRunes(s, maxRunes)
		if out != s {
			r.Truncated = true
		}
		return out
	}
	r.Summary = cut(r.Summary)
	repairs := make([]Repair, len(r.Repairs))
	for i, rep := range r.Repairs {
		rep.Goal = cut(rep.Goal)
		if maxList > 0 {
			rep.At = capList(rep.At, maxList, &rep.AtOmitted, &r.Truncated)
			rep.Edit = capList(rep.Edit, maxList, &rep.EditOmitted, &r.Truncated)
			rep.Constraints = capList(rep.Constraints, maxList, &rep.ConstraintsOmitted, &r.Truncated)
		}
		constraints := make([]string, len(rep.Constraints))
		for j, c := range rep.Constraints {
			constraints[j] = cut(c)
		}
		rep.Constraints = constraints
		repairs[i] = rep
	}
	r.Repairs = repairs
	rules := make([]UnevaluatedRule, len(r.UnevaluatedRules))
	for i, rule := range r.UnevaluatedRules {
		rule.Reason = cut(rule.Reason)
		rules[i] = rule
	}
	r.UnevaluatedRules = rules
	return r
}

// capList keeps the first maxList entries and counts the rest.
func capList[T any](list []T, maxList int, omitted *int, truncated *bool) []T {
	if len(list) <= maxList {
		return list
	}
	*omitted += len(list) - maxList
	*truncated = true
	return list[:maxList]
}

// cutRunes keeps at most n runes, ending a cut string in "…".
func cutRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
