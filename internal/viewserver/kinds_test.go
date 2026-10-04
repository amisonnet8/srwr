package viewserver

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// A tape with a failure and a replace in it: select a.go 2, failure, replace a.go 2 -> "two", failure about a.go.
func tapeWithFailures() []string {
	return []string{
		`{"v":1,"type":"header","session":"s","startedAt":"2026-10-01T08:00:00.000Z"}`,
		`{"v":1,"seq":1,"ts":"2026-10-01T08:00:00.000Z","type":"snapshot","file":"a.go","text":"1\n2\n3\n"}`,
		`{"v":1,"seq":2,"ts":"2026-10-01T08:00:01.000Z","type":"select","file":"a.go","startLine":2,"endLine":2,"why":"look","selection":"sel_A"}`,
		`{"v":1,"seq":3,"ts":"2026-10-01T08:00:02.000Z","type":"failure","tool":"select","file":null,"startLine":9,"endLine":9,"selection":null,"why":"abs","code":"invalid_range","message":"The path is absolute. Give a path relative to the workspace"}`,
		`{"v":1,"seq":4,"ts":"2026-10-01T08:00:03.000Z","type":"replace","file":"a.go","from":"sel_A","startLine":2,"endLine":2,"oldText":"2","newText":"two","newStartLine":2,"newEndLine":2,"selection":"sel_B","why":"change"}`,
		`{"v":1,"seq":5,"ts":"2026-10-01T08:00:04.000Z","type":"failure","tool":"replace","file":"a.go","startLine":null,"endLine":null,"selection":"sel_A","why":"again","code":"selection_stale","message":"stale"}`,
	}
}

type openResult struct {
	Frames []struct {
		Index   int
		Kind    string
		File    string
		Tool    string
		Code    string
		Message string
	}
	Hidden map[string]int
}

func openWith(t *testing.T, root string, kinds any) (openResult, []string) {
	t.Helper()
	params := map[string]any{"tapeId": "20261001-1000-aaaa"}
	if kinds != nil {
		params["kinds"] = kinds
	}
	got := exchange(t, &Server{Root: root}, initReq(1, nil), req(2, "tape/open", params))
	var r openResult
	if strings.Contains(got[1], `"error"`) {
		return r, got
	}
	resultOf(t, got[1], &r)
	return r, got
}

func kindsOf(r openResult) string {
	var k []string
	for _, f := range r.Frames {
		k = append(k, f.Kind)
	}
	return strings.Join(k, " ")
}

func TestTapeOpenKinds(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": tapeWithFailures()}, map[string]string{"a.go": "1\ntwo\n3\n"})

	// Left out: everything but failure, and nothing is said to be hidden (the count would be an item of its own).
	r, got := openWith(t, root, nil)
	if kindsOf(r) != "select replace" || len(r.Hidden) != 1 || r.Hidden["failure"] != 2 {
		t.Errorf("default: %s hidden %v\n%s", kindsOf(r), r.Hidden, got[1])
	}
	// failure asked for: the frames are in the order recorded, and numbered from 0 over what is sent.
	r, _ = openWith(t, root, []string{"select", "replace", "external", "failure"})
	if kindsOf(r) != "select failure replace failure" || len(r.Hidden) != 0 {
		t.Fatalf("all: %s hidden %v", kindsOf(r), r.Hidden)
	}
	for i, f := range r.Frames {
		if f.Index != i {
			t.Errorf("frame %d has index %d", i, f.Index)
		}
	}
	if f := r.Frames[1]; f.File != "" || f.Tool != "select" || f.Code != "invalid_range" || !strings.Contains(f.Message, "absolute") {
		t.Errorf("the first failure = %+v", f)
	}
	if f := r.Frames[3]; f.File != "a.go" || f.Tool != "replace" || f.Code != "selection_stale" {
		t.Errorf("the second failure = %+v", f)
	}
	// Only replace.
	r, _ = openWith(t, root, []string{"replace"})
	if kindsOf(r) != "replace" || r.Hidden["select"] != 1 || r.Hidden["failure"] != 2 {
		t.Errorf("replace only: %s hidden %v", kindsOf(r), r.Hidden)
	}
	// Nothing.
	r, _ = openWith(t, root, []string{})
	if len(r.Frames) != 0 || r.Hidden["select"] != 1 || r.Hidden["replace"] != 1 || r.Hidden["failure"] != 2 {
		t.Errorf("none: %s hidden %v", kindsOf(r), r.Hidden)
	}
	// A name that is not a kind.
	_, got = openWith(t, root, []string{"select", "final"})
	if rpc, code := errorOf(t, got[1]); rpc != -32602 || code != "invalid_params" {
		t.Errorf("unknown kind: %s", got[1])
	}
}

// frame/state takes the index among the frames that were sent, and the text of a file counts the frames that were left out too.
func TestFrameStateWithKinds(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": tapeWithFailures()}, map[string]string{"a.go": "1\ntwo\n3\n"})
	type state struct {
		Before, After string
		Content       *string
	}
	ask := func(kinds []string, index int) state {
		t.Helper()
		got := exchange(t, &Server{Root: root}, initReq(1, nil),
			req(2, "tape/open", map[string]any{"tapeId": "20261001-1000-aaaa", "kinds": kinds}),
			req(3, "frame/state", map[string]any{"tapeId": "20261001-1000-aaaa", "index": index, "file": "a.go"}))
		var s state
		resultOf(t, got[2], &s)
		return s
	}
	// Only failure is shown: its frame (index 0) is the first failure, which came between the select and the replace.
	if s := ask([]string{"failure"}, 0); s.Content == nil || *s.Content != "1\n2\n3\n" || s.Before != "" || s.After != "" {
		t.Errorf("first failure: %+v", s)
	}
	// The replace is hidden, and the second failure (index 1) still sees a.go as the replace left it.
	if s := ask([]string{"failure"}, 1); s.Content == nil || *s.Content != "1\ntwo\n3\n" {
		t.Errorf("second failure: %+v", s)
	}
	// With select and replace shown, index 1 is the replace.
	if s := ask([]string{"select", "replace"}, 1); s.Before != "1\n2\n3\n" || s.After != "1\ntwo\n3\n" {
		t.Errorf("replace: %+v", s)
	}
	// Past what was sent.
	got := exchange(t, &Server{Root: root}, initReq(1, nil),
		req(2, "tape/open", map[string]any{"tapeId": "20261001-1000-aaaa"}),
		req(3, "frame/state", map[string]any{"tapeId": "20261001-1000-aaaa", "index": 2}))
	if rpc, code := errorOf(t, got[2]); rpc != -32602 || code != "invalid_params" {
		t.Errorf("index past the end: %s", got[2])
	}
}

func failureLine(seq int) string {
	return `{"v":1,"seq":` + itoa(seq) + `,"ts":"2026-10-01T08:00:00.000Z","type":"failure","tool":"select","file":null,"startLine":1,"endLine":1,"selection":null,"why":"w","code":"invalid_range","message":"m"}`
}

func TestLiveKinds(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": append(baseTape(), failureLine(3))}, nil)
	in, out := pipe(t, &Server{Root: root, PollInterval: fast})
	in.send(initReq(1, nil))
	out.next()

	// failure is not asked for: it is counted as hidden, and a failure that arrives later is not sent as a frame.
	in.send(req(2, "live/start", map[string]any{}))
	var res struct {
		Frames []struct{ Index int }
		Hidden map[string]int
	}
	resultOf(t, out.next(), &res)
	if len(res.Frames) != 1 || res.Hidden["failure"] != 1 {
		t.Fatalf("live/start = %+v", res)
	}
	appendTo(t, root, "20261001-1000-aaaa", failureLine(4)+"\n")
	var h struct {
		Method string
		Params struct {
			TapeID string
			Hidden map[string]int
		}
	}
	if err := json.Unmarshal([]byte(out.next()), &h); err != nil || h.Method != "live/hidden" || h.Params.Hidden["failure"] != 2 {
		t.Fatalf("live/hidden = %+v (%v)", h, err)
	}
	// A frame that is shown is numbered after the ones sent, not after all of them.
	appendTo(t, root, "20261001-1000-aaaa", selectLine(5)+"\n")
	if f := parseLive(t, out.next()); f.Params.Frame.Seq != 5 || f.Params.Frame.Index != 1 {
		t.Errorf("frame = %+v, want seq 5 and index 1", f)
	}
	in.close()
	out.wait()
}

func TestLiveWithFailureShown(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": baseTape()}, nil)
	in, out := pipe(t, &Server{Root: root, PollInterval: fast})
	in.send(initReq(1, nil))
	out.next()
	in.send(req(2, "live/start", map[string]any{"kinds": []string{"select", "replace", "external", "failure"}}))
	out.next()
	appendTo(t, root, "20261001-1000-aaaa", failureLine(3)+"\n")
	line := out.next()
	var f struct {
		Method string
		Params struct {
			Frame struct {
				Index int
				Kind  string
				Code  string
			}
		}
	}
	if err := json.Unmarshal([]byte(line), &f); err != nil || f.Method != "live/frame" || f.Params.Frame.Kind != "failure" || f.Params.Frame.Code != "invalid_range" || f.Params.Frame.Index != 1 {
		t.Errorf("notification = %s", line)
	}
	if l, ok := out.within(60 * time.Millisecond); ok {
		t.Errorf("an extra notification: %s", l)
	}
	in.close()
	out.wait()
}
