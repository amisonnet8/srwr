package tape

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

const fixtures = "../../extension/test/fixtures"

func readTape(t *testing.T, path string) Result {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // a fixture path found by Glob
	if err != nil {
		t.Fatal(err)
	}
	return Parse(b)
}

func realTapes(t *testing.T) []string {
	t.Helper()
	a, _ := filepath.Glob(filepath.Join(fixtures, "*.tape.jsonl"))
	b, _ := filepath.Glob(filepath.Join(fixtures, "ui-check", ".srwr", "tapes", "*.tape.jsonl"))
	all := append(a, b...)
	if len(all) != 7 {
		t.Fatalf("found %d real tapes, want 7: %v", len(all), all)
	}
	return all
}

// The expected.json files are the answers of the earlier implementation (extension/test/fixtures).
func TestFixturesMatchExpected(t *testing.T) {
	for _, name := range []string{"basic", "external-text", "phase0-bash", "real-playground"} {
		t.Run(name, func(t *testing.T) {
			res := readTape(t, filepath.Join(fixtures, name+".tape.jsonl"))
			if res.Skipped != 0 {
				t.Errorf("skipped %d lines", res.Skipped)
			}
			b, err := os.ReadFile(filepath.Join(fixtures, name+".expected.json")) //nolint:gosec // a fixture path
			if err != nil {
				t.Fatal(err)
			}
			var want struct {
				Files map[string]string `json:"files"`
				Kinds []string          `json:"kinds"`
			}
			if err := json.Unmarshal(b, &want); err != nil {
				t.Fatal(err)
			}

			st := Build(res.Events)
			got := map[string]string{}
			for name, f := range st.Files {
				if !f.Deleted {
					got[name] = f.Text
				}
			}
			if len(got) != len(want.Files) {
				t.Errorf("files = %d, want %d", len(got), len(want.Files))
			}
			for name, text := range want.Files {
				if got[name] != text {
					t.Errorf("file %s = %q, want %q", name, got[name], text)
				}
			}

			var kinds []string
			for _, e := range res.Events {
				if e.Type != TypeHeader && e.Type != TypeSnapshot {
					kinds = append(kinds, e.Type)
				}
			}
			// The expected files are the golden data of version 1, which says select and replace.
			var wantKinds []string
			for _, k := range want.Kinds {
				wantKinds = append(wantKinds, fromVersion1(k, ""))
			}
			if !slices.Equal(kinds, wantKinds) {
				t.Errorf("kinds = %v, want %v", kinds, wantKinds)
			}
		})
	}
}

// Real tapes carry hashes of their own contents, so replaying them checks Splice, Sha and FileHash together.
func TestRealTapesReplayConsistently(t *testing.T) {
	for _, path := range realTapes(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			res := readTape(t, path)
			if res.Skipped != 0 {
				t.Errorf("skipped %d lines", res.Skipped)
			}
			if res.Events[0].Type != TypeHeader {
				t.Errorf("first event is %q, want header", res.Events[0].Type)
			}
			st := NewState()
			seq := 0
			for _, e := range res.Events[1:] {
				seq++
				if e.Seq != seq {
					t.Fatalf("seq = %d, want %d (no gaps)", e.Seq, seq)
				}
				before := ""
				if f := st.Files[e.File]; f != nil {
					before = f.Text
				}
				switch e.Type {
				case TypeSnapshot:
					if got := Sha(*e.Text); got != e.Sha {
						t.Errorf("seq %d: Sha(text) = %s, want %s", e.Seq, got, e.Sha)
					}
					if got := FileHash(e.File); got != e.FileHash {
						t.Errorf("seq %d: FileHash(%s) = %s, want %s", e.Seq, e.File, got, e.FileHash)
					}
				case TypeLook:
					if e.Selection != nil && st.Files[e.File] == nil {
						t.Errorf("seq %d: select of %s before any snapshot", e.Seq, e.File)
					}
				case TypeEdit:
					if got := Sha(before); got != e.FileShaBefore {
						t.Errorf("seq %d: sha before = %s, want %s", e.Seq, got, e.FileShaBefore)
					}
					if got := RangeText(before, e.StartLine, e.EndLine); got != e.OldText {
						t.Errorf("seq %d: RangeText = %q, want oldText %q", e.Seq, got, e.OldText)
					}
				case TypeExternal:
					if got := Sha(before); got != e.ExpectedSha {
						t.Errorf("seq %d: expectedSha = %s, want %s", e.Seq, e.ExpectedSha, got)
					}
					if e.Text != nil && Sha(*e.Text) != e.ActualSha {
						t.Errorf("seq %d: Sha(text) != actualSha", e.Seq)
					}
				}
				st.Apply(e)
				if e.Type == TypeEdit {
					after := st.Files[e.File].Text
					if got := Sha(after); got != e.FileShaAfter {
						t.Errorf("seq %d: sha after = %s, want %s", e.Seq, got, e.FileShaAfter)
					}
					if got := RangeText(after, e.NewStartLine, e.NewEndLine); got != e.NewText {
						t.Errorf("seq %d: new RangeText = %q, want newText %q", e.Seq, got, e.NewText)
					}
				}
			}
			if st.LastSeq != seq {
				t.Errorf("LastSeq = %d, want %d", st.LastSeq, seq)
			}
		})
	}
}

func TestParseLeniency(t *testing.T) {
	const header = `{"v":1,"type":"header","session":"x","startedAt":"2026-09-29T03:00:00.000Z"}` + "\n"
	snap := `{"v":1,"seq":1,"ts":"t","type":"snapshot","file":"a.go","text":"1\n2\n3\n"}` + "\n"
	tests := []struct {
		name         string
		in           string
		wantTypes    []string
		wantSkipped  int
		wantConsumed int
	}{
		{"not json", "not json\n", nil, 1, -1},
		{"json but not an object", "[1]\n\"x\"\nnull\n", nil, 3, -1},
		{"blank lines", "\n\n" + snap, []string{"snapshot"}, 0, -1},
		{"unknown type", `{"v":1,"seq":9,"type":"foo","file":"a.go"}` + "\n", nil, 1, -1},
		{"no seq", `{"v":1,"type":"select","file":"a.go","startLine":1,"endLine":1}` + "\n", nil, 1, -1},
		{"no file", `{"v":1,"seq":2,"type":"select","startLine":1,"endLine":1}` + "\n", nil, 1, -1},
		{"no range", `{"v":1,"seq":2,"type":"select","file":"a.go","why":"x"}` + "\n", nil, 1, -1},
		{"replace without newText", `{"v":1,"seq":2,"type":"replace","file":"a.go","startLine":1,"endLine":1}` + "\n", nil, 1, -1},
		{"snapshot without text", `{"v":1,"seq":1,"type":"snapshot","file":"a.go"}` + "\n", nil, 1, -1},
		{"header and snapshot", header + snap, []string{"header", "snapshot"}, 0, -1},
		{"last line without newline is held back", snap + `{"v":1,"seq":2,"type":"select"`, []string{"snapshot"}, 0, len(snap)},
		{"only a partial line", `{"v":1,"seq":1`, nil, 0, 0},
		{"empty", "", nil, 0, 0},
		{"unknown fields are ignored", `{"v":1,"seq":2,"type":"select","file":"a.go","startLine":1,"endLine":1,"extra":{"a":1}}` + "\n", []string{"look"}, 0, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Parse([]byte(tt.in))
			var types []string
			for _, e := range res.Events {
				types = append(types, e.Type)
			}
			if !slices.Equal(types, tt.wantTypes) {
				t.Errorf("types = %v, want %v", types, tt.wantTypes)
			}
			if res.Skipped != tt.wantSkipped {
				t.Errorf("Skipped = %d, want %d", res.Skipped, tt.wantSkipped)
			}
			want := tt.wantConsumed
			if want < 0 {
				want = len(tt.in) // every line is complete
			}
			if res.Consumed != want {
				t.Errorf("Consumed = %d, want %d", res.Consumed, want)
			}
		})
	}
}

func TestParseFieldTypes(t *testing.T) {
	// The bad-lines case of the golden data: values of the wrong type are read as null; an unreadable ts is kept as text.
	in := `{"v":1,"seq":13,"ts":"yesterday","type":"select","file":"a.go","startLine":3,"endLine":3,"why":5,"selection":7}` + "\n" +
		`{"v":1,"seq":14,"ts":"t","type":"select","file":"a.go","startLine":"3","endLine":3}` + "\n" +
		`{"v":1,"seq":15,"ts":"t","type":"select","file":"a.go","startLine":1,"endLine":1,"why":null,"selection":null}` + "\n"
	res := Parse([]byte(in))
	if len(res.Events) != 2 || res.Skipped != 1 {
		t.Fatalf("events = %d, skipped = %d; want 2 and 1", len(res.Events), res.Skipped)
	}
	a, b := res.Events[0], res.Events[1]
	if a.Why != nil || a.Selection != nil || a.TS != "yesterday" || a.Seq != 13 {
		t.Errorf("first event = %+v", a)
	}
	if b.Why != nil || b.Selection != nil {
		t.Errorf("explicit nulls: %+v", b)
	}
}

func TestParseOldFormats(t *testing.T) {
	in := `{"v":1,"seq":3,"type":"external","file":"a.go","expectedSha":"e","actualSha":"a"}` + "\n" +
		`{"v":1,"seq":4,"type":"external","file":"a.go","deleted":true,"text":null}` + "\n" +
		`{"v":1,"seq":5,"type":"external","file":"a.go","author":{"kind":"external"},"text":""}` + "\n" +
		`{"v":1,"seq":6,"type":"select","file":"a.go","startLine":1,"endLine":1,"why":"x","selection":"sel_1"}` + "\n" +
		`{"v":1,"seq":7,"type":"select","file":"a.go","startLine":1,"endLine":1,"why":null,"selection":null,"source":"hook","tool":"Read"}` + "\n"
	ev := Parse([]byte(in)).Events
	if len(ev) != 5 {
		t.Fatalf("events = %d, want 5", len(ev))
	}
	if ev[0].Text != nil || ev[0].Deleted {
		t.Errorf("external without text: %+v", ev[0])
	}
	if ev[1].Text != nil || !ev[1].Deleted {
		t.Errorf("deleted external: %+v", ev[1])
	}
	if ev[2].Text == nil || *ev[2].Text != "" || ev[2].Author == nil || ev[2].Author.Kind != "external" {
		t.Errorf("external with empty text: %+v", ev[2])
	}
	if ev[3].Source != "" {
		t.Errorf("a select without source should have Source %q (read as mcp), got %q", "", ev[3].Source)
	}
	if ev[4].Source != SourceHook || ev[4].HookTool != "Read" {
		t.Errorf("hook select: %+v", ev[4])
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	events := []Event{
		{Type: TypeHeader, Session: "a1b2", StartedAt: "2026-09-29T11:20:00.000+09:00", Author: &Author{Kind: "ai", Name: "claude"}, Tool: &ToolInfo{Name: "srwr", Version: "(devel)"}},
		{Type: TypeHeader, Session: "c3d4", StartedAt: "2026-10-03T11:20:00.000Z", Author: &Author{Kind: "ai", Name: "claude"}, VCS: json.RawMessage(`{"type":"git","head":"` + strings.Repeat("0123456789", 4) + `","dirty":true}`), Tool: &ToolInfo{Name: "srwr", Version: "(devel)"}},
		{Type: TypeHeader, Session: "e5f6", StartedAt: "2026-10-03T11:20:00.000Z", Author: &Author{Kind: "ai", Name: "claude"}, VCS: json.RawMessage(`{"type":"git","head":null,"dirty":false}`), Tool: &ToolInfo{Name: "srwr", Version: "(devel)"}},
		{Type: TypeSnapshot, Seq: 1, TS: "t", File: "a.go", FileHash: FileHash("a.go"), Text: Str("x <b> & y\n"), Sha: Sha("x <b> & y\n")},
		{Type: TypeLook, Seq: 2, TS: "t", File: "a.go", StartLine: 1, EndLine: 1, Why: Str("見る"), Selection: Str("sel_1"), Source: SourceMCP},
		{Type: TypeLook, Seq: 3, TS: "t", File: "a.go", StartLine: 1, EndLine: 1, Source: SourceHook, HookTool: "Grep"},
		{Type: TypeEdit, Seq: 4, TS: "t", File: "a.go", From: Str("sel_1"), StartLine: 1, EndLine: 1, OldText: "o", NewText: "n\"\n", NewStartLine: 1, NewEndLine: 2, Selection: Str("sel_2"), Why: Str("w"), FileShaBefore: "b", FileShaAfter: "a", Source: SourceMCP},
		{Type: TypeExternal, Seq: 5, TS: "t", File: "a.go", Author: &Author{Kind: "external"}, DetectedBy: "look", ExpectedSha: "e", ActualSha: "a", Text: Str("changed")},
		{Type: TypeExternal, Seq: 6, TS: "t", File: "a.go", Author: &Author{Kind: "external"}, DetectedBy: "hook", ExpectedSha: "e", ActualSha: "a", Deleted: true},
	}
	var all []byte
	for _, e := range events {
		line, err := Marshal(e)
		if err != nil {
			t.Fatalf("Marshal(%s): %v", e.Type, err)
		}
		if !bytes.HasSuffix(line, []byte("}\n")) || bytes.Count(line, []byte("\n")) != 1 {
			t.Errorf("not exactly one line: %q", line)
		}
		if !bytes.HasPrefix(line, []byte(`{"v":2,`)) {
			t.Errorf("v must come first: %q", line)
		}
		all = append(all, line...)
	}
	res := Parse(all)
	if res.Skipped != 0 || len(res.Events) != len(events) {
		t.Fatalf("read back %d events, skipped %d", len(res.Events), res.Skipped)
	}
	for i, got := range res.Events {
		want := events[i]
		if want.VCS == nil {
			want.VCS = json.RawMessage("null")
		}
		gj, _ := json.Marshal(got)
		wj, _ := json.Marshal(want)
		if string(gj) != string(wj) {
			t.Errorf("event %d:\n got %s\nwant %s", i, gj, wj)
		}
	}
	if !strings.Contains(string(all), "x <b> & y") {
		t.Error("<, > and & should not be escaped")
	}
}

func TestMarshalWritesNull(t *testing.T) {
	line, err := Marshal(Event{Type: TypeLook, Seq: 1, TS: "t", File: "a.go", StartLine: 1, EndLine: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"v":2,"seq":1,"ts":"t","type":"look","file":"a.go","startLine":1,"endLine":1,"why":null,"selection":null}` + "\n"
	if string(line) != want {
		t.Errorf("got %s want %s", line, want)
	}
	line, _ = Marshal(Event{Type: TypeEdit, Seq: 1, TS: "t", File: "a.go", StartLine: 1, EndLine: 0, NewText: "x", NewStartLine: 1, NewEndLine: 1})
	if !strings.Contains(string(line), `"from":null`) || !strings.Contains(string(line), `"selection":null`) || !strings.Contains(string(line), `"why":null`) {
		t.Errorf("missing nulls: %s", line)
	}
	line, _ = Marshal(Event{Type: TypeExternal, Seq: 1, TS: "t", File: "a.go"})
	if !strings.Contains(string(line), `"text":null`) || strings.Contains(string(line), "deleted") {
		t.Errorf("external: %s", line)
	}
}

func TestMarshalErrors(t *testing.T) {
	for _, e := range []Event{{Type: "foo"}, {Type: TypeSnapshot, Seq: 1, File: "a.go"}} {
		if _, err := Marshal(e); err == nil {
			t.Errorf("Marshal(%+v) should fail", e)
		}
	}
}

func TestAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.tape.jsonl")
	for i := 1; i <= 3; i++ {
		if err := Append(path, Event{Type: TypeLook, Seq: i, TS: "t", File: "a.go", StartLine: 1, EndLine: 1}); err != nil {
			t.Fatal(err)
		}
	}
	res := readTape(t, path)
	if len(res.Events) != 3 || res.Skipped != 0 || res.Events[2].Seq != 3 {
		t.Errorf("events = %+v, skipped = %d", res.Events, res.Skipped)
	}
}

func TestFormatTS(t *testing.T) {
	jst := time.FixedZone("JST", 9*3600)
	tests := []struct {
		in   time.Time
		want string
	}{
		{time.Date(2026, 9, 29, 11, 20, 4, 0, jst), "2026-09-29T02:20:04.000Z"},
		{time.Date(2026, 9, 29, 11, 20, 4, 123456789, jst), "2026-09-29T02:20:04.123Z"},
		{time.Date(2026, 9, 29, 3, 0, 1, 5e6, time.UTC), "2026-09-29T03:00:01.005Z"},
	}
	for _, tt := range tests {
		if got := FormatTS(tt.in); got != tt.want {
			t.Errorf("FormatTS = %s, want %s", got, tt.want)
		}
	}
}

func TestStateExternal(t *testing.T) {
	events := Parse([]byte(strings.Join([]string{
		`{"v":1,"seq":1,"type":"snapshot","file":"a.go","text":"old\n"}`,
		`{"v":1,"seq":2,"type":"external","file":"a.go","deleted":true,"text":null}`,
		`{"v":1,"seq":3,"type":"snapshot","file":"b.go","text":"b\n"}`,
		`{"v":1,"seq":4,"type":"external","file":"b.go","expectedSha":"e","actualSha":"a"}`,
		`{"v":1,"seq":5,"type":"external","file":"c.go","text":"new c\n"}`,
		"",
	}, "\n"))).Events
	st := Build(events)
	if f := st.Files["a.go"]; f == nil || !f.Deleted || f.Text != "" {
		t.Errorf("a.go = %+v, want deleted", f)
	}
	if f := st.Files["b.go"]; f == nil || f.Text != "b\n" {
		t.Errorf("b.go = %+v: an external without text must leave the content for the next snapshot", f)
	}
	if f := st.Files["c.go"]; f == nil || f.Text != "new c\n" {
		t.Errorf("c.go = %+v", f)
	}
	// A snapshot after the deletion brings the file back.
	st.Apply(Event{Type: TypeSnapshot, Seq: 6, File: "a.go", Text: Str("back\n")})
	if f := st.Files["a.go"]; f.Deleted || f.Text != "back\n" || st.LastSeq != 6 {
		t.Errorf("a.go = %+v, LastSeq = %d", f, st.LastSeq)
	}
}

func TestStateRemembersTheLastLookAndChange(t *testing.T) {
	st := Build(Parse([]byte(strings.Join([]string{
		`{"v":1,"seq":1,"type":"snapshot","file":"a.go","text":"a\nb\n"}`,
		`{"v":1,"seq":2,"type":"select","file":"a.go","startLine":1,"endLine":2,"source":"mcp"}`,
		`{"v":1,"seq":3,"type":"replace","file":"a.go","startLine":1,"endLine":1,"newText":"x","newStartLine":1,"newEndLine":1}`,
		`{"v":2,"seq":4,"type":"look","file":"a.go","startLine":1,"endLine":2,"source":"hook","tool":"Read"}`,
		`{"v":2,"seq":5,"type":"look","file":"b.go","startLine":1,"endLine":1,"source":"mcp"}`,
		`{"v":2,"seq":6,"type":"external","file":"b.go","text":"t\n"}`,
		"",
	}, "\n"))).Events)
	if st.LastLook["a.go"] != 4 || st.LastChange["a.go"] != 3 || st.LastLook["b.go"] != 5 || st.LastChange["b.go"] != 6 {
		t.Errorf("LastLook = %v, LastChange = %v", st.LastLook, st.LastChange)
	}
}

func TestStateCollectsReplaces(t *testing.T) {
	res := readTape(t, filepath.Join(fixtures, "basic.tape.jsonl"))
	st := Build(res.Events)
	if len(st.Edits) != 5 || st.LastSeq != 13 {
		t.Errorf("Edits = %d, LastSeq = %d; want 5 and 13", len(st.Edits), st.LastSeq)
	}
}

func TestValidID(t *testing.T) {
	tests := map[string]bool{
		"20261001-1706-1795": true,
		"a_b.c-D":            true,
		"":                   false,
		".":                  false,
		"..":                 false,
		".hidden":            false,
		"a/b":                false,
		`a\b`:                false,
		"../x":               false,
		"a b":                false,
		"a\x00b":             false,
		"テープ":                false,
	}
	for id, want := range tests {
		if got := ValidID(id); got != want {
			t.Errorf("ValidID(%q) = %v, want %v", id, got, want)
		}
	}
	if got := FileName("x"); got != "x.tape.jsonl" {
		t.Errorf("FileName = %q", got)
	}
}

func TestCreatedExternalRoundTrips(t *testing.T) {
	e := Event{
		Type: TypeExternal, Seq: 5, TS: "2026-10-04T00:00:00.000Z", File: "new.go", Author: &Author{Kind: "external"},
		DetectedBy: "hook", ActualSha: "sha256:b", Created: true,
		Hunks: []Hunk{{StartLine: 1, EndLine: 0, NewText: "a\nb", NewStartLine: 1, NewEndLine: 2}},
	}
	line, err := Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(line), `"created":true`) {
		t.Errorf("line = %s", line)
	}
	got, ok := parseLine(line)
	if !ok || !got.Created || !reflect.DeepEqual(got.Hunks, e.Hunks) {
		t.Errorf("parsed %+v (ok %v)", got, ok)
	}
	// Without Created the field is not written; and the state of a new file is its text.
	e.Created = false
	if line, _ := Marshal(e); strings.Contains(string(line), "created") {
		t.Errorf("created written for an ordinary external: %s", line)
	}
	e.Created = true
	if text := Build([]Event{e}).Files["new.go"].Text; text != "a\nb\n" {
		t.Errorf("text = %q", text)
	}
}

func TestFailureRoundTrips(t *testing.T) {
	two, nine := 3, 9
	f := &FailureInfo{Tool: "look", StartLine: &two, EndLine: &nine, Why: Str("why"), Code: "invalid_range", Message: "m"}
	e := Event{Type: TypeFailure, Seq: 7, TS: "2026-10-04T00:00:00.000Z", Failure: f}
	line, err := Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"v":2,"seq":7,"ts":"2026-10-04T00:00:00.000Z","type":"failure","tool":"look","file":null,"startLine":3,"endLine":9,"selection":null,"why":"why","code":"invalid_range","message":"m"}` + "\n"
	if string(line) != want {
		t.Errorf("line = %s, want %s", line, want)
	}
	got, ok := parseLine(line)
	if !ok || got.Seq != 7 || !reflect.DeepEqual(got.Failure, f) {
		t.Errorf("parsed %+v (ok %v)", got, ok)
	}
	// A replace has a token and no range, and a file when it is known.
	r := &FailureInfo{Tool: "edit", File: Str("a.go"), Selection: Str("sel_x"), Code: "selection_stale", Message: "m"}
	line, _ = Marshal(Event{Type: TypeFailure, Seq: 8, Failure: r})
	if got, ok := parseLine(line); !ok || !reflect.DeepEqual(got.Failure, r) || !strings.Contains(string(line), `"startLine":null`) {
		t.Errorf("replace: %+v %s", got.Failure, line)
	}
	// The state does not change.
	s := Build([]Event{{Type: TypeSnapshot, Seq: 1, File: "a.go", Text: Str("x\n")}, {Type: TypeFailure, Seq: 2, Failure: r}})
	if s.Files["a.go"].Text != "x\n" || s.LastSeq != 2 {
		t.Errorf("state = %+v", s)
	}
}

// A tape of version 1 says select and replace; sub and new were a replace with a tool. Parse reads them (and the replace of version 2) as look, edit and new,
// by the "v" of each line, so that a tape that went on with version 2 is read right too.
func TestParseVersion1(t *testing.T) {
	rng := `"file":"a.go","startLine":1,"endLine":1,`
	repl := `"newText":"x","oldText":"o","newStartLine":1,"newEndLine":1,"hits":2`
	in := `{"v":1,"seq":1,"type":"select",` + rng + `"why":"w","selection":"s"}` + "\n" +
		`{"v":1,"seq":2,"type":"replace",` + rng + repl + `}` + "\n" +
		`{"v":1,"seq":3,"type":"replace",` + rng + repl + `,"tool":"sub"}` + "\n" +
		`{"v":1,"seq":4,"type":"replace",` + rng + repl + `,"tool":"new"}` + "\n" +
		`{"v":1,"seq":5,"type":"replace",` + rng + repl + `,"source":"hook","tool":"Edit"}` + "\n" +
		`{"v":2,"seq":6,"type":"replace",` + rng + repl + `}` + "\n" +
		`{"v":2,"seq":7,"type":"look",` + rng + `"source":"hook","tool":"Read"}` + "\n" +
		`{"v":1,"seq":8,"type":"failure","tool":"sub","code":"count_mismatch","message":"m"}` + "\n" +
		`{"v":1,"seq":9,"type":"failure","tool":"replace","code":"selection_stale","message":"m"}` + "\n" +
		`{"v":2,"seq":10,"type":"failure","tool":"replace","code":"count_mismatch","message":"m"}` + "\n" +
		`{"seq":11,"type":"select",` + rng + `"why":"w","selection":"s"}` + "\n"
	res := Parse([]byte(in))
	if res.Skipped != 0 || len(res.Events) != 11 {
		t.Fatalf("events = %d, skipped = %d", len(res.Events), res.Skipped)
	}
	want := []struct{ typ, hookTool, tool string }{
		{TypeLook, "", ""}, {TypeEdit, "", ""}, {TypeEdit, "", ""}, {TypeNew, "", ""}, {TypeEdit, "Edit", ""},
		{TypeEdit, "", ""}, {TypeLook, "Read", ""},
		{TypeFailure, "", TypeEdit}, {TypeFailure, "", TypeEdit}, {TypeFailure, "", TypeEdit}, {TypeLook, "", ""},
	}
	for i, w := range want {
		e := res.Events[i]
		tool := ""
		if e.Failure != nil {
			tool = e.Failure.Tool
		}
		if e.Type != w.typ || e.HookTool != w.hookTool || tool != w.tool {
			t.Errorf("event %d = %s (hook tool %q, failed tool %q), want %s (%q, %q)", i+1, e.Type, e.HookTool, tool, w.typ, w.hookTool, w.tool)
		}
	}
}

func TestHeaderTitleIsWrittenOnlyWhenThereIsOne(t *testing.T) {
	plain, err := Marshal(Event{Type: TypeHeader, Session: "a1b2", StartedAt: "2026-10-10T00:00:00.000Z", Author: &Author{Kind: "ai", Name: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "title") || strings.Contains(string(plain), `"why"`) {
		t.Errorf("a header with no title: %s", plain)
	}
	titled, err := Marshal(Event{Type: TypeHeader, Session: "a1b2", StartedAt: "2026-10-10T00:00:00.000Z", Title: "docs first", Why: Str("wording")})
	if err != nil {
		t.Fatal(err)
	}
	res := Parse(titled)
	if len(res.Events) != 1 || res.Events[0].Title != "docs first" || res.Events[0].Why == nil || *res.Events[0].Why != "wording" {
		t.Errorf("read back = %+v (%s)", res.Events, titled)
	}
	if got := Parse(plain).Events[0]; got.Title != "" || got.Why != nil {
		t.Errorf("plain header read as %+v", got)
	}
}
