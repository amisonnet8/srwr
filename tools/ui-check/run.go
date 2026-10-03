package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/amisonnet8/srwr/internal/uicheck"
)

// screenRun is one run of the Vim client whose screens are compared with vim/test/baseline/<name>_<theme>.json.
type screenRun struct {
	Name, Scenario string
	Themes         []string
	Rows, Cols     int // 0: 50 by 140
	Live           bool
}

var screenRuns = []screenRun{
	{Name: "replay-why-basic", Scenario: "replay:20260930-0054-why-basic", Themes: []string{"dark", "light"}},
	{Name: "replay-external", Scenario: "replay:20260930-0949-external", Themes: []string{"dark", "light"}},
	{Name: "replay-no-why", Scenario: "replay:20260930-0053-no-why", Themes: []string{"dark", "light"}},
	{Name: "replay-long-why", Scenario: "replay:" + longWhyTape, Themes: []string{"dark", "light"}},
	{Name: "replay-long-why-narrow", Scenario: "replay:" + longWhyTape, Themes: []string{"dark"}, Rows: 50, Cols: 80},
	{Name: "live-basic", Scenario: "live-basic", Themes: []string{"dark", "light"}, Live: true},
	{Name: "live-external", Scenario: "live-external", Themes: []string{"dark", "light"}, Live: true},
}

// vscodeFiles are what extension/test/capture.ts writes, with whether the file is about the live view.
var vscodeFiles = []struct {
	Name string
	Live bool
}{{"all_basic", false}, {"all_ext", false}, {"all_nowhy", false}, {"long_why", false}, {"live_basic", true}, {"live_ext", true}}

// Status of a capture compared with its baseline.
const (
	statusSame  = "same"  // the same as the baseline
	statusDiff  = "diff"  // differs
	statusNew   = "new"   // there is no baseline yet
	statusError = "error" // the capture failed
)

// Report is the result of a run (result.json).
type Report struct {
	Dir      string          `json:"dir"` // the name of the result directory
	Version  string          `json:"version"`
	Time     string          `json:"time"`
	LiveOnly bool            `json:"liveOnly,omitempty"`
	Checks   []CheckResult   `json:"checks"`
	Vim      []CaptureResult `json:"vim"`
	VSCode   []CaptureResult `json:"vscode"`
	Problems []string        `json:"problems,omitempty"` // things the run itself found (the long why on the screen, ...)
	Decision Decision        `json:"decision"`
}

// CaptureResult is one capture compared with its baseline.
type CaptureResult struct {
	Name   string      `json:"name"` // e.g. "replay-why-basic dark"
	Status string      `json:"status"`
	Error  string      `json:"error,omitempty"`
	File   string      `json:"file,omitempty"`   // the capture, in the shape of the baseline file, relative to the result directory
	Target string      `json:"target,omitempty"` // the baseline file it replaces, relative to the repository
	Frames []FrameInfo `json:"frames,omitempty"` // the frames that differ (or all of them, when new)
}

// FrameInfo is a frame that differs, with the pictures of it.
type FrameInfo struct {
	Index int      `json:"index"`
	Label string   `json:"label"`
	Diffs []string `json:"diffs,omitempty"`
	Want  string   `json:"-"` // pictures (SVG), only in the page
	Got   string   `json:"-"`
}

// Decision is what the person decided after looking at the page (qsoku ui-accept).
type Decision struct {
	State string `json:"state"` // pending | ok | ng
	At    string `json:"at,omitempty"`
	Note  string `json:"note,omitempty"`
}

// Clean reports whether nothing differs and every check passed. New captures are not a difference: nobody has said yet what
// they should look like.
func (r *Report) Clean() bool {
	for _, c := range r.Checks {
		if !c.OK {
			return false
		}
	}
	if len(r.Problems) > 0 {
		return false
	}
	for _, c := range append(slices.Clone(r.Vim), r.VSCode...) {
		if c.Status == statusDiff || c.Status == statusError {
			return false
		}
	}
	return true
}

// NewCount is the number of captures without a baseline.
func (r *Report) NewCount() int {
	n := 0
	for _, c := range append(slices.Clone(r.Vim), r.VSCode...) {
		if c.Status == statusNew {
			n++
		}
	}
	return n
}

// DiffCount is the number of frames that differ.
func (r *Report) DiffCount() int {
	n := 0
	for _, c := range append(slices.Clone(r.Vim), r.VSCode...) {
		if c.Status == statusDiff {
			n += len(c.Frames)
			if len(c.Frames) == 0 {
				n++
			}
		}
	}
	return n
}

// runCheck is `ui-check run` (and `ui-check live`): it does every step without stopping at a failure, writes the result
// directory, and returns the report. root is the repository.
func runCheck(root string, liveOnly bool, out io.Writer) (*Report, string, error) {
	log := func(s string) { _, _ = fmt.Fprintln(out, "▶ "+s) }
	stamp := time.Now().Format("20060102-150405")
	base := filepath.Join(root, "ui-check-result")
	dir := filepath.Join(base, stamp)
	if err := os.MkdirAll(filepath.Join(dir, "vim"), 0o750); err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(filepath.Join(dir, "vscode"), 0o750); err != nil {
		return nil, "", err
	}
	rep := &Report{Dir: stamp, Version: describe(root), Time: time.Now().Format("2006-01-02 15:04"), LiveOnly: liveOnly, Decision: Decision{State: "pending"}}
	bin := filepath.Join(root, "bin", "srwr")

	if liveOnly {
		log("自動の検証は、ライブだけを取るときは動かしません（qsoku ui-check で動かします）")
	} else {
		rep.Checks = runChecks(root, log)
	}

	extra, err := os.MkdirTemp("", "srwr-ui-extra-")
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = os.RemoveAll(extra) }()
	haveLong := false
	if !liveOnly {
		log("その場で作るテープ（長い理由）")
		if err := makeLongWhyTape(bin, extra); err != nil {
			rep.Problems = append(rep.Problems, "長い理由のテープを作れなかった："+err.Error())
		} else {
			haveLong = true
		}
	}

	log("Vim の画面を取る")
	rep.Vim = captureVimAll(root, bin, dir, extra, haveLong, liveOnly, rep)
	log("VSCode（拡張）の画面を取る")
	rep.VSCode = captureVSCode(root, dir, extra, haveLong, liveOnly)

	if err := writeReport(dir, rep); err != nil {
		return rep, dir, err
	}
	latest := filepath.Join(base, "latest")
	if err := os.RemoveAll(latest); err != nil {
		return rep, dir, err
	}
	if err := copyTree(dir, latest); err != nil {
		return rep, dir, err
	}
	return rep, dir, nil
}

func writeReport(dir string, rep *Report) error {
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "result.json"), append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.html"), []byte(renderPage(rep)), 0o600)
}

func describe(root string) string {
	cmd := exec.Command("git", "describe", "--always", "--dirty", "--tags")
	cmd.Dir = root
	b, err := cmd.Output()
	if err != nil {
		return "(unknown)"
	}
	return strings.TrimSpace(string(b))
}

// copyTree copies a directory (the result goes to latest/ too, so that the page is always at one place).
func copyTree(src, dst string) error {
	if out, err := exec.Command("cp", "-r", src, dst).CombinedOutput(); err != nil { //nolint:gosec // our own directories
		return fmt.Errorf("cp: %w\n%s", err, out)
	}
	return nil
}

// workspaceFor makes a workspace for one Vim run: the fixed workspace, with the tape made on the spot over it.
func workspaceFor(root, extra string, withExtra bool) (string, error) {
	ws, err := os.MkdirTemp("", "srwr-ui-ws-")
	if err != nil {
		return "", err
	}
	if err := copyTree(filepath.Join(root, "extension", "test", "fixtures", "ui-check")+"/.", ws); err != nil {
		return "", err
	}
	if withExtra {
		if err := copyTree(extra+"/.", ws); err != nil {
			return "", err
		}
	}
	return ws, nil
}

func captureVimAll(root, bin, dir, extra string, haveLong, liveOnly bool, rep *Report) []CaptureResult {
	type job struct {
		sc    screenRun
		theme string
	}
	var jobs []job
	for _, sc := range screenRuns {
		if liveOnly && !sc.Live {
			continue
		}
		if strings.Contains(sc.Name, "long-why") && !haveLong {
			continue
		}
		for _, th := range sc.Themes {
			jobs = append(jobs, job{sc, th})
		}
	}
	results := make([]CaptureResult, len(jobs))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = captureVimOne(root, bin, dir, extra, j.sc, j.theme, rep)
		}()
	}
	wg.Wait()
	return results
}

func captureVimOne(root, bin, dir, extra string, sc screenRun, theme string, rep *Report) CaptureResult {
	res := CaptureResult{Name: sc.Name + " " + theme}
	ws, err := workspaceFor(root, extra, strings.Contains(sc.Scenario, longWhyTape))
	if err != nil {
		return CaptureResult{Name: res.Name, Status: statusError, Error: err.Error()}
	}
	defer func() { _ = os.RemoveAll(ws) }()
	got, err := uicheck.CaptureVim(uicheck.VimOptions{Repo: root, Bin: bin, Workspace: ws, Scenario: sc.Scenario, Theme: theme,
		VimBin: os.Getenv("VIM_BIN"), Rows: sc.Rows, Cols: sc.Cols})
	if err != nil {
		res.Status, res.Error = statusError, err.Error()
		return res
	}
	normal := "#1e1e1e"
	if theme == "light" {
		normal = "#ffffff"
	}
	fg := "#d4d4d4"
	if theme == "light" {
		fg = "#1f2328"
	}
	cols, rows := 140, 50
	if sc.Cols > 0 {
		cols, rows = sc.Cols, sc.Rows
	}
	now := &uicheck.Baseline{Cols: cols, Rows: rows, Normal: normal}
	for i, g := range got.Grids {
		now.Frames = append(now.Frames, uicheck.Reduce(g, got.Labels[i]))
	}
	file := filepath.Join("vim", sc.Name+"_"+theme+".json")
	target := filepath.Join("vim", "test", "baseline", sc.Name+"_"+theme+".json")
	res.File, res.Target = file, target
	data, err := uicheck.MarshalBaseline(now)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, file), data, 0o600)
	}
	if err != nil {
		res.Status, res.Error = statusError, err.Error()
		return res
	}
	if strings.Contains(sc.Name, "long-why") {
		for _, fr := range []struct {
			frame int
			why   string
		}{{0, longSelectWhy}, {1, longReplaceWhy}} {
			if fr.frame < len(got.Grids) {
				n, problems := verifyLongWhy(got.Grids[fr.frame], fr.why)
				for _, p := range problems {
					rep.addProblem(fmt.Sprintf("長い理由（%s、frame %d、%d行）：%s", res.Name, fr.frame+1, n, p))
				}
			}
		}
	}
	old, err := os.ReadFile(filepath.Join(root, target)) //nolint:gosec // a path under the repository
	if os.IsNotExist(err) {
		res.Status = statusNew
		for i, g := range got.Grids {
			res.Frames = append(res.Frames, FrameInfo{Index: i + 1, Label: got.Labels[i], Got: g.SVG(res.Name+" "+got.Labels[i], nil)})
		}
		return res
	}
	if err != nil {
		res.Status, res.Error = statusError, err.Error()
		return res
	}
	want, err := uicheck.ReadBaseline(old)
	if err != nil {
		res.Status, res.Error = statusError, err.Error()
		return res
	}
	diffs := uicheck.CompareBaseline(want, got)
	if len(diffs) == 0 {
		res.Status = statusSame
		return res
	}
	res.Status = statusDiff
	for _, d := range diffs {
		fi := FrameInfo{Index: d.Index, Label: d.Label, Diffs: d.Diffs}
		if d.Index >= 1 && d.Index <= len(want.Frames) {
			wg := uicheck.GridOf(want.Frames[d.Index-1], want.Cols, want.Rows, want.Normal, fg)
			gg := uicheck.GridOf(now.Frames[d.Index-1], cols, rows, normal, fg)
			var marks [][2]int
			if wg.Cols == gg.Cols && wg.Rows == gg.Rows {
				marks = uicheck.DiffCells(wg, gg, append(slices.Clone(want.Frames[d.Index-1].Skip), 0))
			}
			fi.Want = wg.SVG("baseline "+d.Label, nil)
			fi.Got = gg.SVG("now "+d.Label, marks)
		}
		res.Frames = append(res.Frames, fi)
	}
	return res
}

var problemMu sync.Mutex

func (r *Report) addProblem(s string) {
	problemMu.Lock()
	defer problemMu.Unlock()
	r.Problems = append(r.Problems, s)
}

func captureVSCode(root, dir, extra string, haveLong, liveOnly bool) []CaptureResult {
	outDir := filepath.Join(dir, "vscode")
	args := []string{"--require", "./out/test/setup.js", "out/test/capture.js", outDir}
	if haveLong {
		args = append(args, extra)
	}
	env := []string(nil)
	if liveOnly {
		env = []string{"SRWR_CAPTURE_ONLY=live"}
	}
	b, err := capture(filepath.Join(root, "extension"), env, "node", args...)
	var out []CaptureResult
	for _, f := range vscodeFiles {
		if liveOnly && !f.Live {
			continue
		}
		if f.Name == "long_why" && !haveLong {
			continue
		}
		res := CaptureResult{Name: f.Name, File: filepath.Join("vscode", f.Name+".json"), Target: filepath.Join("extension", "test", "baseline", f.Name+".json")}
		data, rerr := os.ReadFile(filepath.Join(dir, res.File)) //nolint:gosec // our own result directory
		if rerr != nil {
			res.Status, res.Error = statusError, fmt.Sprintf("no capture (%v): %s", err, lastLines(string(b), 6))
			out = append(out, res)
			continue
		}
		got, rerr := uicheck.ReadShots(data)
		if rerr != nil {
			res.Status, res.Error = statusError, rerr.Error()
			out = append(out, res)
			continue
		}
		old, rerr := os.ReadFile(filepath.Join(root, res.Target)) //nolint:gosec // a path under the repository
		if os.IsNotExist(rerr) {
			res.Status = statusNew
			for i, s := range got {
				res.Frames = append(res.Frames, FrameInfo{Index: i + 1, Label: shotLabel(s, i), Got: uicheck.ShotSVG(s, f.Name+" "+shotLabel(s, i))})
			}
			out = append(out, res)
			continue
		}
		want, rerr := uicheck.ReadShots(old)
		if rerr != nil {
			res.Status, res.Error = statusError, rerr.Error()
			out = append(out, res)
			continue
		}
		diffs := uicheck.CompareShots(want, got)
		if len(diffs) == 0 {
			res.Status = statusSame
			out = append(out, res)
			continue
		}
		res.Status = statusDiff
		for _, d := range diffs {
			fi := FrameInfo{Index: d.Index, Label: d.Label, Diffs: d.Diffs}
			if d.Index >= 1 && d.Index <= len(want) && d.Index <= len(got) {
				fi.Want = uicheck.ShotSVG(want[d.Index-1], "baseline "+d.Label)
				fi.Got = uicheck.ShotSVG(got[d.Index-1], "now "+d.Label)
			}
			res.Frames = append(res.Frames, fi)
		}
		out = append(out, res)
	}
	return out
}

func shotLabel(s uicheck.Shot, i int) string {
	if l, ok := s["label"].(string); ok && l != "" {
		return l
	}
	return fmt.Sprintf("frame %d", i+1)
}
