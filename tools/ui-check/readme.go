package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/amisonnet8/srwr/internal/uicheck"
)

// The pictures of the README (`qsoku readme-media`). They are made from a small session run by the real `srwr mcp`, in
// English, and a real Vim is taken frame by frame (the same parts as qsoku ui-check), so a picture is never hand-drawn from
// memory: the README shows what srwr really does.

const demoSource = `package main

import "fmt"

func main() {
	fmt.Println(greet("world"))
}

func greet(name string) string {
	return "hello " + name
}
`

// demoAfter is the file after the recording: a person added a line, which the last frame (final) shows.
const demoAfter = demoSource + "\n// TODO: read the name from the command line\n"

// demoStep is one call of the AI: a select (newText is empty) or a replace of the range the last select declared.
type demoStep struct {
	kind       string // select | replace
	start, end int
	newText    string
	why        string
}

var demoSteps = []demoStep{
	{kind: "select", start: 9, end: 11, why: "greet is what main prints: check what it does when the name is empty"},
	{kind: "replace", newText: "func greet(name string) string {\n\tif name == \"\" {\n\t\treturn \"hello, stranger\"\n\t}\n\treturn \"hello, \" + name\n}",
		why: "An empty name printed \"hello \" with nothing after it, so greet a stranger instead"},
	{kind: "select", start: 5, end: 7, why: "main tries only one name: look at the call before adding the empty case"},
	{kind: "replace", newText: "func main() {\n\tfmt.Println(greet(\"world\"))\n\tfmt.Println(greet(\"\"))\n}",
		why: "Print both greetings, so the empty-name case shows up when the program runs"},
}

// demoHolds is how many seconds each of the five frames (four steps and the final diff) stays on the screen.
var demoHolds = []float64{2.8, 3.2, 2.8, 3.2, 3.6}

const demoTape = "20261003-0000-readme"

// makeDemoWorkspace runs demoSteps with the real `srwr mcp` in a new directory and returns it. The file is left as a
// person changed it after the recording.
func makeDemoWorkspace(bin string) (string, error) {
	work, err := os.MkdirTemp("", "srwr-readme-")
	if err != nil {
		return "", err
	}
	fail := func(err error) (string, error) {
		_ = os.RemoveAll(work)
		return "", err
	}
	if err := os.WriteFile(filepath.Join(work, "main.go"), []byte(demoSource), 0o600); err != nil {
		return fail(err)
	}
	c, err := startMCP(bin, work)
	if err != nil {
		return fail(err)
	}
	if _, err := c.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "readme", "version": "0"}}); err != nil {
		_ = c.close()
		return fail(err)
	}
	if err := c.notify("notifications/initialized"); err != nil {
		_ = c.close()
		return fail(err)
	}
	token := ""
	for _, s := range demoSteps {
		args := map[string]any{"why": s.why}
		name := s.kind
		if s.kind == "select" {
			args["file"], args["startLine"], args["endLine"] = "main.go", s.start, s.end
		} else {
			args["selection"], args["newText"] = token, s.newText
		}
		res, err := c.tool(name, args)
		if err != nil {
			_ = c.close()
			return fail(err)
		}
		token, _ = res["selection"].(string)
	}
	if err := c.close(); err != nil {
		return fail(err)
	}
	tapes, err := filepath.Glob(filepath.Join(work, ".srwr", "tapes", "*.tape.jsonl"))
	if err != nil || len(tapes) != 1 {
		return fail(fmt.Errorf("srwr mcp made %d tapes, want 1 (%v)", len(tapes), err))
	}
	if err := os.Rename(tapes[0], filepath.Join(work, ".srwr", "tapes", demoTape+".tape.jsonl")); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(work, "main.go"), []byte(demoAfter), 0o600); err != nil {
		return fail(err)
	}
	return work, nil
}

// vimDemo takes the screens of a real Vim stepping through the demo tape, in one theme, and returns the animated image.
func vimDemo(root, bin, work, theme string) (string, error) {
	got, err := uicheck.CaptureVim(uicheck.VimOptions{Repo: root, Bin: bin, Workspace: work, Scenario: "replay:" + demoTape,
		Theme: theme, Lang: "en", VimBin: os.Getenv("VIM_BIN"), Rows: 27, Cols: 124})
	if err != nil {
		return "", err
	}
	if len(got.Grids) != len(demoHolds) {
		return "", fmt.Errorf("%d screens were taken, want %d (four steps and the final diff)", len(got.Grids), len(demoHolds))
	}
	grids := make([]*uicheck.Grid, len(got.Grids))
	for i, g := range got.Grids {
		grids[i] = g.WithoutTopRows(1) // the tab line of the Vim started for the picture
	}
	return uicheck.AnimatedSVG("srwr view in Vim: the AI's select and replace, with the reason of each, step by step", grids, demoHolds)
}

// runReadme makes the pictures under docs/images/ and extension/media/readme/ and the page to look at them.
func runReadme(root string, out io.Writer) error {
	bin := filepath.Join(root, "bin", "srwr")
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("%s is missing; run qsoku bin first", bin)
	}
	work, err := makeDemoWorkspace(bin)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(work) }()
	images := filepath.Join(root, "docs", "images")
	if err := os.MkdirAll(images, 0o750); err != nil {
		return err
	}
	// The README shows the dark pictures only (a decision of the person who reads them first).
	for _, theme := range []string{"dark"} {
		svg, err := vimDemo(root, bin, work, theme)
		if err != nil {
			return fmt.Errorf("the Vim demo (%s): %w", theme, err)
		}
		name := filepath.Join(images, "demo-vim_"+theme+".svg")
		if err := os.WriteFile(name, []byte(svg), 0o644); err != nil { //nolint:gosec // a picture of the repository, read by everyone
			return err
		}
		_, _ = fmt.Fprintln(out, "wrote", name, strconv.Itoa(len(svg)), "bytes")
		code := vscodeFrame(theme, 1)
		name = filepath.Join(images, "vscode_"+theme+".svg")
		if err := os.WriteFile(name, []byte(code), 0o644); err != nil { //nolint:gosec // see above
			return err
		}
		_, _ = fmt.Fprintln(out, "wrote", name, strconv.Itoa(len(code)), "bytes")
	}
	media := filepath.Join(root, "extension", "media", "readme")
	jobs := []pngJob{
		{filepath.Join(images, "vscode_dark.svg"), filepath.Join(media, "replay.png"), "2"},
		{filepath.Join(images, "banner.svg"), filepath.Join(media, "banner.png"), "1"},
		// The icon of the extension in the Marketplace (a PNG of 128 pixels or more is asked for), made at twice the size.
		{filepath.Join(root, "extension", "media", "icon.svg"), filepath.Join(root, "extension", "media", "icon.png"), "2"},
	}
	if err := renderPNGs(jobs, out); err != nil {
		return err
	}
	return writeReadmePage(root, out)
}
