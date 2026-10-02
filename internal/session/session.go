// Package session decides which tape is current, serializes writers with a lock, and reads the
// tape up to date before anything runs on it. srwr mcp and srwr hook both write the tape only
// through here, so that several processes in one workspace share one tape.
//
// Nothing here keeps state of its own between calls except a cache of what was already read
// from the tape; every call first reads whatever other processes appended (.claude/rules/go-code.md).
package session

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amisonnet8/srwr/internal/tape"
)

// DefaultGap is how long after its last event a session is still the current one.
const DefaultGap = 30 * time.Minute

// Options tune a Workspace. The zero value is right for real use.
type Options struct {
	Now     func() time.Time // for tests; default time.Now
	Gap     time.Duration    // default DefaultGap
	Version string           // written to the header of new tapes
}

// Workspace is the .srwr directory of a working directory.
type Workspace struct {
	root string
	dir  string
	opts Options

	author atomic.Pointer[tape.Author]

	// mu keeps goroutines of this process out of each other's way; the file lock does the same
	// between processes. Everything below is guarded by it.
	mu    sync.Mutex
	cache *tapeCache
}

// Open returns the Workspace for root. It does not touch the file system.
func Open(root string, opts Options) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Gap <= 0 {
		opts.Gap = DefaultGap
	}
	w := &Workspace{root: abs, dir: filepath.Join(abs, ".srwr"), opts: opts}
	w.author.Store(&tape.Author{Kind: "ai", Name: "unknown"})
	return w, nil
}

// Root returns the absolute path of the workspace.
func (w *Workspace) Root() string { return w.root }

// SetAuthor sets who the header of a new tape says wrote it.
func (w *Workspace) SetAuthor(a tape.Author) { w.author.Store(&a) }

// TapePath returns the path of the tape with the given ID.
func (w *Workspace) TapePath(id string) string {
	return filepath.Join(w.dir, "tapes", tape.FileName(id))
}

// tapeCache is what was read from one tape so far.
type tapeCache struct {
	id     string
	state  *tape.State
	offset int64     // bytes of the tape that are in state
	tail   int64     // bytes after offset: a line a crashed writer left unfinished
	last   time.Time // time of the last event
	exists bool      // the tape file exists (false for a session that has not written yet)
}

// Tx is the current session while a call to Do runs. It must not be used after Do returns.
type Tx struct {
	w   *Workspace
	tc  *tapeCache
	key []byte
}

// Do runs fn with the workspace lock held. Before fn runs, the current session is decided and
// its tape is read up to date, so State and NextSeq include what other processes wrote.
func (w *Workspace) Do(fn func(*Tx) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.MkdirAll(filepath.Join(w.dir, "tapes"), 0o750); err != nil {
		return err
	}
	unlock, err := lockFile(filepath.Join(w.dir, "lock"))
	if err != nil {
		return fmt.Errorf("lock %s: %w", filepath.Join(w.dir, "lock"), err)
	}
	defer unlock()

	key, err := loadOrCreateKey(w.dir)
	if err != nil {
		return err
	}
	tc, err := w.current()
	if err != nil {
		return err
	}
	return fn(&Tx{w: w, tc: tc, key: key})
}

// current decides which session is current and reads its tape. When there is none, it
// returns an empty session with a fresh tape ID; the tape is made when something is appended.
func (w *Workspace) current() (*tapeCache, error) {
	now := w.opts.Now()
	if id, ok := w.readActive(); ok {
		tc, err := w.load(id)
		if err != nil {
			return nil, err
		}
		if tc.exists && now.Sub(tc.last) <= w.opts.Gap {
			w.cache = tc
			return tc, nil
		}
	}
	id, err := w.newID(now)
	if err != nil {
		return nil, err
	}
	w.cache = &tapeCache{id: id, state: tape.NewState()}
	return w.cache, nil
}

// readActive returns the tape ID in .srwr/active, if it is a usable one.
func (w *Workspace) readActive() (string, bool) {
	b, err := os.ReadFile(filepath.Join(w.dir, "active"))
	if err != nil {
		return "", false
	}
	id := strings.TrimSpace(string(b))
	return id, tape.ValidID(id)
}

func (w *Workspace) writeActive(id string) error {
	tmp, err := os.CreateTemp(w.dir, "active-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.WriteString(id + "\n")
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(tmp.Name())
		return werr
	}
	if err := os.Rename(tmp.Name(), filepath.Join(w.dir, "active")); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

// load reads the tape with the given ID up to date, continuing from where the cache stopped.
func (w *Workspace) load(id string) (*tapeCache, error) {
	path := w.TapePath(id)
	f, err := os.Open(path) //nolint:gosec // id passed tape.ValidID
	if errors.Is(err, os.ErrNotExist) {
		return &tapeCache{id: id, state: tape.NewState()}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	tc := w.cache
	if tc == nil || tc.id != id || !tc.exists || info.Size() < tc.offset {
		// A different tape, or one that shrank: someone replaced it. Read it again from the start.
		tc = &tapeCache{id: id, state: tape.NewState(), exists: true}
	}
	buf := make([]byte, info.Size()-tc.offset)
	if _, err := f.ReadAt(buf, tc.offset); err != nil && len(buf) > 0 {
		return nil, err
	}
	res := tape.Parse(buf)
	for _, e := range res.Events {
		tc.state.Apply(e)
		if t, ok := eventTime(e); ok {
			tc.last = t
		}
	}
	tc.offset += int64(res.Consumed)
	tc.tail = int64(len(buf) - res.Consumed)
	if tc.last.IsZero() {
		tc.last = info.ModTime()
	}
	return tc, nil
}

// eventTime returns the time an event happened, if the tape says it in a form that can be read.
func eventTime(e tape.Event) (time.Time, bool) {
	s := e.TS
	if e.Type == tape.TypeHeader {
		s = e.StartedAt
	}
	t, err := time.Parse(time.RFC3339, s)
	return t, err == nil
}

// newID returns a tape ID such as "20261001-1706-1795" that no tape has yet.
func (w *Workspace) newID(now time.Time) (string, error) {
	const chars = "0123456789abcdefghijklmnopqrstuvwxyz"
	for range 100 {
		var b [4]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		for i := range b {
			b[i] = chars[int(b[i])%len(chars)]
		}
		id := now.Format("20060102-1504") + "-" + string(b[:])
		if _, err := os.Stat(w.TapePath(id)); errors.Is(err, os.ErrNotExist) {
			return id, nil
		}
	}
	return "", errors.New("could not find an unused tape ID")
}

// TapeID returns the ID of the current tape. For a session that has not written yet, it is the
// ID the tape will have, so that tokens issued now are valid in it.
func (t *Tx) TapeID() string { return t.tc.id }

// Key returns the HMAC key of the workspace.
func (t *Tx) Key() []byte { return t.key }

// State returns what the tape says so far. Callers must not change it.
func (t *Tx) State() *tape.State { return t.tc.state }

// NextSeq returns the seq the next appended event must have.
func (t *Tx) NextSeq() int { return t.tc.state.LastSeq + 1 }

// Now returns the current time of the workspace's clock.
func (t *Tx) Now() time.Time { return t.w.opts.Now() }

// Append writes e at the end of the tape. e.Seq must be NextSeq(): callers need the seq before
// the event exists, to put it in the token. An empty TS is filled in. The first Append of a
// session makes the tape, with its header, and points .srwr/active at it.
func (t *Tx) Append(e tape.Event) error {
	if e.Seq != t.NextSeq() {
		return fmt.Errorf("seq %d appended, want %d", e.Seq, t.NextSeq())
	}
	if e.TS == "" {
		e.TS = tape.FormatTS(t.Now())
	}
	line, err := tape.Marshal(e)
	if err != nil {
		return err
	}
	path := t.w.TapePath(t.tc.id)

	if !t.tc.exists {
		if err := t.createTape(path); err != nil {
			return err
		}
	}
	if t.tc.tail > 0 {
		// A writer died in the middle of a line. End it, so that it does not swallow this event.
		if err := tape.AppendLine(path, []byte{'\n'}); err != nil {
			return err
		}
		t.tc.offset += t.tc.tail + 1
		t.tc.tail = 0
	}
	if err := tape.AppendLine(path, line); err != nil {
		return err
	}
	t.tc.offset += int64(len(line))
	t.tc.state.Apply(e)
	if when, ok := eventTime(e); ok {
		t.tc.last = when
	}
	return nil
}

// createTape writes the header of a new tape and makes it the current one.
func (t *Tx) createTape(path string) error {
	header := tape.Event{
		Type:      tape.TypeHeader,
		Session:   t.tc.id[strings.LastIndexByte(t.tc.id, '-')+1:],
		StartedAt: tape.FormatTS(t.Now()),
		Author:    t.w.author.Load(),
		Tool:      &tape.ToolInfo{Name: "srwr", Version: t.w.opts.Version},
	}
	line, err := tape.Marshal(header)
	if err != nil {
		return err
	}
	if err := tape.AppendLine(path, line); err != nil {
		return err
	}
	if err := t.w.writeActive(t.tc.id); err != nil {
		return err
	}
	t.tc.exists = true
	t.tc.offset = int64(len(line))
	t.tc.tail = 0
	t.tc.last = t.Now()
	return nil
}
