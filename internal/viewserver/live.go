package viewserver

import (
	"os"
	"strings"
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
	kinds    timeline.Kinds
	shown    int             // how many frames of this tape were sent: the next one has this index
	hidden   timeline.Hidden // how many of each kind were left out

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
	TapeID *string         `json:"tapeId"`
	Frames []wireFrame     `json:"frames"`
	Hidden timeline.Hidden `json:"hidden,omitempty"`

	w *liveWatcher
}

// After starts watching, now that the client has the response: a notification must never come before it.
func (r liveStartResult) After() bool {
	r.w.start()
	return false
}

func (c *conn) liveStart(raw []byte) (any, *jsonrpc.Error) {
	var p struct {
		WithText bool      `json:"withText"`
		Kinds    *[]string `json:"kinds"`
	}
	if err := decode(raw, &p); err != nil {
		return nil, err
	}
	kinds, kerr := parseKinds(p.Kinds)
	if kerr != nil {
		return nil, kerr
	}
	c.stopLive()
	w := &liveWatcher{c: c, withText: p.WithText, kinds: kinds, hidden: timeline.Hidden{}, builder: timeline.NewBuilder(), quit: make(chan struct{}), done: make(chan struct{})}
	c.live = w

	res := liveStartResult{Frames: []wireFrame{}, Hidden: timeline.Hidden{}, w: w}
	if id, ok := c.srv.newestTape(); ok {
		if data, err := tape.ReadAll(c.srv.tapesDir(), id); err == nil {
			parsed := tape.Parse(data)
			w.tapeID, w.offset, w.builder = id, parsed.Consumed, timeline.Build(parsed.Events)
			res.TapeID = &id
			shown, _, hidden := timeline.Filter(w.builder.Frames(), kinds)
			w.shown, w.hidden = len(shown), hidden
			res.Frames, res.Hidden = wireAll(shown, p.WithText), hidden
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
		path, found := tape.Find(s.tapesDir(), id)
		if !found {
			continue
		}
		st, err := os.Stat(path)
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
	path, found := tape.Find(w.c.srv.tapesDir(), id)
	if !found {
		return nil
	}

	if id != w.tapeID {
		// Another tape is the newest now: the client starts its list again, with this tape from its first frame.
		data, err := tape.ReadFile(path)
		if err != nil {
			return nil //nolint:nilerr // try again at the next tick
		}
		parsed := tape.Parse(data)
		w.tapeID, w.offset, w.builder = id, parsed.Consumed, timeline.Build(parsed.Events)
		w.shown, w.hidden = 0, timeline.Hidden{}
		for _, f := range w.builder.Frames() {
			if err := w.offer(f); err != nil {
				return err
			}
		}
		if len(w.hidden) == 0 {
			return nil
		}
		return w.notifyHidden()
	}

	buf := newBytes(path, w.offset)
	if len(buf) == 0 {
		return nil
	}
	parsed := tape.Parse(buf) // a last line without its newline is left for the next time
	w.offset += parsed.Consumed
	before := len(w.hidden)
	left := w.hiddenTotal()
	for _, e := range parsed.Events {
		if w.builder.Add(e) {
			frames := w.builder.Frames()
			if err := w.offer(frames[len(frames)-1]); err != nil {
				return err
			}
		}
	}
	if len(w.hidden) != before || w.hiddenTotal() != left {
		return w.notifyHidden()
	}
	return nil
}

// offer sends a frame if the client asked for its kind, numbered after the ones sent. Otherwise it only counts it.
func (w *liveWatcher) offer(f timeline.Frame) error {
	if !w.kinds.Shows(f.Kind) {
		w.hidden[f.Kind]++
		return nil
	}
	f.Index = w.shown
	w.shown++
	return w.send(f)
}

func (w *liveWatcher) hiddenTotal() int {
	n := 0
	for _, v := range w.hidden {
		n += v
	}
	return n
}

// notifyHidden tells the client how many frames of each kind are left out now.
func (w *liveWatcher) notifyHidden() error {
	return w.c.out.Notify("live/hidden", struct {
		TapeID string          `json:"tapeId"`
		Hidden timeline.Hidden `json:"hidden"`
	}{w.tapeID, w.hidden})
}

func (w *liveWatcher) send(f timeline.Frame) error {
	return w.c.out.Notify("live/frame", struct {
		TapeID string    `json:"tapeId"`
		Frame  wireFrame `json:"frame"`
	}{w.tapeID, wire(f, w.withText)})
}

// newBytes returns what the tape at path holds after offset, or nothing. A closed tape is compressed: it is expanded, so that what
// was written just before the session ended and not yet read is not lost.
func newBytes(path string, offset int) []byte {
	if strings.HasSuffix(path, tape.GzSuffix) {
		data, err := tape.ReadFile(path)
		if err != nil || len(data) <= offset {
			return nil
		}
		return data[offset:]
	}
	f, err := os.Open(path) //nolint:gosec // the path was found from a tape ID that passed tape.ValidID
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || st.Size() <= int64(offset) {
		return nil
	}
	buf := make([]byte, st.Size()-int64(offset))
	if n, err := f.ReadAt(buf, int64(offset)); err != nil && n < len(buf) {
		return nil
	}
	return buf
}
