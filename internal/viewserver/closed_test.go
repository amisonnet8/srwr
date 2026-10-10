package viewserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amisonnet8/srwr/internal/tape"
)

// closeTapes compresses the tapes of a workspace, as a session that ended does.
func closeTapes(t *testing.T, root string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if err := tape.Compress(filepath.Join(root, ".srwr", "tapes"), id); err != nil {
			t.Fatal(err)
		}
	}
}

// A closed tape (compressed) is shown exactly as it was when it was plain: the list, the frames and the state of a frame.
func TestAClosedTapeIsShownLikeAPlainOne(t *testing.T) {
	tapes := map[string][]string{
		"20261001-1000-aaaa": smallTape("2026-10-01T10:00:00.000Z"),
		"20261002-1000-bbbb": smallTape("2026-10-02T10:00:00.000Z"),
	}
	files := map[string]string{"a.go": "1\ntwo\n3\n"}
	plain := workspace(t, tapes, files)
	closed := workspace(t, tapes, files)
	mod := time.Date(2026, 10, 1, 17, 6, 21, 0, time.UTC)
	for _, root := range []string{plain, closed} {
		for id := range tapes {
			p := filepath.Join(root, ".srwr", "tapes", tape.FileName(id))
			if err := chtimes(p, mod); err != nil {
				t.Fatal(err)
			}
		}
	}
	closeTapes(t, closed, "20261001-1000-aaaa", "20261002-1000-bbbb")

	reqs := []string{
		initReq(1, nil),
		req(2, "tapes/list", map[string]any{}),
		req(3, "tape/open", map[string]any{"tapeId": "20261001-1000-aaaa", "withText": true}),
		req(4, "frame/state", map[string]any{"tapeId": "20261001-1000-aaaa", "index": 1}),
		req(5, "tape/open", map[string]any{"tapeId": "20261003-1000-zzzz"}),
	}
	want := exchange(t, &Server{Root: plain}, reqs...)
	got := exchange(t, &Server{Root: closed}, reqs...)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("response %d differs:\n closed %s\n plain  %s", i+1, got[i], want[i])
		}
	}
	if !strings.Contains(got[1], "20261002-1000-bbbb") || !strings.Contains(got[2], `"after":"1\ntwo\n3\n"`) {
		t.Errorf("the closed tapes are not shown: %s %s", got[1], got[2])
	}
}

// live/start follows the newest tape also when it is a closed one, and a tape that is closed while it is followed loses no event
// that was written just before.
func TestLiveWithClosedTapes(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": baseTape()}, nil)
	in, out := pipe(t, &Server{Root: root, PollInterval: time.Hour}) // the tick is driven by the test below
	in.send(initReq(1, nil))
	out.next()
	closeTapes(t, root, "20261001-1000-aaaa")
	res := startLive(t, in, out, 2, map[string]any{})
	if res.TapeID == nil || *res.TapeID != "20261001-1000-aaaa" || len(res.Frames) != 1 {
		t.Fatalf("live/start on a closed tape = %+v", res)
	}
	in.close()
	out.wait()
}

func TestLiveReadsTheEndOfATapeThatWasClosed(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": baseTape()}, nil)
	in, out := pipe(t, &Server{Root: root, PollInterval: 50 * time.Millisecond})
	in.send(initReq(1, nil))
	out.next()
	res := startLive(t, in, out, 2, map[string]any{})
	if res.TapeID == nil || len(res.Frames) != 1 {
		t.Fatalf("live/start = %+v", res)
	}
	// Two events are written, and the session ends and the tape is compressed before the next look at it.
	appendTo(t, root, "20261001-1000-aaaa", selectLine(3)+"\n"+selectLine(4)+"\n")
	closeTapes(t, root, "20261001-1000-aaaa")
	for want := 1; want <= 2; want++ {
		f := parseLive(t, out.next())
		if f.Params.Frame.Index != want || f.Params.Frame.Seq != want+2 {
			t.Errorf("notification %d = %+v", want, f)
		}
	}
	in.close()
	out.wait()
}

func chtimes(path string, t time.Time) error { return os.Chtimes(path, t, t) }

// tapes/list says the title and why the AI gave with the session tool, and has neither for a tape without them.
func TestTapesListHasTheTitleOfASession(t *testing.T) {
	titled := smallTape("2026-10-02T10:00:00.000Z")
	titled[0] = `{"v":2,"type":"header","session":"s","startedAt":"2026-10-02T10:00:00.000Z","title":"docs first","why":"wording"}`
	root := workspace(t, map[string][]string{
		"20261001-1000-aaaa": smallTape("2026-10-01T10:00:00.000Z"),
		"20261002-1000-bbbb": titled,
	}, map[string]string{"a.go": "1\ntwo\n3\n"})
	got := exchange(t, &Server{Root: root}, initReq(1, nil), req(2, "tapes/list", map[string]any{}))
	var r struct {
		Result struct {
			Tapes []struct{ TapeID, Title, Why string }
		}
	}
	if err := json.Unmarshal([]byte(got[1]), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Result.Tapes) != 2 || r.Result.Tapes[0].TapeID != "20261002-1000-bbbb" || r.Result.Tapes[0].Title != "docs first" || r.Result.Tapes[0].Why != "wording" {
		t.Errorf("tapes = %+v", r.Result.Tapes)
	}
	if strings.Contains(got[1], `"title":""`) || strings.Count(got[1], `"title"`) != 1 {
		t.Errorf("a tape without a title has the key: %s", got[1])
	}
}
