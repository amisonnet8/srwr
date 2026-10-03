package uicheck

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
)

// Shot is what the extension showed for one frame (extension/test/capture.ts): a JSON object. Only these keys are compared;
// the others (the document text, the decorations) are not part of what a person looks at in the comparison.
type Shot map[string]any

// shotKeys are the keys of a shot that are compared, as extension/test/capture.test.ts does.
var shotKeys = []string{"label", "frame", "tabs", "tree", "status", "viewDescription", "quickPick"}

// ReadShots reads a file of shots (an array).
func ReadShots(data []byte) ([]Shot, error) {
	var s []Shot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return s, nil
}

// ShotDiff is how one frame of the extension differs from the baseline.
type ShotDiff struct {
	Index int // 1-based; 0 when the number of frames differs
	Label string
	Diffs []string // one per key that differs
}

// CompareShots compares what the extension showed with the baseline. Nothing returned means they are the same.
func CompareShots(want, got []Shot) []ShotDiff {
	if len(want) != len(got) {
		return []ShotDiff{{Diffs: []string{fmt.Sprintf("%d frames were shown, the baseline has %d", len(got), len(want))}}}
	}
	var out []ShotDiff
	for i, w := range want {
		g := got[i]
		var diffs []string
		for _, k := range shotKeys {
			if reflect.DeepEqual(w[k], g[k]) {
				continue
			}
			if k == "tabs" {
				diffs = append(diffs, tabDiffs(w[k], g[k])...)
			} else {
				diffs = append(diffs, fmt.Sprintf("%s: want %s, got %s", k, short(w[k]), short(g[k])))
			}
		}
		if len(diffs) > 0 {
			label, _ := w["label"].(string)
			if label == "" {
				label = fmt.Sprintf("frame %v", w["frame"])
			}
			out = append(out, ShotDiff{Index: i + 1, Label: label, Diffs: diffs})
		}
	}
	return out
}

// short is a value for a message: JSON, cut when long.
func short(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	if r := []rune(string(b)); len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return string(b)
}

// tabDiffs says what differs in the editors that were open: the document, where it was scrolled to, the options, and the
// decorations line by line (the decoration is shown as its colors; the line number labels are shown as they are).
func tabDiffs(want, got any) []string {
	w, _ := want.([]any)
	g, _ := got.([]any)
	if len(w) != len(g) {
		return []string{fmt.Sprintf("tabs: %d editors were open, the baseline has %d", len(g), len(w))}
	}
	var out []string
	for i := range w {
		wt, _ := w[i].(map[string]any)
		gt, _ := g[i].(map[string]any)
		name := fmt.Sprintf("editor %d (%v)", i+1, wt["uri"])
		for _, k := range []string{"uri", "column", "options", "reveal"} {
			if !reflect.DeepEqual(wt[k], gt[k]) {
				out = append(out, fmt.Sprintf("%s %s: want %s, got %s", name, k, short(wt[k]), short(gt[k])))
			}
		}
		if wtext, gtext := fmt.Sprint(wt["text"]), fmt.Sprint(gt["text"]); wtext != gtext {
			out = append(out, fmt.Sprintf("%s document: %s", name, firstLineDiff(wtext, gtext)))
		}
		gone, added := decorationSet(wt["decorations"]), decorationSet(gt["decorations"])
		var minus, plus []string
		for d := range gone {
			if !added[d] {
				minus = append(minus, d)
			}
		}
		for d := range added {
			if !gone[d] {
				plus = append(plus, d)
			}
		}
		sort.Strings(minus)
		sort.Strings(plus)
		if len(minus)+len(plus) > 0 {
			out = append(out, fmt.Sprintf("%s decorations: baseline only %s; now only %s", name, strings.Join(first(minus, 3), " | "), strings.Join(first(plus, 3), " | ")))
		}
	}
	return out
}

// decorationSet lists the decorations as strings: "line <n> <options>".
func decorationSet(v any) map[string]bool {
	out := map[string]bool{}
	list, _ := v.([]any)
	for _, d := range list {
		m, _ := d.(map[string]any)
		opts, _ := json.Marshal(m["opts"])
		ranges, _ := m["ranges"].([]any)
		for _, r := range ranges {
			rm, _ := r.(map[string]any)
			if rm == nil {
				continue
			}
			if before, ok := rm["before"]; ok {
				out[fmt.Sprintf("line %v before %q", rm["line"], before)] = true
				continue
			}
			out[fmt.Sprintf("line %v %s", rm["line"], opts)] = true
		}
	}
	return out
}

func first(s []string, n int) []string {
	if len(s) > n {
		return append(slices.Clone(s[:n]), fmt.Sprintf("…(%d more)", len(s)-n))
	}
	return s
}

// firstLineDiff names the first line where two texts differ.
func firstLineDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b || i >= len(w) || i >= len(g) {
			return fmt.Sprintf("line %d: want %q, got %q", i+1, a, b)
		}
	}
	return "differs"
}
