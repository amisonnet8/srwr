package viewserver

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amisonnet8/srwr/internal/tape"
)

const fast = 2 * time.Millisecond

func selectLine(seq int) string {
	return fmt.Sprintf(`{"v":1,"seq":%d,"ts":"2026-10-01T08:00:00.000Z","type":"select","file":"a.go","startLine":1,"endLine":1,"why":"n%d","selection":null}`, seq, seq)
}

func baseTape() []string {
	return []string{
		`{"v":1,"type":"header","session":"s","startedAt":"2026-10-01T08:00:00.000Z"}`,
		`{"v":1,"seq":1,"ts":"2026-10-01T08:00:00.000Z","type":"snapshot","file":"a.go","text":"1\n2\n"}`,
		selectLine(2),
	}
}

func appendTo(t *testing.T, root, id, text string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(root, ".srwr", "tapes", tape.FileName(id)), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600) //nolint:gosec // a path in a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

type liveFrame struct {
	Method string
	Params struct {
		TapeID string
		Frame  struct {
			Index  int
			Seq    int
			Why    *string
			Before *string
			After  *string
		}
	}
}

func parseLive(t *testing.T, line string) liveFrame {
	t.Helper()
	var f liveFrame
	if err := json.Unmarshal([]byte(line), &f); err != nil || f.Method != "live/frame" {
		t.Fatalf("not a live/frame notification: %s (%v)", line, err)
	}
	return f
}

type liveStarted struct {
	TapeID *string
	Frames []struct{ Index int }
}

func startLive(t *testing.T, in pipeIn, out pipeOut, id int, params map[string]any) liveStarted {
	t.Helper()
	in.send(req(id, "live/start", params))
	var res liveStarted
	resultOf(t, out.next(), &res)
	return res
}

func TestLiveFollowsTheTape(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": baseTape()}, nil)
	in, out := pipe(t, &Server{Root: root, PollInterval: fast})
	in.send(initReq(1, nil))
	out.next()

	res := startLive(t, in, out, 2, map[string]any{"withText": true})
	if res.TapeID == nil || *res.TapeID != "20261001-1000-aaaa" || len(res.Frames) != 1 {
		t.Fatalf("live/start = %+v", res)
	}

	appendTo(t, root, "20261001-1000-aaaa", selectLine(3)+"\n"+selectLine(4)+"\n")
	for want := 1; want <= 2; want++ {
		f := parseLive(t, out.next())
		if f.Params.TapeID != "20261001-1000-aaaa" || f.Params.Frame.Index != want || f.Params.Frame.Seq != want+2 {
			t.Errorf("notification %d = %+v", want, f)
		}
		if f.Params.Frame.Before == nil || *f.Params.Frame.Before != "1\n2\n" {
			t.Errorf("withText: before = %v", f.Params.Frame.Before)
		}
	}
	in.close()
	out.wait()
}

func TestLiveWithoutText(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": baseTape()}, nil)
	in, out := pipe(t, &Server{Root: root, PollInterval: fast})
	in.send(initReq(1, nil))
	out.next()
	startLive(t, in, out, 2, nil)
	appendTo(t, root, "20261001-1000-aaaa", selectLine(3)+"\n")
	line := out.next()
	if strings.Contains(line, `"before"`) || strings.Contains(line, `"after"`) {
		t.Errorf("text was not asked for: %s", line)
	}
	parseLive(t, line)
	in.close()
	out.wait()
}

func TestLiveHoldsBackAnUnfinishedLine(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": baseTape()}, nil)
	in, out := pipe(t, &Server{Root: root, PollInterval: fast})
	in.send(initReq(1, nil))
	out.next()
	startLive(t, in, out, 2, nil)

	line := selectLine(3) + "\n"
	appendTo(t, root, "20261001-1000-aaaa", line[:40])
	if l, ok := out.within(100 * time.Millisecond); ok {
		t.Fatalf("a frame came from half a line: %s", l)
	}
	appendTo(t, root, "20261001-1000-aaaa", line[40:])
	if f := parseLive(t, out.next()); f.Params.Frame.Seq != 3 || f.Params.Frame.Index != 1 {
		t.Errorf("frame = %+v", f)
	}
	if l, ok := out.within(50 * time.Millisecond); ok {
		t.Errorf("a second notification for the same line: %s", l)
	}
	in.close()
	out.wait()
}

// The notifications start after the response to live/start: never one before it, none lost, none twice.
// The tape is long when the view starts, so that making the response takes a while: a watcher
// that began before the response was written would have time to send something first.
func TestLiveNeverSendsBeforeTheResponse(t *testing.T) {
	const prefill, total = 20000, 300
	var long strings.Builder
	long.WriteString(strings.Join(baseTape(), "\n") + "\n")
	for seq := 3; seq < 3+prefill; seq++ {
		long.WriteString(selectLine(seq) + "\n")
	}

	for round := range 3 {
		root := workspace(t, nil, nil)
		writeTape(t, root, "20261001-1000-aaaa", long.String())
		in, out := pipe(t, &Server{Root: root, PollInterval: time.Millisecond})
		in.send(initReq(1, nil))
		out.next()

		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seq := 3 + prefill; seq < 3+prefill+total; seq++ {
				appendTo(t, root, "20261001-1000-aaaa", selectLine(seq)+"\n")
				select {
				case <-stop:
					return
				case <-time.After(100 * time.Microsecond):
				}
			}
		}()

		in.send(req(2, "live/start", map[string]any{"withText": true}))
		first := out.next()
		if strings.Contains(first, `"method"`) {
			t.Fatalf("round %d: a notification came before the response", round)
		}
		var res liveStarted
		resultOf(t, first, &res)
		if len(res.Frames) < 1+prefill {
			t.Fatalf("round %d: %d frames in the response, want at least %d", round, len(res.Frames), 1+prefill)
		}

		next := len(res.Frames)
		for next < 1+prefill+total { // frame 0 is seq 2
			f := parseLive(t, out.next())
			if f.Params.Frame.Index != next {
				t.Fatalf("round %d: frame index %d, want %d", round, f.Params.Frame.Index, next)
			}
			next++
		}
		close(stop)
		wg.Wait()
		in.close()
		out.wait()
	}
}

func TestLiveSwitchesToANewerTape(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": baseTape()}, nil)
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(root, ".srwr", "tapes", tape.FileName("20261001-1000-aaaa")), old, old); err != nil {
		t.Fatal(err)
	}
	in, out := pipe(t, &Server{Root: root, PollInterval: fast})
	in.send(initReq(1, nil))
	out.next()
	res := startLive(t, in, out, 2, map[string]any{"withText": true})
	if res.TapeID == nil || *res.TapeID != "20261001-1000-aaaa" {
		t.Fatalf("live/start = %+v", res)
	}

	// A new tape begins: its frames come from its first.
	appendTo(t, root, "20261002-1000-bbbb", strings.Join(baseTape(), "\n")+"\n"+selectLine(3)+"\n")
	for want := range 2 {
		f := parseLive(t, out.next())
		if f.Params.TapeID != "20261002-1000-bbbb" || f.Params.Frame.Index != want {
			t.Fatalf("notification = %+v, want frame %d of the new tape", f, want)
		}
	}
	// And it is followed from then on.
	appendTo(t, root, "20261002-1000-bbbb", selectLine(4)+"\n")
	if f := parseLive(t, out.next()); f.Params.TapeID != "20261002-1000-bbbb" || f.Params.Frame.Index != 2 {
		t.Errorf("notification = %+v", f)
	}
	// Writing to the old tape does not take the view back.
	if l, ok := out.within(50 * time.Millisecond); ok {
		t.Errorf("unexpected %s", l)
	}
	in.close()
	out.wait()
}

func TestLiveBeforeAnyTape(t *testing.T) {
	root := workspace(t, nil, nil)
	in, out := pipe(t, &Server{Root: root, PollInterval: fast})
	in.send(initReq(1, nil))
	out.next()

	in.send(req(2, "live/start", nil))
	if line := out.next(); line != `{"jsonrpc":"2.0","id":2,"result":{"tapeId":null,"frames":[]}}` {
		t.Errorf("got %s", line)
	}
	appendTo(t, root, "20261001-1000-aaaa", strings.Join(baseTape(), "\n")+"\n")
	if f := parseLive(t, out.next()); f.Params.TapeID != "20261001-1000-aaaa" || f.Params.Frame.Index != 0 {
		t.Errorf("notification = %+v", f)
	}
	in.close()
	out.wait()
}

func TestLiveStop(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": baseTape()}, nil)
	in, out := pipe(t, &Server{Root: root, PollInterval: fast})
	in.send(initReq(1, nil))
	out.next()
	startLive(t, in, out, 2, nil)

	appendTo(t, root, "20261001-1000-aaaa", selectLine(3)+"\n")
	parseLive(t, out.next())

	in.send(req(3, "live/stop", nil))
	for {
		l := out.next()
		if strings.Contains(l, `"id":3`) {
			if l != `{"jsonrpc":"2.0","id":3,"result":{}}` {
				t.Errorf("got %s", l)
			}
			break
		}
		parseLive(t, l) // a notification written before the response is fine
	}
	appendTo(t, root, "20261001-1000-aaaa", selectLine(4)+"\n")
	if l, ok := out.within(100 * time.Millisecond); ok {
		t.Errorf("a notification after live/stop: %s", l)
	}
	// The connection goes on.
	in.send(req(4, "tapes/list", nil))
	if l := out.next(); !strings.Contains(l, `"id":4`) {
		t.Errorf("got %s", l)
	}
	// Stopping again is fine.
	in.send(req(5, "live/stop", nil))
	if l := out.next(); !strings.Contains(l, `"id":5`) {
		t.Errorf("got %s", l)
	}
	in.close()
	out.wait()
}

func TestLiveStartedAgainReplacesTheFirst(t *testing.T) {
	root := workspace(t, map[string][]string{"20261001-1000-aaaa": baseTape()}, nil)
	in, out := pipe(t, &Server{Root: root, PollInterval: fast})
	in.send(initReq(1, nil))
	out.next()
	startLive(t, in, out, 2, nil)
	res := startLive(t, in, out, 3, nil)
	if len(res.Frames) != 1 {
		t.Fatalf("frames = %d", len(res.Frames))
	}
	appendTo(t, root, "20261001-1000-aaaa", selectLine(3)+"\n")
	parseLive(t, out.next())
	if l, ok := out.within(100 * time.Millisecond); ok {
		t.Errorf("the frame was sent twice: %s", l)
	}
	in.close()
	out.wait()
}

// countingWriter notes whether anything is written after the server has returned.
type countingWriter struct {
	w      io.Writer
	closed atomic.Bool
	late   atomic.Int32
}

func (c *countingWriter) Write(p []byte) (int, error) {
	if c.closed.Load() {
		c.late.Add(1)
	}
	return c.w.Write(p)
}

// When the connection ends, by shutdown or by the input ending, the watching stops: nothing is written afterwards.
func TestLiveStopsWhenTheConnectionEnds(t *testing.T) {
	for _, how := range []string{"input ends", "shutdown", "stop then input ends"} {
		t.Run(how, func(t *testing.T) {
			root := workspace(t, map[string][]string{"20261001-1000-aaaa": baseTape()}, nil)
			inR, inW := io.Pipe()
			cw := &countingWriter{w: io.Discard}
			done := make(chan error, 1)
			go func() {
				err := (&Server{Root: root, PollInterval: time.Millisecond}).Serve(inR, cw)
				cw.closed.Store(true)
				done <- err
			}()
			send := func(s string) {
				if _, err := io.WriteString(inW, s+"\n"); err != nil {
					t.Fatal(err)
				}
			}
			send(initReq(1, nil))
			send(req(2, "live/start", nil))

			// Keep the tape growing so that there is always something to send.
			quit := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				for seq := 3; ; seq++ {
					select {
					case <-quit:
						return
					case <-time.After(200 * time.Microsecond):
						appendTo(t, root, "20261001-1000-aaaa", selectLine(seq)+"\n")
					}
				}
			}()

			time.Sleep(30 * time.Millisecond) // let some frames flow; this is the only thing there is to wait for
			switch how {
			case "shutdown":
				send(req(3, "shutdown", nil))
			case "stop then input ends":
				send(req(3, "live/stop", nil))
				_ = inW.Close()
			default:
				_ = inW.Close()
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("Serve did not return")
			}
			time.Sleep(30 * time.Millisecond)
			close(quit)
			wg.Wait()
			if n := cw.late.Load(); n != 0 {
				t.Errorf("%d writes after Serve returned", n)
			}
		})
	}
}
