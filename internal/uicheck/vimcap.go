package uicheck

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

// VimOptions says what CaptureVim takes the screens of.
type VimOptions struct {
	Repo      string // the repository (where vim/ is)
	Bin       string // the srwr binary
	Workspace string // a workspace made for this capture (its tapes are the ones the scenario opens)
	Scenario  string // replay:<tape id> | live-basic | live-external (vim/test/screen/capture.vim)
	Theme     string // dark | light
	VimBin    string // the Vim to run; "vim" when empty
	Rows      int    // the size of the screen; 50 by 140 when 0
	Cols      int
}

// CaptureVim runs a real Vim on a terminal (script(1)) and takes its screen frame by frame: an outer Vim runs the inner
// Vim with srwr-view.vim in a terminal window and reads it with term_scrape() (vim/test/screen/capture.vim).
func CaptureVim(o VimOptions) (*Capture, error) {
	vimBin := o.VimBin
	if vimBin == "" {
		vimBin = "vim"
	}
	outDir, err := os.MkdirTemp("", "srwr-screens-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(outDir) }()
	out := filepath.Join(outDir, "screens.json")
	cmdline := vimBin + " -Nu NONE -i NONE -S " + filepath.Join(o.Repo, "vim", "test", "screen", "capture.vim")
	cmd := exec.Command("script", "-qec", cmdline, "/dev/null") //nolint:gosec // fixed arguments and paths made by the caller
	cmd.Env = append(os.Environ(), "SRWR_REPO="+o.Repo, "SRWR_BIN="+o.Bin, "WS="+o.Workspace, "SCENARIO="+o.Scenario, "THEME="+o.Theme, "OUT="+out,
		"TERM=xterm-256color", "SRWR_VIM_BIN="+vimBin,
		// One language for every machine: UTF-8 (the text is Japanese), and Vim's own messages in English.
		"LC_ALL=C.UTF-8", "LANG=C.UTF-8")
	if o.Rows > 0 {
		cmd.Env = append(cmd.Env, "ROWS="+strconv.Itoa(o.Rows), "COLS="+strconv.Itoa(o.Cols))
	}
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Minute):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("the outer Vim did not finish; it printed:\n%s", output.String())
	}
	data, err := os.ReadFile(out) //nolint:gosec // a path in a temporary directory
	if err != nil {
		msg, _ := os.ReadFile(out + ".error") //nolint:gosec // see above
		return nil, fmt.Errorf("no screens were taken: %w\n%s\n%s", err, msg, output.String())
	}
	normal := "#1e1e1e"
	if o.Theme == "light" {
		normal = "#ffffff"
	}
	return ParseCapture(data, normal)
}

// FrameDiff is how one screen differs from the baseline.
type FrameDiff struct {
	Index int    // 1-based; 0 when the whole capture does not match (the number of screens)
	Label string // what was taken
	Want  string // the label in the baseline
	Diffs []string
}

// CompareBaseline compares the screens of a capture with a baseline. Nothing returned means they are the same.
// The tab line (row 1) is not compared: it says [無名] or [No Name] by the language of the Vim, which is not srwr's.
func CompareBaseline(want *Baseline, got *Capture) []FrameDiff {
	if len(got.Grids) != len(want.Frames) {
		return []FrameDiff{{Diffs: []string{fmt.Sprintf("%d screens were taken, the baseline has %d", len(got.Grids), len(want.Frames))}}}
	}
	var out []FrameDiff
	for i, g := range got.Grids {
		frame := Reduce(g, got.Labels[i])
		wantFrame := want.Frames[i]
		wantFrame.Skip = append(slices.Clone(wantFrame.Skip), 0)
		if diffs := Compare(wantFrame, frame, want.Normal, 6); len(diffs) > 0 {
			out = append(out, FrameDiff{Index: i + 1, Label: got.Labels[i], Want: want.Frames[i].Label, Diffs: diffs})
		}
	}
	return out
}

// ErrNoBaseline says a baseline does not exist yet (a new scenario): the screens are shown as new ones.
var ErrNoBaseline = errors.New("no baseline")
