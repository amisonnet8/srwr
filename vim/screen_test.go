package vim

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amisonnet8/srwr/internal/uicheck"
)

// The screen test runs a real Vim on a terminal (script), takes its screen frame by frame with the outer Vim's
// term_scrape() (vim/test/screen/capture.vim), and compares it with the baseline (vim/test/baseline), which was made from
// the approved images. It takes a while, so `go test ./...` skips it; `qsoku vim-test` sets SRWR_SCREEN_TEST=1.

var scenarios = []struct{ name, scenario string }{
	{"replay-why-basic", "replay:20260930-0054-why-basic"},
	{"replay-external", "replay:20260930-0949-external"},
	{"replay-no-why", "replay:20260930-0053-no-why"},
	{"live-basic", "live-basic"},
	{"live-external", "live-external"},
}

var (
	buildOnce sync.Once
	binPath   string
	buildErr  error
)

func srwrBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "srwr-screen-")
		if err != nil {
			buildErr = err
			return
		}
		binPath = filepath.Join(dir, "srwr")
		cmd := exec.Command("go", "build", "-o", binPath, "../cmd/srwr") //nolint:gosec // fixed arguments
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{string(out), err}
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return binPath
}

type buildError struct {
	out string
	err error
}

func (e *buildError) Error() string { return "go build: " + e.err.Error() + "\n" + e.out }

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	if out, err := exec.Command("cp", "-r", src+"/.", dst).CombinedOutput(); err != nil { //nolint:gosec // paths made by this test
		t.Fatalf("cp: %v\n%s", err, out)
	}
}

// capture runs the outer Vim and returns the screens it took.
func capture(t *testing.T, scenario, theme string) *uicheck.Capture {
	t.Helper()
	return captureSized(t, scenario, theme, 0, 0)
}

// captureSized is capture on a screen of rows by cols (0 for the default, 50 by 140).
func captureSized(t *testing.T, scenario, theme string, rows, cols int) *uicheck.Capture {
	t.Helper()
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	copyDir(t, filepath.Join(repo, "extension", "test", "fixtures", "ui-check"), ws)
	out := filepath.Join(t.TempDir(), "screens.json")
	vimBin := os.Getenv("VIM_BIN")
	if vimBin == "" {
		vimBin = "vim"
	}
	cmdline := vimBin + " -Nu NONE -i NONE -S " + filepath.Join(repo, "vim", "test", "screen", "capture.vim")
	cmd := exec.Command("script", "-qec", cmdline, "/dev/null") //nolint:gosec // fixed arguments and paths made by this test
	cmd.Env = append(os.Environ(), "SRWR_REPO="+repo, "SRWR_BIN="+srwrBinary(t), "WS="+ws, "SCENARIO="+scenario, "THEME="+theme, "OUT="+out, "TERM=xterm-256color", "SRWR_VIM_BIN="+vimBin,
		// One language for every machine: UTF-8 (the text is Japanese), and Vim's own messages in English.
		"LC_ALL=C.UTF-8", "LANG=C.UTF-8")
	if rows > 0 {
		cmd.Env = append(cmd.Env, "ROWS="+strconv.Itoa(rows), "COLS="+strconv.Itoa(cols))
	}
	cmd.Stdin = nil
	done := make(chan error, 1)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Minute):
		_ = cmd.Process.Kill()
		t.Fatalf("the outer Vim did not finish; it printed:\n%s", output.String())
	}
	data, err := os.ReadFile(out) //nolint:gosec // a path in a temporary directory
	if err != nil {
		msg, _ := os.ReadFile(out + ".error") //nolint:gosec // see above
		t.Fatalf("no screens were taken: %v\n%s\n%s", err, msg, output.String())
	}
	normal := "#1e1e1e"
	if theme == "light" {
		normal = "#ffffff"
	}
	c, err := uicheck.ParseCapture(data, normal)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestScreens(t *testing.T) {
	if os.Getenv("SRWR_SCREEN_TEST") == "" {
		t.Skip("set SRWR_SCREEN_TEST=1 (qsoku vim-test does)")
	}
	if runtime.GOOS != "linux" {
		t.Skip("the screens are taken with script(1) as it is on Linux")
	}
	for _, tool := range []string{"vim", "script", "go", "cp"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("no %s", tool)
		}
	}
	for _, theme := range []string{"dark", "light"} {
		for _, sc := range scenarios {
			t.Run(sc.name+"/"+theme, func(t *testing.T) {
				t.Parallel()
				data, err := os.ReadFile(filepath.Join("test", "baseline", sc.name+"_"+theme+".json")) //nolint:gosec // a fixed path under vim/test/baseline
				if err != nil {
					t.Fatal(err)
				}
				want, err := uicheck.ReadBaseline(data)
				if err != nil {
					t.Fatal(err)
				}
				got := capture(t, sc.scenario, theme)
				if len(got.Grids) != len(want.Frames) {
					t.Fatalf("%d screens were taken, the baseline has %d", len(got.Grids), len(want.Frames))
				}
				for i, g := range got.Grids {
					frame := uicheck.Reduce(g, got.Labels[i])
					// The tab line says [無名] or [No Name] by the language of the Vim, which is not srwr's: it is not compared.
					wantFrame := want.Frames[i]
					wantFrame.Skip = append(slices.Clone(wantFrame.Skip), 0)
					if diffs := uicheck.Compare(wantFrame, frame, want.Normal, 6); len(diffs) > 0 {
						t.Errorf("frame %d (%s, baseline %s) differs:\n  %s", i+1, got.Labels[i], want.Frames[i].Label, strings.Join(diffs, "\n  "))
					}
				}
			})
		}
	}
}

// The range near the bottom of the window (UI gate of R5, decision 1): on a screen of 40 rows the why row and the first
// line of the range are both on the screen in every frame, the range line right under the why row.
func TestRangeIsNeverHiddenUnderTheWhyRows(t *testing.T) {
	if os.Getenv("SRWR_SCREEN_TEST") == "" || runtime.GOOS != "linux" {
		t.Skip("set SRWR_SCREEN_TEST=1 (qsoku vim-test does); Linux only")
	}
	for _, tool := range []string{"vim", "script", "go", "cp"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("no %s", tool)
		}
	}
	for _, rows := range []int{40, 30, 25} {
		t.Run(strconv.Itoa(rows)+" rows", func(t *testing.T) {
			t.Parallel()
			got := captureSized(t, "replay:20260930-0054-why-basic", "dark", rows, 140)
			if len(got.Grids) != 7 {
				t.Fatalf("%d screens", len(got.Grids))
			}
			for i, g := range got.Grids {
				why := -1
				for r := 1; r < g.Rows-2; r++ {
					if bg := g.Cells[r][46].BG; strings.Contains(g.RowText(r), "◆ ") && (bg == "#0b61a4" || bg == "#b45f06") {
						why = r
						break
					}
				}
				if why < 0 {
					t.Errorf("frame %d: no why row on the screen of %d rows", i+1, rows)
					continue
				}
				if i == 0 {
					// text.go:37 does not fit from the top of a screen this small: the why row is put in the middle
					// of the window (what zz does), not left at the bottom edge where the cursor would put it.
					if want := 1 + (rows-3-1)/2; why != want {
						t.Errorf("frame 1: the why row is on row %d of the screen of %d rows, want the middle of the window, row %d", why, rows, want)
					}
				}
				if bg := g.Cells[why+1][46].BG; bg != "#1d3a5c" && bg != "#583c27" {
					t.Errorf("frame %d: the row under the why row is not the range (background %s) on %d rows:\n%s", i+1, bg, rows, g.RowText(why+1))
				}
			}
		})
	}
}
