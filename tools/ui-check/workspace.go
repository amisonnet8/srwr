package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// tapes are the fixed tapes (extension/test/fixtures/ui-check), by the short name a person types.
var tapes = map[string]struct {
	ID, Started, Look string
}{
	"why-basic": {"20260930-0054-why-basic", "2026-09-30 00:54:08", "7コマ。select は青の理由の行＋薄い青、replace（5〜7）は橙。dark と light の両方で読めるか"},
	"external":  {"20260930-0949-external", "2026-09-30 09:49:46", "11コマ。6コマ目の外部変更、10・11の録画後は左右の差分（前＝青、後＝橙、変わった行だけ）。見やすいか"},
	"no-why":    {"20260930-0053-no-why", "2026-09-30 00:53:32", "12コマ。理由の行が無く、範囲の色だけ。11・12は録画後の差分"},
}

// prepareWorkspace makes a new workspace in dst from the fixed one in src: the real files and the tapes. dst is removed first,
// so the result is the same every time. srwr.path is set to bin so that the extension finds the binary just built.
// With live, the tapes are left out; the live tape is fed by feedLive.
func prepareWorkspace(src, dst, bin string, live bool) error {
	if err := os.RemoveAll(dst); err != nil { //nolint:gosec // the workspace of this tool, under the temporary directory
		return err
	}
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if live && strings.HasSuffix(rel, ".tape.jsonl") {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750) //nolint:gosec // inside the workspace of this tool
		}
		b, err := os.ReadFile(p) //nolint:gosec // a path found by WalkDir under the fixtures
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o600) //nolint:gosec // inside the workspace of this tool
	})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dst, ".srwr", "tapes"), 0o750); err != nil { //nolint:gosec // inside that workspace
		return err
	}
	settings, err := json.MarshalIndent(map[string]string{"srwr.path": bin}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dst, ".vscode"), 0o750); err != nil { //nolint:gosec // inside that workspace
		return err
	}
	return os.WriteFile(filepath.Join(dst, ".vscode", "settings.json"), append(settings, '\n'), 0o600) //nolint:gosec // inside the workspace of this tool
}

// liveTapeName is the tape the live view follows.
const liveTapeName = "20260930-0000-live.tape.jsonl"

// feedLive writes the lines of the fixed tape why-basic into the workspace's live tape. The lines up to and including the
// first operation are written at once; then it waits for the person (wait), and writes one line every `every`.
func feedLive(workspace, fixture string, wait func(), every time.Duration, log io.Writer) error {
	f, err := os.Open(filepath.Join(fixture, ".srwr", "tapes", tapes["why-basic"].ID+".tape.jsonl")) //nolint:gosec // a fixed path under the fixtures
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return err
	}
	tape := filepath.Join(workspace, ".srwr", "tapes", liveTapeName)
	first := firstOperation(lines)
	if err := os.WriteFile(tape, []byte(strings.Join(lines[:first+1], "\n")+"\n"), 0o600); err != nil { //nolint:gosec // inside the workspace of this tool
		return err
	}
	wait()
	for i, l := range lines[first+1:] {
		time.Sleep(every)
		out, err := os.OpenFile(tape, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // the tape this tool made
		if err != nil {
			return err
		}
		_, werr := fmt.Fprintln(out, l)
		if cerr := out.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return werr
		}
		_, _ = fmt.Fprintf(log, "追記 %d/%d\n", i+1, len(lines)-first-1)
	}
	return nil
}

// firstOperation is the index of the first select, replace or external line.
func firstOperation(lines []string) int {
	for i, l := range lines {
		var e struct{ Type string }
		if json.Unmarshal([]byte(l), &e) == nil && (e.Type == "select" || e.Type == "replace" || e.Type == "external") {
			return i
		}
	}
	return len(lines) - 1
}
