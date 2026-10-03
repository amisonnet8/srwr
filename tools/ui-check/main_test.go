package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	for _, f := range tapes {
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
