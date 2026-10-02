package viewserver

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/amisonnet8/srwr/internal/jsonrpc"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/timeline"
)

// liveWatcher follows the newest tape and sends the frames that are added to it. It is one
// goroutine, started after the response to live/start has been written and stopped, and waited for,
// before the response to anything that ends the live view.
type liveWatcher struct {
	c        *conn
	withText bool

	// What has been read: the tape, how far, and what was made of it. Only the goroutine touches these once it runs.
	tapeID  string
	offset  int
	builder *timeline.Builder

	started bool
	quit    chan struct{}
	done    chan struct{}
	once    sync.Once
}

// liveStartResult is the response to live/start.
type liveStartResult struct {
	TapeID *string     `json:"tapeId"`
	Frames []wireFrame `json:"frames"`

	w *liveWatcher
}

// After starts watching, now that the client has the response: a notification must never come before it.
func (r liveStartResult) After() bool {
	r.w.start()
	return false
}

func (c *conn) liveStart(raw []byte) (any, *jsonrpc.Error) {
	var p struct {
		WithText bool `json:"withText"`
	}
	if err := decode(raw, &p); err != nil {
		return nil, err
	}
	c.stopLive()
	w := &liveWatcher{c: c, withText: p.WithText, builder: timeline.NewBuilder(), quit: make(chan struct{}), done: make(chan struct{})}
	c.live = w

	res := liveStartResult{Frames: []wireFrame{}, w: w}
	if id, ok := c.srv.newestTape(); ok {
		if data, err := os.ReadFile(filepath.Join(c.srv.tapesDir(), tape.FileName(id))); err == nil {
			parsed := tape.Parse(data)
			w.tapeID, w.offset, w.builder = id, parsed.Consumed, timeline.Build(parsed.Events)
			res.TapeID = &id
			res.Frames = wireAll(w.builder.Frames(), p.WithText)
		}
	}
	return res, nil
}

func (c *conn) stopLive() {
	if c.live != nil {
		c.live.stop()
		c.live = nil
	}
}

func (w *liveWatcher) start() {
	w.started = true
	go w.run()
}

// stop ends the goroutine and waits for it. After it returns nothing more is written.
func (w *liveWatcher) stop() {
	w.once.Do(func() { close(w.quit) })
	if w.started {
		<-w.done
	}
}

func (w *liveWatcher) run() {
	defer close(w.done)
	ticker := time.NewTicker(w.c.srv.poll())
	defer ticker.Stop()
	for {
		select {
		case <-w.quit:
			return
		case <-ticker.C:
			if err := w.tick(); err != nil {
				return
			}
		}
	}
}

// newestTape returns the tape that was written most recently.
func (s *Server) newestTape() (string, bool) {
	best, bestID := int64(0), ""
	for _, id := range s.tapeIDs() { // newest name first, so on a tie the later name wins
		st, err := os.Stat(filepath.Join(s.tapesDir(), tape.FileName(id)))
		if err != nil {
			continue
		}
		if m := st.ModTime().UnixNano(); bestID == "" || m > best {
			best, bestID = m, id
		}
	}
	return bestID, bestID != ""
}

// tick looks at the tape once. A write error ends the watching.
func (w *liveWatcher) tick() error {
	id, ok := w.c.srv.newestTape()
	if !ok {
		return nil
	}
	path := filepath.Join(w.c.srv.tapesDir(), tape.FileName(id))

	if id != w.tapeID {
		// Another tape is the newest now: the client starts its list again, with this tape from its first frame.
		data, err := os.ReadFile(path) //nolint:gosec // id passed tape.ValidID
		if err != nil {
			return nil // try again at the next tick
		}
		parsed := tape.Parse(data)
		w.tapeID, w.offset, w.builder = id, parsed.Consumed, timeline.Build(parsed.Events)
		for _, f := range w.builder.Frames() {
			if err := w.send(f); err != nil {
				return err
			}
		}
		return nil
	}

	f, err := os.Open(path) //nolint:gosec // id passed tape.ValidID
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || st.Size() <= int64(w.offset) {
		return nil
	}
	buf := make([]byte, st.Size()-int64(w.offset))
	if n, err := f.ReadAt(buf, int64(w.offset)); err != nil && n < len(buf) {
		return nil
	}
	parsed := tape.Parse(buf) // a last line without its newline is left for the next time
	w.offset += parsed.Consumed
	for _, e := range parsed.Events {
		if w.builder.Add(e) {
			frames := w.builder.Frames()
			if err := w.send(frames[len(frames)-1]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *liveWatcher) send(f timeline.Frame) error {
	return w.c.out.Notify("live/frame", struct {
		TapeID string    `json:"tapeId"`
		Frame  wireFrame `json:"frame"`
	}{w.tapeID, wire(f, w.withText)})
}
