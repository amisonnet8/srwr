package viewserver

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amisonnet8/srwr/internal/tape"
)

// workspace makes a workspace with the given tapes (ID -> lines of the tape) and files.
func workspace(t *testing.T, tapes map[string][]string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".srwr", "tapes"), 0o750); err != nil {
		t.Fatal(err)
	}
	for id, lines := range tapes {
		writeTape(t, root, id, strings.Join(lines, "\n")+"\n")
	}
	for name, text := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeTape(t *testing.T, root, id, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".srwr", "tapes", tape.FileName(id)), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A small tape: select line 2 of a.go, replace it.
func smallTape(started string) []string {
	return []string{
		`{"v":1,"type":"header","session":"s","startedAt":"` + started + `"}`,
		`{"v":1,"seq":1,"ts":"2026-10-01T08:00:00.000Z","type":"snapshot","file":"a.go","fileHash":"x","text":"1\n2\n3\n","sha":"x"}`,
		`{"v":1,"seq":2,"ts":"2026-10-01T08:00:01.000Z","type":"select","file":"a.go","startLine":2,"endLine":2,"why":"見る","selection":"sel_A"}`,
		`{"v":1,"seq":3,"ts":"2026-10-01T08:00:02.000Z","type":"replace","file":"a.go","from":"sel_A","startLine":2,"endLine":2,"oldText":"2","newText":"two","newStartLine":2,"newEndLine":2,"selection":"sel_B","why":"変える"}`,
	}
}

func TestInitialize(t *testing.T) {
	root := workspace(t, nil, nil)
	srv := &Server{Root: root, Version: "v-test"}
	got := exchange(t, srv, initReq(1, map[string]any{}))
	if want := `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"serverVersion":"v-test"}}`; got[0] != want {
		t.Errorf("got %s\nwant %s", got[0], want)
	}

	t.Run("a different protocol version", func(t *testing.T) {
		got := exchange(t, srv, req(1, "initialize", map[string]any{"client": "vim", "protocolVersion": 2}))
		if rpc, code := errorOf(t, got[0]); rpc != -32000 || code != "protocol_mismatch" {
			t.Errorf("got %s", got[0])
		}
	})
	t.Run("no protocol version", func(t *testing.T) {
		got := exchange(t, srv, req(1, "initialize", map[string]any{"client": "vim"}))
		if rpc, code := errorOf(t, got[0]); rpc != -32602 || code != "invalid_params" {
			t.Errorf("got %s", got[0])
		}
	})
	t.Run("options the server does not know are ignored", func(t *testing.T) {
		got := exchange(t, srv, initReq(1, map[string]any{"jumpLabels": true, "jumpThresholdLines": 30, "future": 1}))
		if !strings.Contains(got[0], `"result"`) {
			t.Errorf("got %s", got[0])
		}
	})
}

func TestRequestsBeforeInitialize(t *testing.T) {
	srv := &Server{Root: workspace(t, nil, nil)}
	for _, m := range []string{"tapes/list", "tape/open", "frame/state", "tape/close", "live/start", "live/stop", "shutdown"} {
		got := exchange(t, srv, req(1, m, map[string]any{}))
		if rpc, code := errorOf(t, got[0]); rpc != -32000 || code != "not_initialized" {
			t.Errorf("%s: %s", m, got[0])
		}
	}
	// An unknown method is unknown whether or not the client has said hello.
	got := exchange(t, srv, req(1, "no/such", map[string]any{}))
	if rpc, _ := errorOf(t, got[0]); rpc != -32601 {
		t.Errorf("got %s", got[0])
	}
	got = exchange(t, srv, initReq(1, nil), req(2, "no/such", nil))
	if rpc, _ := errorOf(t, got[1]); rpc != -32601 {
		t.Errorf("got %s", got[1])
	}
}

func TestTapesList(t *testing.T) {
	root := workspace(t, map[string][]string{
		"20261001-1000-aaaa": smallTape("2026-10-01T10:00:00.000+09:00"),
		"20261002-1000-bbbb": smallTape("2026-10-02T10:00:00.000+09:00"),
		"20261003-1000-cccc": { // only a snapshot: nothing that was done, so not listed
			`{"v":1,"type":"header","session":"c","startedAt":"2026-10-03T10:00:00.000+09:00"}`,
			`{"v":1,"seq":1,"type":"snapshot","file":"a.go","text":"x"}`,
		},
		"20261004-1000-dddd": {}, // empty
		"20261005-1000-eeee": {`not json`, `{"v":1,"seq":1,"type":"foo"}`},
		"20261006-1000-ffff": { // no header
			`{"v":1,"seq":1,"type":"select","file":"b.go","startLine":1,"endLine":1,"why":null,"selection":null}`,
		},
	}, nil)
	// Things in the tapes directory that are not tapes.
	if err := os.WriteFile(filepath.Join(root, ".srwr", "tapes", "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".srwr", "tapes", "dir.tape.jsonl"), 0o750); err != nil {
		t.Fatal(err)
	}
	mod := time.Date(2026, 10, 1, 17, 6, 21, 26_000_000, time.FixedZone("JST", 9*3600))
	if err := os.Chtimes(filepath.Join(root, ".srwr", "tapes", tape.FileName("20261002-1000-bbbb")), mod, mod); err != nil {
		t.Fatal(err)
	}

	got := exchange(t, &Server{Root: root}, initReq(1, nil), req(2, "tapes/list", map[string]any{}))
	var list struct {
		Tapes []struct {
			TapeID    string
			StartedAt string
			UpdatedAt string
			Ops       int
			Files     []string
		}
	}
	resultOf(t, got[1], &list)
	var ids []string
	for _, ti := range list.Tapes {
		ids = append(ids, ti.TapeID)
	}
	if want := []string{"20261006-1000-ffff", "20261002-1000-bbbb", "20261001-1000-aaaa"}; !slices.Equal(ids, want) {
		t.Fatalf("tapes = %v, want %v (newest first; only tapes with operations)", ids, want)
	}
	b := list.Tapes[1]
	if b.StartedAt != "2026-10-02T10:00:00.000+09:00" || b.Ops != 2 || !slices.Equal(b.Files, []string{"a.go"}) ||
		b.UpdatedAt != "2026-10-01T17:06:21.026+09:00" {
		t.Errorf("entry = %+v", b)
	}
	if f := list.Tapes[0]; f.StartedAt != "" || f.Ops != 1 || !slices.Equal(f.Files, []string{"b.go"}) {
		t.Errorf("a tape without a header: %+v", f)
	}
}

func TestTapesListWithoutATapesDirectory(t *testing.T) {
	got := exchange(t, &Server{Root: t.TempDir()}, initReq(1, nil), req(2, "tapes/list", map[string]any{}))
	if want := `{"jsonrpc":"2.0","id":2,"result":{"tapes":[]}}`; got[1] != want {
		t.Errorf("got %s", got[1])
	}
}

func TestTapesListSeesAChangedTape(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": smallTape("2026-10-01T10:00:00.000+09:00")}, nil)
	srv := &Server{Root: root}
	// One connection; the tape grows between two lists.
	in, out := pipe(t, srv)
	in.send(initReq(1, nil))
	out.next()
	in.send(req(2, "tapes/list", nil))
	var first struct{ Tapes []struct{ Ops int } }
	resultOf(t, out.next(), &first)

	f, err := os.OpenFile(filepath.Join(root, ".srwr", "tapes", tape.FileName("20261001-1000-aaaa")), os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // a path in a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"v":1,"seq":4,"type":"select","file":"a.go","startLine":1,"endLine":1,"why":"x","selection":null}` + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	in.send(req(3, "tapes/list", nil))
	var second struct{ Tapes []struct{ Ops int } }
	resultOf(t, out.next(), &second)
	if first.Tapes[0].Ops != 2 || second.Tapes[0].Ops != 3 {
		t.Errorf("ops = %d then %d, want 2 then 3", first.Tapes[0].Ops, second.Tapes[0].Ops)
	}
	in.close()
	out.wait()
}

func TestTapeOpen(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": smallTape("2026-10-01T10:00:00.000+09:00")}, map[string]string{"a.go": "1\ntwo\n3\n"})
	srv := &Server{Root: root}

	t.Run("without text", func(t *testing.T) {
		got := exchange(t, srv, initReq(1, nil), req(2, "tape/open", map[string]any{"tapeId": "20261001-1000-aaaa"}))
		want := `{"jsonrpc":"2.0","id":2,"result":{"frames":[` +
			`{"index":0,"kind":"select","seq":2,"ts":1790841601000,"file":"a.go","range":{"start":2,"end":2},"why":"見る","selection":"sel_A","from":null,"parent":null},` +
			`{"index":1,"kind":"replace","seq":3,"ts":1790841602000,"file":"a.go","range":{"start":2,"end":2},"oldRange":{"start":2,"end":2},"why":"変える","selection":"sel_B","from":"sel_A","parent":0}` +
			`],"tapeId":"20261001-1000-aaaa"}}`
		if got[1] != want {
			t.Errorf("got  %s\nwant %s", got[1], want)
		}
	})
	t.Run("with text", func(t *testing.T) {
		got := exchange(t, srv, initReq(1, nil), req(2, "tape/open", map[string]any{"tapeId": "20261001-1000-aaaa", "withText": true}))
		if !strings.Contains(got[1], `"parent":null,"before":"1\n2\n3\n","after":"1\n2\n3\n"}`) ||
			!strings.Contains(got[1], `"parent":0,"before":"1\n2\n3\n","after":"1\ntwo\n3\n"}`) {
			t.Errorf("got %s", got[1])
		}
	})
	t.Run("the last diff", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("1\ntwo\n3\n4\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.WriteFile(filepath.Join(root, "a.go"), []byte("1\ntwo\n3\n"), 0o600) }()

		var opened struct {
			Frames []struct {
				Kind          string
				File          string
				Seq           int
				Before, After string
				Range         struct{ Start, End int }
			}
		}
		got := exchange(t, srv, initReq(1, nil), req(2, "tape/open", map[string]any{"tapeId": "20261001-1000-aaaa", "withText": true}))
		resultOf(t, got[1], &opened)
		if len(opened.Frames) != 3 || opened.Frames[2].Kind != "final" {
			t.Fatalf("frames = %+v", opened.Frames)
		}
		if f := opened.Frames[2]; f.Before != "1\ntwo\n3\n" || f.After != "1\ntwo\n3\n4\n" || f.Seq != 3 || f.Range.End != 4 {
			t.Errorf("final = %+v", f)
		}

		// Not wanted: no final.
		got = exchange(t, srv, initReq(1, map[string]any{"diffFrames": false}), req(2, "tape/open", map[string]any{"tapeId": "20261001-1000-aaaa"}))
		resultOf(t, got[1], &opened)
		if len(opened.Frames) != 2 {
			t.Errorf("with diffFrames false: %d frames, want 2", len(opened.Frames))
		}
	})
	t.Run("opened again, it is read again", func(t *testing.T) {
		in, out := pipe(t, srv)
		in.send(initReq(1, nil))
		out.next()
		in.send(req(2, "tape/open", map[string]any{"tapeId": "20261001-1000-aaaa"}))
		var a struct{ Frames []struct{} }
		resultOf(t, out.next(), &a)

		f, _ := os.OpenFile(filepath.Join(root, ".srwr", "tapes", tape.FileName("20261001-1000-aaaa")), os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // a path in a temporary directory
		_, _ = f.WriteString(`{"v":1,"seq":4,"type":"select","file":"a.go","startLine":1,"endLine":1,"why":"x","selection":null}` + "\n")
		_ = f.Close()
		defer writeTape(t, root, "20261001-1000-aaaa", strings.Join(smallTape("2026-10-01T10:00:00.000+09:00"), "\n")+"\n")

		in.send(req(3, "tape/open", map[string]any{"tapeId": "20261001-1000-aaaa"}))
		var b struct{ Frames []struct{} }
		resultOf(t, out.next(), &b)
		if len(a.Frames) != 2 || len(b.Frames) != 3 {
			t.Errorf("%d then %d frames, want 2 then 3", len(a.Frames), len(b.Frames))
		}
		in.close()
		out.wait()
	})
}

func TestTapeErrors(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": smallTape("")}, nil)
	srv := &Server{Root: root}
	tests := []struct {
		name   string
		method string
		params any
		rpc    int
		code   string
	}{
		{"open: no such tape", "tape/open", map[string]any{"tapeId": "nothing"}, -32000, "tape_not_found"},
		{"open: slash", "tape/open", map[string]any{"tapeId": "a/b"}, -32602, "invalid_params"},
		{"open: parent directory", "tape/open", map[string]any{"tapeId": "../20261001-1000-aaaa"}, -32602, "invalid_params"},
		{"open: dots", "tape/open", map[string]any{"tapeId": ".."}, -32602, "invalid_params"},
		{"open: absolute", "tape/open", map[string]any{"tapeId": "/etc/passwd"}, -32602, "invalid_params"},
		{"open: backslash", "tape/open", map[string]any{"tapeId": `..\x`}, -32602, "invalid_params"},
		{"open: empty", "tape/open", map[string]any{"tapeId": ""}, -32602, "invalid_params"},
		{"open: missing", "tape/open", map[string]any{}, -32602, "invalid_params"},
		{"open: wrong type", "tape/open", map[string]any{"tapeId": 5}, -32602, "invalid_params"},
		{"open: params not an object", "tape/open", []int{1}, -32602, "invalid_params"},
		{"state: not opened", "frame/state", map[string]any{"tapeId": "20261001-1000-aaaa", "index": 0}, -32000, "tape_not_found"},
		{"close: not opened", "tape/close", map[string]any{"tapeId": "20261001-1000-aaaa"}, -32000, "tape_not_found"},
		{"close: bad id", "tape/close", map[string]any{"tapeId": "../x"}, -32602, "invalid_params"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exchange(t, srv, initReq(1, nil), req(2, tt.method, tt.params))
			if rpc, code := errorOf(t, got[1]); rpc != tt.rpc || code != tt.code {
				t.Errorf("got %s", got[1])
			}
		})
	}

	if runtime.GOOS != "windows" {
		t.Run("a tape that cannot be read", func(t *testing.T) {
			p := filepath.Join(root, ".srwr", "tapes", tape.FileName("20261009-1000-zzzz"))
			if err := os.WriteFile(p, []byte("x"), 0o000); err != nil {
				t.Fatal(err)
			}
			// Running as root, the mode makes no difference.
			if f, err := os.Open(p); err == nil { //nolint:gosec // a path in a temporary directory
				_ = f.Close()
				t.Skip("files can be read despite their mode")
			}
			got := exchange(t, srv, initReq(1, nil), req(2, "tape/open", map[string]any{"tapeId": "20261009-1000-zzzz"}), req(3, "tapes/list", nil))
			if rpc, code := errorOf(t, got[1]); rpc != -32000 || code != "tape_unreadable" {
				t.Errorf("got %s", got[1])
			}
			if strings.Contains(got[2], "zzzz") {
				t.Errorf("an unreadable tape is listed: %s", got[2])
			}
		})
	}
}

func TestFrameState(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": smallTape("")}, nil)
	srv := &Server{Root: root}
	open := req(2, "tape/open", map[string]any{"tapeId": "20261001-1000-aaaa"})
	state := func(id int, params map[string]any) string {
		params["tapeId"] = "20261001-1000-aaaa"
		return req(id, "frame/state", params)
	}
	got := exchange(t, srv, initReq(1, nil), open,
		state(3, map[string]any{"index": 1}),
		state(4, map[string]any{"index": -1}),
		state(5, map[string]any{"index": -1, "file": "a.go"}),
		state(6, map[string]any{"index": 0, "file": "other.go"}),
		state(7, map[string]any{"index": 3}),
		state(8, map[string]any{"index": -2}),
		state(9, map[string]any{}),
		req(10, "tape/close", map[string]any{"tapeId": "20261001-1000-aaaa"}),
		state(11, map[string]any{"index": 0}),
	)
	want := map[int]string{
		3:  `{"jsonrpc":"2.0","id":3,"result":{"after":"1\ntwo\n3\n","before":"1\n2\n3\n","content":"1\ntwo\n3\n"}}`,
		4:  `{"jsonrpc":"2.0","id":4,"result":{"after":"","before":"","content":null}}`,
		5:  `{"jsonrpc":"2.0","id":5,"result":{"after":"","before":"","content":"1\n2\n3\n"}}`,
		6:  `{"jsonrpc":"2.0","id":6,"result":{"after":"1\n2\n3\n","before":"1\n2\n3\n","content":null}}`,
		10: `{"jsonrpc":"2.0","id":10,"result":{}}`,
	}
	for i, line := range got[2:] {
		id := i + 3
		if w, ok := want[id]; ok && line != w {
			t.Errorf("id %d:\n got %s\nwant %s", id, line, w)
		}
	}
	for id, wantCode := range map[int]string{7: "invalid_params", 8: "invalid_params", 9: "invalid_params", 11: "tape_not_found"} {
		if _, code := errorOf(t, got[id-1]); code != wantCode {
			t.Errorf("id %d: %s", id, got[id-1])
		}
	}
}

func TestShutdownEndsTheConnection(t *testing.T) {
	srv := &Server{Root: workspace(t, nil, nil)}
	got := exchange(t, srv, initReq(1, nil), req(2, "shutdown", nil), req(3, "tapes/list", nil))
	if len(got) != 2 || got[1] != `{"jsonrpc":"2.0","id":2,"result":{}}` {
		t.Errorf("got %q: nothing may be answered after shutdown", got)
	}
}

// The tapes are shared and anyone may have written them. A path in a tape must not lead the last
// diff to a file outside the workspace.
func TestLastDiffDoesNotTrustPaths(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("TOP SECRET\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := workspace(t, nil, map[string]string{"inside.txt": "inside\n"})
	rel, err := filepath.Rel(root, filepath.Join(outside, "secret.txt"))
	if err != nil {
		t.Skip("no relative path to the outside on this platform")
	}

	paths := []string{
		"../secret.txt",
		filepath.ToSlash(rel),
		filepath.Join(outside, "secret.txt"),
		"/etc/passwd",
		"./../../etc/passwd",
		"sub/../../secret.txt",
		"a\x00b",
		"",
		`\\server\share\x`,
		"C:/Windows/win.ini",
		"..",
		".",
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "link.txt")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "linkdir")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Dir(root), filepath.Join(root, "up")); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, "link.txt", "linkdir/secret.txt", "up/"+filepath.Base(outside)+"/secret.txt")
	}

	var lines []string
	lines = append(lines, `{"v":1,"type":"header","session":"x"}`)
	seq := 0
	for _, p := range paths {
		seq++
		enc := strings.ReplaceAll(strings.ReplaceAll(p, `\`, `\\`), "\x00", `\u0000`)
		lines = append(lines,
			`{"v":1,"seq":`+itoa(seq)+`,"type":"snapshot","file":"`+enc+`","text":"was\n"}`,
			`{"v":1,"seq":`+itoa(seq+100)+`,"type":"select","file":"`+enc+`","startLine":1,"endLine":1,"why":"w","selection":null}`)
	}
	lines = append(lines,
		`{"v":1,"seq":900,"type":"snapshot","file":"inside.txt","text":"before\n"}`,
		`{"v":1,"seq":901,"type":"select","file":"inside.txt","startLine":1,"endLine":1,"why":"w","selection":null}`)
	writeTape(t, root, "20261001-1000-aaaa", strings.Join(lines, "\n")+"\n")

	got := exchange(t, &Server{Root: root}, initReq(1, nil), req(2, "tape/open", map[string]any{"tapeId": "20261001-1000-aaaa", "withText": true}))
	if strings.Contains(got[1], "TOP SECRET") || strings.Contains(got[1], "root:") {
		t.Fatalf("a file outside the workspace was read: %s", got[1])
	}
	var opened struct {
		Frames []struct {
			Kind, File    string
			Before, After string
			Deleted       bool
		}
	}
	resultOf(t, got[1], &opened)
	finals := map[string]struct {
		before, after string
		deleted       bool
	}{}
	for _, f := range opened.Frames {
		if f.Kind == "final" {
			finals[f.File] = struct {
				before, after string
				deleted       bool
			}{f.Before, f.After, f.Deleted}
		}
	}
	for _, p := range paths {
		if f, ok := finals[p]; !ok || !f.deleted || f.after != "" {
			t.Errorf("%q: final = %+v (present %v), want a file that does not exist", p, f, ok)
		}
	}
	if f := finals["inside.txt"]; f.before != "before\n" || f.after != "inside\n" || f.deleted {
		t.Errorf("a path inside the workspace is read as usual: %+v", f)
	}
}

func TestLastDiffMakesTextValid(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": {
		`{"v":1,"seq":1,"type":"snapshot","file":"a.bin","text":"x\n"}`,
		`{"v":1,"seq":2,"type":"select","file":"a.bin","startLine":1,"endLine":1,"why":"w","selection":null}`,
	}}, nil)
	if err := os.WriteFile(filepath.Join(root, "a.bin"), []byte("caf\xe9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := exchange(t, &Server{Root: root}, initReq(1, nil), req(2, "tape/open", map[string]any{"tapeId": "20261001-1000-aaaa", "withText": true}))
	if !strings.Contains(got[1], `"after":"caf\ufffd\n"`) && !strings.Contains(got[1], "caf\ufffd") {
		t.Errorf("got %s", got[1])
	}
}
