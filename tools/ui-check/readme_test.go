package main

import (
	"strconv"
	"strings"
	"testing"
)

func TestDemoScenarioIsConsistent(t *testing.T) {
	if len(demoHolds) != len(demoSteps)+1 {
		t.Fatalf("%d holds for %d steps and the final diff", len(demoHolds), len(demoSteps))
	}
	lines := strings.Split(strings.TrimSuffix(demoSource, "\n"), "\n")
	for i, s := range demoSteps {
		switch s.kind {
		case "look":
			if s.start < 1 || s.end < s.start || s.end > len(lines) {
				t.Errorf("step %d selects %d-%d of %d lines", i, s.start, s.end, len(lines))
			}
		case "edit":
			if i == 0 || demoSteps[i-1].kind != "look" || s.newText == "" {
				t.Errorf("step %d must replace what the look before it declared", i)
			}
			prev := demoSteps[i-1]
			lines = splice(lines, prev.start, prev.end, strings.Split(s.newText, "\n"))
		default:
			t.Errorf("step %d is %q", i, s.kind)
		}
		if strings.TrimSpace(s.why) == "" {
			t.Errorf("step %d has no why", i)
		}
	}
}

func TestSplice(t *testing.T) {
	got := splice([]string{"a", "b", "c", "d"}, 2, 3, []string{"x", "y", "z"})
	if strings.Join(got, ",") != "a,x,y,z,d" {
		t.Errorf("splice = %v", got)
	}
}

func TestWrapWhy(t *testing.T) {
	rows := wrapWhy("◆ one two three four five", 12)
	if strings.Join(rows, "|") != "◆ one two|  three four|  five" {
		t.Errorf("rows = %q", rows)
	}
	for _, r := range wrapWhy("◆ "+demoSteps[1].why, whyWrap) {
		if len([]rune(r)) > whyWrap+2 {
			t.Errorf("a row of %d characters: %q", len([]rune(r)), r)
		}
	}
}

func TestVSCodeFrame(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		for _, current := range []int{0, 1, 2, 3} {
			svg := vscodeFrame(theme, current)
			if !strings.HasPrefix(svg, "<svg ") || !strings.HasSuffix(svg, "</svg>\n") {
				t.Errorf("%s %d: not an image", theme, current)
			}
			wantBanner := "#0b61a4"
			if demoSteps[current].kind == "edit" {
				wantBanner = "#b45f06"
			}
			if !strings.Contains(svg, `fill="`+wantBanner+`"/>`) {
				t.Errorf("%s frame %d: no reason row of %s", theme, current+1, wantBanner)
			}
			if !strings.Contains(svg, "main.go:9-14") || !strings.Contains(svg, ">"+strconv.Itoa(current+1)+"/5<") {
				t.Errorf("%s frame %d: the list or the position is missing", theme, current+1)
			}
		}
	}
	// A replace shows the file after it: the new lines are in the picture, and the old return is not.
	svg := vscodeFrame("dark", 1)
	if !strings.Contains(svg, "stranger") || strings.Contains(svg, "&#34;hello &#34;+ name") {
		t.Error("the picture of the replace should show the new lines")
	}
}
