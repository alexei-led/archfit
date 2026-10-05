package reporttest

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The string rules a strict state-report consumer (the archfit App's decoder)
// applies to every kept string: no control character and no line or paragraph
// separator in any of them, at most appTextRunes in free text, and at most
// appTaskTextRunes in an agent task's goal and constraints.
const (
	appTextRunes     = 500
	appTaskTextRunes = 4096
)

// droppedByConsumer names the state keys the App does not keep (matched_by and
// task declarations carry source identifiers; seams keep only their identity
// and coupling ratings). Their strings still may not break a line, but they
// carry no length cap.
var droppedByConsumer = map[string]struct{}{"matched_by": {}, "declarations": {}, "seams": {}}

// taskTextKeys are the agent-task fields the consumer caps at appTaskTextRunes.
var taskTextKeys = map[string]struct{}{"goal": {}, "constraints": {}}

// AppTextViolations walks an archfit.architecture-state.v1 document and lists
// every string a strict consumer would reject, as "path: rule". An empty result
// means every string is a single line within its cap.
func AppTextViolations(stateJSON []byte) ([]string, error) {
	var doc any
	if err := json.Unmarshal(stateJSON, &doc); err != nil {
		return nil, fmt.Errorf("decode state: %w", err)
	}
	var out []string
	walkAppText(doc, "", appTextRunes, false, &out)
	sort.Strings(out)
	return out, nil
}

func walkAppText(v any, path string, limit int, dropped bool, out *[]string) {
	switch t := v.(type) {
	case map[string]any:
		for key, child := range t {
			childLimit, childDropped := limit, dropped
			if _, ok := droppedByConsumer[key]; ok {
				childDropped = true
			}
			if _, ok := taskTextKeys[key]; ok && strings.HasPrefix(path, ".agent_tasks[") {
				childLimit = appTaskTextRunes
			}
			walkAppText(child, path+"."+key, childLimit, childDropped, out)
		}
	case []any:
		for i, child := range t {
			walkAppText(child, fmt.Sprintf("%s[%d]", path, i), limit, dropped, out)
		}
	case string:
		if strings.ContainsFunc(t, func(r rune) bool { return unicode.IsControl(r) || r == ' ' || r == ' ' }) {
			*out = append(*out, strings.TrimPrefix(path, ".")+": must be a single line without control characters")
		}
		if !dropped && utf8.RuneCountInString(t) > limit {
			*out = append(*out, fmt.Sprintf("%s: %d runes exceeds %d", strings.TrimPrefix(path, "."), utf8.RuneCountInString(t), limit))
		}
	}
}
