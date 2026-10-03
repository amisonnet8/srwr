package timeline

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

func parse(t *testing.T, lines ...string) []tape.Event {
	t.Helper()
	res := tape.Parse([]byte(strings.Join(lines, "\n") + "\n"))
	if res.Skipped != 0 {
		t.Fatalf("%d lines cannot be read", res.Skipped)
	}
	return res.Events
}

func ptr[T any](v T) *T { return &v }

func TestFramesOfATape(t *testing.T) {
	ev := parse(t,
		`{"v":1,"seq":1,"ts":"2026-09-29T03:00:01.000Z","type":"snapshot","file":"a.go","text":"1\n2\n3\n"}`,
		`{"v":1,"seq":2,"ts":"2026-09-29T03:00:02.000Z","type":"select","file":"a.go","startLine":2,"endLine":3,"why":"見る","selection":"sel_1"}`,
		`{"v":1,"seq":3,"ts":"2026-09-29T03:00:03.000Z","type":"replace","file":"a.go","from":"sel_1","startLine":2,"endLine":3,"oldText":"2\n3","newText":"X","newStartLine":2,"newEndLine":2,"selection":"sel_2","why":"変える"}`,
		`{"v":1,"seq":4,"ts":"2026-09-29T03:00:04.000Z","type":"replace","file":"a.go","from":"sel_2","startLine":2,"endLine":2,"oldText":"X","newText":"","newStartLine":2,"newEndLine":1,"selection":"sel_3","why":"消す"}`,
		`{"v":1,"seq":5,"ts":"2026-09-29T03:00:05.000Z","type":"replace","file":"a.go","from":"sel_unknown","startLine":1,"endLine":1,"oldText":"1","newText":"one","newStartLine":1,"newEndLine":1,"selection":null,"why":null}`,
	)
	b := Build(ev)
	f := b.Frames()
	if len(f) != 4 || b.Ops() != 4 || !slices.Equal(b.Files(), []string{"a.go"}) {
		t.Fatalf("%d frames, %d ops, files %v", len(f), b.Ops(), b.Files())
	}
	tests := []struct {
		i              int
		kind           string
		rng            Range
		old            *Range
		before, after  string
		parent         *int
		why            *string
		seq            int
		ts             int64
		hasFrom, isSel bool
	}{
		{0, "select", Range{2, 3}, nil, "1\n2\n3\n", "1\n2\n3\n", nil, ptr("見る"), 2, 1790650802000, false, true},
		{1, "replace", Range{2, 2}, &Range{2, 3}, "1\n2\n3\n", "1\nX\n", ptr(0), ptr("変える"), 3, 1790650803000, true, true},
		{2, "replace", Range{2, 1}, &Range{2, 2}, "1\nX\n", "1\n", ptr(1), ptr("消す"), 4, 1790650804000, true, true},
		{3, "replace", Range{1, 1}, &Range{1, 1}, "1\n", "one\n", nil, nil, 5, 1790650805000, true, false},
	}
	for _, tt := range tests {
		g := f[tt.i]
		if g.Index != tt.i || g.Kind != tt.kind || g.Range != tt.rng || g.Before != tt.before || g.After != tt.after ||
			g.Seq != tt.seq || g.TS != tt.ts || g.File != "a.go" || g.Deleted {
			t.Errorf("frame %d = %+v", tt.i, g)
		}
		if (g.OldRange == nil) != (tt.old == nil) || (tt.old != nil && *g.OldRange != *tt.old) {
			t.Errorf("frame %d: OldRange = %v, want %v", tt.i, g.OldRange, tt.old)
		}
		if (g.Parent == nil) != (tt.parent == nil) || (tt.parent != nil && *g.Parent != *tt.parent) {
			t.Errorf("frame %d: Parent = %v, want %v", tt.i, g.Parent, tt.parent)
		}
		if (g.Why == nil) != (tt.why == nil) || (tt.why != nil && *g.Why != *tt.why) {
			t.Errorf("frame %d: Why = %v, want %v", tt.i, g.Why, tt.why)
		}
		if g.From != nil != tt.hasFrom || g.Selection != nil != tt.isSel {
			t.Errorf("frame %d: From = %v, Selection = %v", tt.i, g.From, g.Selection)
		}
	}
}

func TestTimestampsFallBackToTheFrameBefore(t *testing.T) {
	ev := parse(t,
		`{"v":1,"seq":1,"ts":"yesterday","type":"select","file":"a.go","startLine":1,"endLine":1}`,
		`{"v":1,"seq":2,"ts":"2026-09-29T03:00:13.000Z","type":"select","file":"a.go","startLine":1,"endLine":1}`,
		`{"v":1,"seq":3,"ts":"later","type":"select","file":"a.go","startLine":1,"endLine":1}`,
		`{"v":1,"seq":4,"type":"select","file":"a.go","startLine":1,"endLine":1}`,
	)
	var got []int64
	for _, f := range Build(ev).Frames() {
		got = append(got, f.TS)
	}
	if want := []int64{0, 1790650813000, 1790650813000, 1790650813000}; !slices.Equal(got, want) {
		t.Errorf("ts = %v, want %v", got, want)
	}
}

func TestExternalFrames(t *testing.T) {
	tests := []struct {
		name          string
		lines         []string
		wantBefore    string
		wantAfter     string
		wantRange     Range
		wantDeleted   bool
		wantLastState string // what the next select is shown on
	}{
		{
			name: "with text",
			lines: []string{
				`{"v":1,"seq":1,"type":"snapshot","file":"a.go","text":"1\n"}`,
				`{"v":1,"seq":2,"type":"external","file":"a.go","text":"1\n2\n3\n"}`,
				`{"v":1,"seq":3,"type":"snapshot","file":"a.go","text":"1\n2\n3\n"}`,
				`{"v":1,"seq":4,"type":"select","file":"a.go","startLine":1,"endLine":1}`,
			},
			wantBefore: "1\n", wantAfter: "1\n2\n3\n", wantRange: Range{1, 3}, wantLastState: "1\n2\n3\n",
		},
		{
			name: "deleted",
			lines: []string{
				`{"v":1,"seq":1,"type":"snapshot","file":"a.go","text":"old\n"}`,
				`{"v":1,"seq":2,"type":"external","file":"a.go","deleted":true,"text":null}`,
				`{"v":1,"seq":3,"type":"select","file":"b.go","startLine":1,"endLine":1}`,
			},
			wantBefore: "old\n", wantAfter: "", wantRange: Range{1, 0}, wantDeleted: true, wantLastState: "",
		},
		{
			name: "old format: the snapshot right after is the new content",
			lines: []string{
				`{"v":1,"seq":1,"type":"snapshot","file":"a.go","text":"1\n"}`,
				`{"v":1,"seq":2,"type":"external","file":"a.go"}`,
				`{"v":1,"seq":3,"type":"snapshot","file":"a.go","text":"X\nY\n"}`,
				`{"v":1,"seq":4,"type":"snapshot","file":"a.go","text":"Z\n"}`,
				`{"v":1,"seq":5,"type":"select","file":"a.go","startLine":1,"endLine":1}`,
			},
			wantBefore: "1\n", wantAfter: "X\nY\n", wantRange: Range{1, 2}, wantLastState: "Z\n",
		},
		{
			name: "old format: a snapshot of another file does not count",
			lines: []string{
				`{"v":1,"seq":1,"type":"snapshot","file":"a.go","text":"1\n"}`,
				`{"v":1,"seq":2,"type":"external","file":"a.go"}`,
				`{"v":1,"seq":3,"type":"snapshot","file":"b.go","text":"b\n"}`,
				`{"v":1,"seq":4,"type":"snapshot","file":"a.go","text":"Z\n"}`,
				`{"v":1,"seq":5,"type":"select","file":"a.go","startLine":1,"endLine":1}`,
			},
			wantBefore: "1\n", wantAfter: "1\n", wantRange: Range{1, 1}, wantLastState: "Z\n",
		},
		{
			name: "old format with nothing after it",
			lines: []string{
				`{"v":1,"seq":1,"type":"snapshot","file":"a.go","text":"1\n"}`,
				`{"v":1,"seq":2,"type":"external","file":"a.go"}`,
			},
			wantBefore: "1\n", wantAfter: "1\n", wantRange: Range{1, 1}, wantLastState: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Build(parse(t, tt.lines...)).Frames()
			x := f[0]
			if x.Kind != KindExternal || x.Before != tt.wantBefore || x.After != tt.wantAfter || x.Range != tt.wantRange || x.Deleted != tt.wantDeleted ||
				x.Why != nil || x.Selection != nil || x.From != nil {
				t.Errorf("external frame = %+v", x)
			}
			if len(f) > 1 && f[1].Before != tt.wantLastState {
				t.Errorf("the next frame is shown on %q, want %q", f[1].Before, tt.wantLastState)
			}
		})
	}
}

func TestReplaceWithoutANewRange(t *testing.T) {
	ev := parse(t,
		`{"v":1,"seq":1,"type":"snapshot","file":"a.go","text":"1\n2\n"}`,
		`{"v":1,"seq":2,"type":"replace","file":"a.go","startLine":2,"endLine":2,"oldText":"2","newText":"x\ny"}`,
	)
	f := Build(ev).Frames()[0]
	if f.Range != (Range{2, 3}) || f.After != "1\nx\ny\n" {
		t.Errorf("frame = %+v", f)
	}
}

func TestOnlyFramesCountAsTouching(t *testing.T) {
	ev := parse(t,
		`{"v":1,"seq":1,"type":"snapshot","file":"snap-only.go","text":"s\n"}`,
		`{"v":1,"seq":2,"type":"snapshot","file":"b.go","text":"b\n"}`,
		`{"v":1,"seq":3,"type":"select","file":"b.go","startLine":1,"endLine":1}`,
		`{"v":1,"seq":4,"type":"select","file":"a.go","startLine":1,"endLine":1}`,
		`{"v":1,"seq":5,"type":"select","file":"b.go","startLine":1,"endLine":1}`,
	)
	b := Build(ev)
	if !slices.Equal(b.Files(), []string{"b.go", "a.go"}) || b.Ops() != 3 {
		t.Errorf("files = %v, ops = %d", b.Files(), b.Ops())
	}
}

// A tape read whole and a tape read event by event give the same frames.
func TestAddOneByOne(t *testing.T) {
	ev := parse(t,
		`{"v":1,"type":"header","session":"x"}`,
		`{"v":1,"seq":1,"type":"snapshot","file":"a.go","text":"1\n"}`,
		`{"v":1,"seq":2,"type":"select","file":"a.go","startLine":1,"endLine":1,"selection":"s1"}`,
		`{"v":1,"seq":3,"type":"replace","file":"a.go","from":"s1","startLine":1,"endLine":1,"newText":"2","newStartLine":1,"newEndLine":1,"selection":"s2"}`,
	)
	b := NewBuilder()
	var added []bool
	for _, e := range ev {
		added = append(added, b.Add(e))
	}
	if want := []bool{false, false, true, true}; !slices.Equal(added, want) {
		t.Errorf("Add reported %v, want %v", added, want)
	}
	if got, want := b.Frames(), Build(ev).Frames(); len(got) != len(want) || got[1].Before != want[1].Before || *got[1].Parent != 0 {
		t.Errorf("frames differ: %+v vs %+v", got, want)
	}
}

func TestAppendFinals(t *testing.T) {
	state := tape.Build(parse(t,
		`{"v":1,"seq":1,"type":"snapshot","file":"same.go","text":"s\n"}`,
		`{"v":1,"seq":2,"type":"snapshot","file":"changed.go","text":"c\n"}`,
		`{"v":1,"seq":3,"type":"snapshot","file":"gone.go","text":"g\n"}`,
		`{"v":1,"seq":4,"type":"snapshot","file":"deleted.go","text":"d\n"}`,
		`{"v":1,"seq":5,"type":"external","file":"deleted.go","deleted":true}`,
		`{"v":1,"seq":6,"type":"snapshot","file":"deleted-and-gone.go","text":"x\n"}`,
		`{"v":1,"seq":7,"type":"external","file":"deleted-and-gone.go","deleted":true}`,
	))
	files := []string{"same.go", "changed.go", "gone.go", "deleted.go", "deleted-and-gone.go", "untouched-by-snapshot.go", "empty.go"}
	current := map[string]string{"same.go": "s\n", "changed.go": "c2\nmore\n", "deleted.go": "", "empty.go": ""}
	read := func(rel string) (string, bool) { s, ok := current[rel]; return s, ok }

	prev := []Frame{{Index: 0, Kind: KindSelect, Seq: 9, TS: 123}}
	got := AppendFinals(prev, state, files, read)
	var names []string
	for _, f := range got[1:] {
		names = append(names, f.File)
	}
	// same.go and empty.go are as the tape left them; deleted-and-gone.go is gone as the tape says.
	// untouched-by-snapshot.go has no content in the tape and does not exist: it counts as an empty file that has been removed.
	if want := []string{"changed.go", "gone.go", "deleted.go", "untouched-by-snapshot.go"}; !slices.Equal(names, want) {
		t.Fatalf("final frames for %v, want %v", names, want)
	}
	byName := map[string]Frame{}
	for _, f := range got[1:] {
		byName[f.File] = f
		if f.Kind != KindFinal || f.Seq != 9 || f.TS != 123 || f.Index < 1 {
			t.Errorf("%s: %+v", f.File, f)
		}
	}
	if c := byName["changed.go"]; c.Before != "c\n" || c.After != "c2\nmore\n" || c.Deleted || c.Range != (Range{1, 2}) {
		t.Errorf("changed.go = %+v", c)
	}
	if g := byName["gone.go"]; g.Before != "g\n" || g.After != "" || !g.Deleted || g.Range != (Range{1, 0}) {
		t.Errorf("gone.go = %+v", g)
	}
	// The tape says it was deleted, the file is there now (empty): a frame, but it is not "deleted".
	if d := byName["deleted.go"]; d.Before != "" || d.After != "" || d.Deleted {
		t.Errorf("deleted.go = %+v", d)
	}

	// No frames before: seq and ts are 0.
	only := AppendFinals(nil, state, []string{"gone.go"}, read)
	if len(only) != 1 || only[0].Index != 0 || only[0].Seq != 0 || only[0].TS != 0 {
		t.Errorf("final without frames = %+v", only)
	}
}

func TestContentAt(t *testing.T) {
	frames := []Frame{
		{File: "a", Before: "a0", After: "a0"},
		{File: "b", Before: "b0", After: "b1"},
		{File: "a", Before: "a0", After: "a1"},
		{File: "b", Before: "b1", After: "b1"},
	}
	tests := []struct {
		file string
		i    int
		want string
		ok   bool
	}{
		{"a", -1, "a0", true}, // nobody has touched it yet: before the first frame that does
		{"a", 0, "a0", true},
		{"a", 1, "a0", true},
		{"a", 2, "a1", true},
		{"a", 3, "a1", true},
		{"b", -1, "b0", true},
		{"b", 0, "b0", true}, // not yet changed
		{"b", 1, "b1", true},
		{"b", 3, "b1", true},
		{"c", 1, "", false},
		{"a", 99, "a1", true},
	}
	for _, tt := range tests {
		got, ok := ContentAt(frames, tt.file, tt.i)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ContentAt(%s, %d) = %q, %v; want %q, %v", tt.file, tt.i, got, ok, tt.want, tt.ok)
		}
	}
	if _, ok := ContentAt(nil, "a", 0); ok {
		t.Error("ContentAt of no frames should be none")
	}
}

// The vcs of the header is for people who look at the tape; the frames are the same with it, without it, and with null.
func TestVCSOfTheHeaderDoesNotChangeTheFrames(t *testing.T) {
	body := []string{
		`{"v":1,"seq":1,"ts":"2026-09-29T03:00:01.000Z","type":"snapshot","file":"a.go","text":"1\n2\n3\n"}`,
		`{"v":1,"seq":2,"ts":"2026-09-29T03:00:02.000Z","type":"select","file":"a.go","startLine":2,"endLine":3,"why":"見る","selection":"sel_1"}`,
	}
	headers := []string{
		`{"v":1,"type":"header","session":"a1b2","startedAt":"2026-09-29T03:00:00.000Z","author":{"kind":"ai","name":"claude"}}`,
		`{"v":1,"type":"header","session":"a1b2","startedAt":"2026-09-29T03:00:00.000Z","author":{"kind":"ai","name":"claude"},"vcs":null}`,
		`{"v":1,"type":"header","session":"a1b2","startedAt":"2026-09-29T03:00:00.000Z","author":{"kind":"ai","name":"claude"},"vcs":{"type":"git","head":"` + strings.Repeat("0", 40) + `","dirty":true,"future":[1]}}`,
		`{"v":1,"type":"header","session":"a1b2","startedAt":"2026-09-29T03:00:00.000Z","author":{"kind":"ai","name":"claude"},"vcs":{"type":"git","head":null,"dirty":false}}`,
	}
	var want []Frame
	for i, h := range headers {
		got := Build(parse(t, append([]string{h}, body...)...)).Frames()
		if i == 0 {
			want = got
			continue
		}
		gj, _ := json.Marshal(got)
		wj, _ := json.Marshal(want)
		if string(gj) != string(wj) || got[0].Before != want[0].Before || got[0].After != want[0].After {
			t.Errorf("header %d: frames %s, want %s", i, gj, wj)
		}
	}
}
