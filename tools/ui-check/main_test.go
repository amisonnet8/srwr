package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amisonnet8/srwr/internal/uicheck"
)

const fixture = "../../extension/test/fixtures/ui-check"

func TestPrepareWorkspace(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "w")
	if err := os.MkdirAll(dst, 0o750); err != nil {
		t.Fatal(err)
	}
	// Something left from the last time must not stay.
	if err := os.WriteFile(filepath.Join(dst, "old.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareWorkspace(fixture, dst, "/x/bin/srwr", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "old.txt")); err == nil {
		t.Error("a file of the last run is still there")
	}
	for name, f := range tapes {
		if name == "long-why" {
			continue // added by addLongWhy
		}
		if _, err := os.Stat(filepath.Join(dst, ".srwr", "tapes", f.ID+".tape.jsonl")); err != nil {
			t.Errorf("tape %s: %v", f.ID, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "text.go")); err != nil {
		t.Errorf("the real files are needed for the last diff: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dst, ".vscode", "settings.json")) //nolint:gosec // a path in a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]string
	if err := json.Unmarshal(b, &s); err != nil || s["srwr.path"] != "/x/bin/srwr" {
		t.Errorf("settings = %s (%v)", b, err)
	}
}

func TestPrepareWorkspaceLiveHasNoTapes(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "w")
	if err := prepareWorkspace(fixture, dst, "/x/srwr", true); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dst, ".srwr", "tapes"))
	if err != nil || len(entries) != 0 {
		t.Errorf("tapes = %v (%v), want none", entries, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "text.go")); err != nil {
		t.Error("the real files must be there in live too")
	}
}

func TestFixedTapesAreWhereTheNamesSay(t *testing.T) {
	for name, f := range tapes {
		if name == "long-why" {
			continue // made on the spot (TestLongWhyTapeIsMadeByTheRealMCP)
		}
		if _, err := os.Stat(filepath.Join(fixture, ".srwr", "tapes", f.ID+".tape.jsonl")); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestFeedLive(t *testing.T) {
	w := t.TempDir()
	if err := os.MkdirAll(filepath.Join(w, ".srwr", "tapes"), 0o750); err != nil {
		t.Fatal(err)
	}
	tape := filepath.Join(w, ".srwr", "tapes", liveTapeName)
	var atWait int
	var log strings.Builder
	wait := func() {
		b, err := os.ReadFile(tape) //nolint:gosec // a path in a temporary directory
		if err != nil {
			t.Error(err)
		}
		atWait = strings.Count(string(b), "\n")
	}
	if err := feedLive(w, fixture, wait, time.Millisecond, &log); err != nil {
		t.Fatal(err)
	}
	all, err := os.ReadFile(filepath.Join(fixture, ".srwr", "tapes", tapes["why-basic"].ID+".tape.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(tape) //nolint:gosec // a path in a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(all) {
		t.Error("the live tape does not end up equal to the fixed tape")
	}
	// The start holds the header, the snapshots and the first operation, and nothing after it.
	lines := strings.Split(strings.TrimSuffix(string(all), "\n"), "\n")
	if want := firstOperation(lines) + 1; atWait != want || atWait >= len(lines) {
		t.Errorf("%d lines at the start, want %d of %d", atWait, want, len(lines))
	}
}

func TestRunUsage(t *testing.T) {
	var out strings.Builder
	for _, args := range [][]string{nil, {"open"}, {"open", "vim", "why-basic"}, {"open", "vscode", "nope"}} {
		if err := run(args, &out); err == nil {
			t.Errorf("run(%v) succeeded", args)
		}
	}
}

func TestVimGuideCoversEveryTape(t *testing.T) {
	names := []string{"live"}
	for name := range tapes {
		names = append(names, name)
	}
	for _, name := range names {
		steps := vimGuide[name]
		if len(steps) < 3 {
			t.Errorf("%s: %d steps", name, len(steps))
		}
		for i, s := range steps {
			if strings.TrimSpace(s) == "" {
				t.Errorf("%s step %d is empty", name, i+1)
			}
		}
	}
	if !strings.Contains(strings.Join(vimGuide["live"], "\n"), "[[") || !strings.Contains(strings.Join(vimGuide["live"], "\n"), "`L`") {
		t.Error("the live guide must say how to go back and how to return to the latest")
	}
}

func TestSplitLight(t *testing.T) {
	for name, want := range map[string][2]any{"why-basic": {"why-basic", false}, "why-basic-light": {"why-basic", true}, "live": {"live", false}} {
		base, light := splitLight(name)
		if base != want[0] || light != want[1] {
			t.Errorf("splitLight(%q) = %q, %v", name, base, light)
		}
	}
	for _, name := range []string{"why-basic", "external", "no-why", "live"} {
		for _, step := range vimGuide[name] {
			if strings.Contains(step, "background=light") {
				t.Errorf("%s: a step asks the person to type %q; the -light name does it", name, step)
			}
		}
	}
}

func TestVimInitIsThePlainColorsOfTheImages(t *testing.T) {
	dark, light := vimInit(false), vimInit(true)
	if !strings.Contains(dark, "background=dark") || !strings.Contains(dark, "guibg=#1e1e1e") || !strings.Contains(dark, "guifg=#d4d4d4") {
		t.Errorf("dark = %q", dark)
	}
	if !strings.Contains(light, "background=light") || !strings.Contains(light, "guibg=#ffffff") || !strings.Contains(light, "guifg=#1f2328") {
		t.Errorf("light = %q", light)
	}
}

func TestLongWhyTapeIsMadeByTheRealMCP(t *testing.T) {
	bin, err := filepath.Abs("../../bin/srwr")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skip("bin/srwr is not built (qsoku bin)")
	}
	extra := t.TempDir()
	if err := makeLongWhyTape(bin, extra); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(extra, ".srwr", "tapes", longWhyTape+".tape.jsonl")) //nolint:gosec // a path in a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	var whys []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var e struct {
			Type string
			Why  *string
		}
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatal(err)
		}
		types = append(types, e.Type)
		if e.Why != nil {
			whys = append(whys, *e.Why)
		}
	}
	if got := strings.Join(types, ","); got != "header,snapshot,select,replace" {
		t.Errorf("events = %s", got)
	}
	if len(whys) != 2 || whys[0] != longSelectWhy || whys[1] != longReplaceWhy {
		t.Errorf("whys = %q", whys)
	}
	a, err := os.ReadFile(filepath.Join(extra, "a.go")) //nolint:gosec // a path in a temporary directory
	if err != nil || !strings.Contains(string(a), "println") {
		t.Errorf("a.go is not the file as the replace left it: %q %v", a, err)
	}
}

func TestVerifyLongWhy(t *testing.T) {
	const why = "とても長い理由です。折り返されるはず。"
	mk := func(rows ...string) *uicheck.Grid {
		g := uicheck.NewGrid(60, 12, "#1e1e1e")
		for i, r := range rows {
			g.Put(1+i, 0, r, "#ffffff", "#0b61a4")
		}
		return g
	}
	if n, p := verifyLongWhy(mk("◆ とても長い理由です。", "  折り返されるはず。"), why); n != 2 || len(p) != 0 {
		t.Errorf("a good folding: rows %d, problems %v", n, p)
	}
	for name, g := range map[string]*uicheck.Grid{
		"one row":        mk("◆ とても長い理由です。折り返されるはず。"),
		"not indented":   mk("◆ とても長い理由です。", "折り返されるはず。"),
		"a part is lost": mk("◆ とても長い理由です。", "  折り返されるは"),
		"no mark":        mk("  とても長い理由です。", "  折り返されるはず。"),
	} {
		if _, p := verifyLongWhy(g, why); len(p) == 0 {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestLongWhyTapeHasTheSameTimesEveryRun(t *testing.T) {
	bin, err := filepath.Abs("../../bin/srwr")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skip("bin/srwr is not built (qsoku bin)")
	}
	var made [2]string
	for i := range made {
		extra := t.TempDir()
		if err := makeLongWhyTape(bin, extra); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(extra, ".srwr", "tapes", longWhyTape+".tape.jsonl")) //nolint:gosec // a path in a temporary directory
		if err != nil {
			t.Fatal(err)
		}
		made[i] = string(b)
		time.Sleep(1100 * time.Millisecond)
	}
	// The events differ by the selection tokens (they hold the tape's id and the time they were made); the times are the same.
	times := func(s string) []string { return regexp.MustCompile(`"(?:startedAt|ts)":"[^"]*"`).FindAllString(s, -1) }
	if a, b := times(made[0]), times(made[1]); len(a) < 4 || strings.Join(a, ",") != strings.Join(b, ",") {
		t.Errorf("times differ between runs:\n%v\n%v", a, b)
	}
	if !strings.Contains(made[0], `"startedAt":"2026-10-03T00:00:00.000+09:00"`) {
		t.Errorf("the header time is not the fixed one: %s", strings.SplitN(made[0], "\n", 2)[0])
	}
}

func TestVSCodeIsOpenedWithItsOwnProfileInEnglish(t *testing.T) {
	install, open := vscodeArgs("/b", "/b/x.vsix", "/b/ws")
	for name, args := range map[string][]string{"install": install, "open": open} {
		if !slices.Contains(args, "--user-data-dir") || !slices.Contains(args, "--extensions-dir") {
			t.Errorf("%s: %v has no profile of its own (a VSCode in Japanese would show the extension in Japanese)", name, args)
		}
	}
	if !slices.Contains(install, "--install-extension") || !slices.Contains(open, "--locale") || open[len(open)-1] != "/b/ws" {
		t.Errorf("install %v, open %v", install, open)
	}
	if i := slices.Index(open, "--extensions-dir"); i < 0 || open[i+1] != install[slices.Index(install, "--extensions-dir")+1] {
		t.Errorf("the extension is installed where VSCode does not look: %v %v", install, open)
	}
}
