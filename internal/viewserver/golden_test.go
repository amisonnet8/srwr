package viewserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/timeline"
)

// The golden data (extension/test/golden) are the frames the earlier implementation made from
// each tape. Every one is checked here through the real server: the frames, the files, and
// the content of each file at each frame.

const (
	goldenDir   = "../../extension/test/golden"
	fixturesDir = "../../extension/test/fixtures"
)

type goldenFrame struct {
	Index     int             `json:"index"`
	Kind      string          `json:"kind"`
	Seq       int             `json:"seq"`
	TS        int64           `json:"ts"`
	File      string          `json:"file"`
	Range     timeline.Range  `json:"range"`
	OldRange  *timeline.Range `json:"oldRange"`
	Why       *string         `json:"why"`
	Selection *string         `json:"selection"`
	From      *string         `json:"from"`
	Parent    *int            `json:"parent"`
	Deleted   bool            `json:"deleted"`
	Before    *string         `json:"before"`
	After     *string         `json:"after"`
}

type golden struct {
	Name     string `json:"name"`
	Tape     string `json:"tape"`
	TapeText string `json:"tapeText"`
	Current  struct {
		Kind  string            `json:"kind"`
		Files map[string]string `json:"files"`
		Dir   string            `json:"dir"`
	} `json:"current"`
	Frames    []goldenFrame        `json:"frames"`
	Files     []string             `json:"files"`
	ContentAt map[string][]*string `json:"contentAt"`
}

func TestGolden(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(goldenDir, "*.json"))
	if err != nil || len(paths) != 22 {
		t.Fatalf("found %d golden files (%v), want 22", len(paths), err)
	}
	for _, path := range paths {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".json"), func(t *testing.T) {
			b, err := os.ReadFile(path) //nolint:gosec // a path found by Glob
			if err != nil {
				t.Fatal(err)
			}
			var g golden
			if err := json.Unmarshal(b, &g); err != nil {
				t.Fatal(err)
			}
			checkGolden(t, g)
		})
	}
}

// workspaceFor makes the workspace a golden case was made in, and returns its root, the ID of the tape, and whether the last diff is wanted.
func workspaceFor(t *testing.T, g golden) (root, tapeID string, diffFrames bool) {
	t.Helper()
	diffFrames = g.Current.Kind != "none"
	if g.Current.Kind == "dir" {
		// The workspace is the fixtures' own, which is only read.
		root, err := filepath.Abs(filepath.Join(fixturesDir, g.Current.Dir))
		if err != nil {
			t.Fatal(err)
		}
		return root, strings.TrimSuffix(filepath.Base(g.Tape), tape.FileSuffix), true
	}

	root = t.TempDir()
	tapeID = g.Name
	var data []byte
	if g.TapeText != "" {
		data = []byte(g.TapeText)
	} else {
		var err error
		if data, err = os.ReadFile(filepath.Join(fixturesDir, g.Tape)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, ".srwr", "tapes"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".srwr", "tapes", tape.FileName(tapeID)), data, 0o600); err != nil { //nolint:gosec // a path in a temporary directory
		t.Fatal(err)
	}
	for name, text := range g.Current.Files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, tapeID, diffFrames
}

func checkGolden(t *testing.T, g golden) {
	root, id, diffFrames := workspaceFor(t, g)
	srv := &Server{Root: root, Version: "test"}

	// Everything in one connection: open, then the state of each file at each frame.
	reqs := []string{
		initReq(1, map[string]any{"diffFrames": diffFrames}),
		req(2, "tapes/list", map[string]any{}),
		req(3, "tape/open", map[string]any{"tapeId": id, "withText": true}),
	}
	type stateKey struct {
		file string
		i    int
	}
	var keys []stateKey
	files := make([]string, 0, len(g.ContentAt))
	for f := range g.ContentAt {
		files = append(files, f)
	}
	slices.Sort(files)
	for _, f := range files {
		for i := -1; i < len(g.Frames); i++ {
			keys = append(keys, stateKey{f, i})
			reqs = append(reqs, req(10+len(keys), "frame/state", map[string]any{"tapeId": id, "index": i, "file": f}))
		}
	}
	lines := exchange(t, srv, reqs...)
	if len(lines) != len(reqs) {
		t.Fatalf("%d responses for %d requests", len(lines), len(reqs))
	}

	var list struct {
		Tapes []struct {
			TapeID string
			Files  []string
		}
	}
	resultOf(t, lines[1], &list)
	found := false
	for _, ti := range list.Tapes {
		if ti.TapeID == id {
			found = true
			if !slices.Equal(ti.Files, g.Files) {
				t.Errorf("files = %v, want %v", ti.Files, g.Files)
			}
		}
	}
	if !found {
		t.Errorf("the tape %s is not in tapes/list: %+v", id, list.Tapes)
	}

	var opened struct {
		Frames []goldenFrame
		TapeID string
	}
	resultOf(t, lines[2], &opened)
	if opened.TapeID != id {
		t.Errorf("tapeId = %q, want %q", opened.TapeID, id)
	}
	if len(opened.Frames) != len(g.Frames) {
		t.Fatalf("%d frames, want %d", len(opened.Frames), len(g.Frames))
	}
	for i, got := range opened.Frames {
		want := g.Frames[i]
		if !sameFrame(got, want) {
			t.Errorf("frame %d:\n got %s\nwant %s", i, short(got), short(want))
		}
	}

	for n, k := range keys {
		var st struct {
			Content *string `json:"content"`
		}
		resultOf(t, lines[3+n], &st)
		want := g.ContentAt[k.file][k.i+1]
		if (st.Content == nil) != (want == nil) || (want != nil && *st.Content != *want) {
			t.Errorf("content of %s at frame %d = %s, want %s", k.file, k.i, quote(st.Content), quote(want))
		}
	}
}

// goldenKind is the kind the golden data (written for version 1, which said select and replace) has now: look and edit.
func goldenKind(kind string) string {
	switch kind {
	case "select":
		return "look"
	case "replace":
		return "edit"
	}
	return kind
}

func sameFrame(a, b goldenFrame) bool {
	// Not compared: jumpLabel, which the protocol no longer has.
	return a.Index == b.Index && a.Kind == goldenKind(b.Kind) && a.Seq == b.Seq && a.TS == b.TS && a.File == b.File && a.Range == b.Range &&
		eqPtr(a.OldRange, b.OldRange) && eqPtr(a.Why, b.Why) && eqPtr(a.Selection, b.Selection) && eqPtr(a.From, b.From) &&
		eqPtr(a.Parent, b.Parent) && a.Deleted == b.Deleted && eqPtr(a.Before, b.Before) && eqPtr(a.After, b.After)
}

func eqPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func short(f goldenFrame) string {
	trim := func(p *string) string {
		if p == nil {
			return "null"
		}
		return quote(p)
	}
	b, _ := json.Marshal(struct {
		goldenFrame
		Before, After string
	}{goldenFrame: f, Before: trim(f.Before), After: trim(f.After)})
	return string(b)
}

func quote(p *string) string {
	if p == nil {
		return "null"
	}
	b, _ := json.Marshal(*p)
	if len(b) > 80 {
		return string(b[:80]) + "…"
	}
	return string(b)
}
