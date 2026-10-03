package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/amisonnet8/srwr/internal/uicheck"
)

// vimScenario is one file of vim/test/baseline: the approved images of a set of frames, in order.
type vimScenario struct {
	name string // the name of the baseline file and of the scenario in vim/test/screen/capture.vim
	dir  string // under handoff/design/images/vim
}

var vimScenarios = []vimScenario{
	{"replay-why-basic", "replay/why-basic"},
	{"replay-external", "replay/external"},
	{"replay-no-why", "replay/no-why"},
	{"live-basic", "live/why-basic"},
	{"live-external", "live/external"},
}

// liveBehind are the stages of the live view of why-basic that look at an older frame (stage index → what the status
// line says). Their approved images show the cut-off status line; the UI gate of R5 decided how to draw it (LiveStatus).
var liveBehind = map[int]struct {
	index, total, behind int
	where                string
}{
	3: {2, 3, 1, "text.go:103"},
	4: {2, 5, 3, "text.go:103"},
	5: {2, 7, 5, "text.go:103"},
}

// normal is the background of the Normal group the images were drawn with (vim/test/screen/capture.vim sets the same).
var normal = map[string]string{"dark": "#1e1e1e", "light": "#ffffff"}

// makeVimBaseline reads the approved images under images (handoff/design/images/vim) and writes the baseline files into out.
func makeVimBaseline(images, out string) error {
	if err := os.MkdirAll(out, 0o750); err != nil { //nolint:gosec // the directory the caller names for the baseline
		return err
	}
	for _, theme := range []string{"dark", "light"} {
		for _, sc := range vimScenarios {
			files, err := filepath.Glob(filepath.Join(images, filepath.FromSlash(sc.dir), "*_"+theme+".svg"))
			if err != nil || len(files) == 0 {
				return fmt.Errorf("no images for %s %s (%v)", sc.name, theme, err)
			}
			sort.Strings(files)
			b := &uicheck.Baseline{}
			for i, f := range files {
				g, err := uicheck.ReadSVG(f, normal[theme])
				if err != nil {
					return fmt.Errorf("%s: %w", f, err)
				}
				if sc.name == "live-basic" {
					if lb, ok := liveBehind[i]; ok {
						if err := uicheck.LiveStatus(g, g.Rows-2, 41, lb.index, lb.total, lb.behind, lb.where); err != nil {
							return fmt.Errorf("%s: %w", f, err)
						}
					}
				}
				if sc.name == "replay-no-why" && i >= 2 && i <= 6 {
					// A range longer than the window: every row of the window is painted.
					uicheck.PaintWindowRange(g, 1, g.Rows-3, 45, map[string]string{"dark": "#1d3a5c", "light": "#cfe3fb"}[theme])
				}
				b.Cols, b.Rows, b.Normal = g.Cols, g.Rows, g.Normal
				label := strings.TrimSuffix(filepath.Base(f), "_"+theme+".svg")
				fr := uicheck.Reduce(g, label)
				if sc.name == "live-external" && i == 1 {
					// The right-hand status line of the diff frame is too narrow for its text; what the approved image
					// shows is the old line cut by the close hint, which the UI gate of R5 changed (decision 2).
					fr.Skip = []int{g.Rows - 2}
				}
				b.Frames = append(b.Frames, fr)
			}
			data, err := uicheck.MarshalBaseline(b)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(out, sc.name+"_"+theme+".json"), data, 0o600); err != nil { //nolint:gosec // see above
				return err
			}
		}
	}
	return nil
}
