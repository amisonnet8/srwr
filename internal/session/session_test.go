package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amisonnet8/srwr/internal/tape"
)

var idRe = regexp.MustCompile(`^\d{8}-\d{4}-[0-9a-z]{4}$`)

type clock struct{ t time.Time }

func (c *clock) Now() time.Time { return c.t }

func newClock() *clock {
	return &clock{t: time.Date(2026, 10, 1, 17, 6, 0, 0, time.FixedZone("JST", 9*3600))}
}

func open(t *testing.T, root string, c *clock) *Workspace {
	t.Helper()
	opts := Options{Gap: 30 * time.Minute, Version: "v-test"}
	if c != nil {
		opts.Now = c.Now
	}
	w, err := Open(root, opts)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func selectEvent(seq int) tape.Event {
	return tape.Event{Type: tape.TypeSelect, Seq: seq, File: "a.go", StartLine: 1, EndLine: 1, Why: tape.Str("見る"), Source: tape.SourceMCP}
}

func readTape(t *testing.T, w *Workspace, id string) tape.Result {
	t.Helper()
	b, err := tape.ReadAll(filepath.Dir(w.TapePath(id)), id)
	if err != nil {
		t.Fatal(err)
	}
	return tape.Parse(b)
}

func TestFirstCallMakesNoTape(t *testing.T) {
	root := t.TempDir()
	w := open(t, root, newClock())
	var id string
	err := w.Do(func(tx *Tx) error {
		id = tx.TapeID()
		if !idRe.MatchString(id) || id[:13] != "20261001-0806" {
			t.Errorf("TapeID = %q", id)
		}
		if tx.NextSeq() != 1 || len(tx.State().Files) != 0 || len(tx.Key()) != 32 {
			t.Errorf("fresh session: NextSeq = %d, files = %d, key = %d bytes", tx.NextSeq(), len(tx.State().Files), len(tx.Key()))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// "何も操作しなければ、空のテープは残らない"
	if _, err := os.Stat(w.TapePath(id)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a tape exists after a call that wrote nothing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".srwr", "active")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("active exists after a call that wrote nothing: %v", err)
	}
}

func TestAppendMakesTheTape(t *testing.T) {
	root := t.TempDir()
	c := newClock()
	w := open(t, root, c)
	w.SetAuthor(tape.Author{Kind: "ai", Name: "demo"})
	var id string
	if err := w.Do(func(tx *Tx) error {
		id = tx.TapeID()
		return tx.Append(selectEvent(1))
	}); err != nil {
		t.Fatal(err)
	}

	res := readTape(t, w, id)
	if res.Skipped != 0 || len(res.Events) != 2 {
		t.Fatalf("events = %d, skipped = %d", len(res.Events), res.Skipped)
	}
	h := res.Events[0]
	if h.Type != tape.TypeHeader || h.Session != id[len(id)-4:] || h.StartedAt != "2026-10-01T08:06:00.000Z" ||
		h.Author == nil || h.Author.Name != "demo" || h.Tool == nil || h.Tool.Name != "srwr" || h.Tool.Version != "v-test" {
		t.Errorf("header = %+v", h)
	}
	if e := res.Events[1]; e.Seq != 1 || e.TS != "2026-10-01T08:06:00.000Z" {
		t.Errorf("event = %+v", e)
	}
	active, _ := os.ReadFile(filepath.Join(root, ".srwr", "active")) //nolint:gosec // a path in a temporary directory
	if string(bytes.TrimSpace(active)) != id {
		t.Errorf("active = %q, want %q", active, id)
	}

	// The next call is in the same session and sees what was written.
	if err := w.Do(func(tx *Tx) error {
		if tx.TapeID() != id || tx.NextSeq() != 2 {
			t.Errorf("TapeID = %s NextSeq = %d", tx.TapeID(), tx.NextSeq())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAppendChecksSeq(t *testing.T) {
	w := open(t, t.TempDir(), newClock())
	err := w.Do(func(tx *Tx) error {
		if err := tx.Append(selectEvent(2)); err == nil {
			t.Error("seq 2 on an empty tape should be refused")
		}
		if err := tx.Append(selectEvent(1)); err != nil {
			return err
		}
		if err := tx.Append(selectEvent(1)); err == nil {
			t.Error("seq 1 twice should be refused")
		}
		return tx.Append(selectEvent(2))
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSessionEndsAfterTheGap(t *testing.T) {
	root := t.TempDir()
	c := newClock()
	w := open(t, root, c)
	var first string
	if err := w.Do(func(tx *Tx) error { first = tx.TapeID(); return tx.Append(selectEvent(1)) }); err != nil {
		t.Fatal(err)
	}

	// Exactly the gap later is still the same session; one millisecond more is a new one.
	c.t = c.t.Add(30 * time.Minute)
	if err := w.Do(func(tx *Tx) error {
		if tx.TapeID() != first {
			t.Errorf("after exactly the gap: TapeID = %s, want %s", tx.TapeID(), first)
		}
		return tx.Append(selectEvent(2))
	}); err != nil {
		t.Fatal(err)
	}

	c.t = c.t.Add(30*time.Minute + time.Millisecond)
	var second string
	if err := w.Do(func(tx *Tx) error {
		second = tx.TapeID()
		if second == first || tx.NextSeq() != 1 || len(tx.State().Files) != 0 {
			t.Errorf("after the gap: TapeID = %s (first %s), NextSeq = %d", second, first, tx.NextSeq())
		}
		// The pointer to the old tape stays until something is written.
		active, _ := os.ReadFile(filepath.Join(root, ".srwr", "active")) //nolint:gosec // a path in a temporary directory
		if string(bytes.TrimSpace(active)) != first {
			t.Errorf("active changed before a write: %q", active)
		}
		return tx.Append(selectEvent(1))
	}); err != nil {
		t.Fatal(err)
	}

	active, _ := os.ReadFile(filepath.Join(root, ".srwr", "active")) //nolint:gosec // a path in a temporary directory
	if string(bytes.TrimSpace(active)) != second {
		t.Errorf("active = %q, want %q", active, second)
	}
	if n := len(readTape(t, w, first).Events); n != 3 { // header + 2 events
		t.Errorf("the first tape has %d events, want 3", n)
	}
	if n := len(readTape(t, w, second).Events); n != 2 {
		t.Errorf("the second tape has %d events, want 2", n)
	}
}

func TestBadActive(t *testing.T) {
	for name, content := range map[string]string{
		"missing tape":    "20261001-0000-aaaa\n",
		"path":            "../../etc/passwd\n",
		"separator":       "a/b\n",
		"empty":           "\n",
		"dot":             "..\n",
		"binary":          "\x00\x01",
		"tape of nowhere": "x\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, ".srwr"), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ".srwr", "active"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			w := open(t, root, newClock())
			err := w.Do(func(tx *Tx) error {
				if !idRe.MatchString(tx.TapeID()) || tx.NextSeq() != 1 {
					t.Errorf("TapeID = %q NextSeq = %d", tx.TapeID(), tx.NextSeq())
				}
				return tx.Append(selectEvent(1))
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Two Workspaces on one directory stand for two processes.
func TestSeesWhatAnotherProcessWrote(t *testing.T) {
	root := t.TempDir()
	c := newClock()
	a, b := open(t, root, c), open(t, root, c)
	var id string
	if err := a.Do(func(tx *Tx) error { id = tx.TapeID(); return tx.Append(selectEvent(1)) }); err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= 6; i++ {
		w := []*Workspace{a, b}[i%2]
		if err := w.Do(func(tx *Tx) error {
			if tx.TapeID() != id || tx.NextSeq() != i {
				t.Fatalf("round %d: TapeID = %s, NextSeq = %d", i, tx.TapeID(), tx.NextSeq())
			}
			return tx.Append(selectEvent(i))
		}); err != nil {
			t.Fatal(err)
		}
	}
	res := readTape(t, a, id)
	if res.Skipped != 0 || len(res.Events) != 7 {
		t.Errorf("events = %d, skipped = %d", len(res.Events), res.Skipped)
	}
}

func TestFinishesAnUnfinishedLine(t *testing.T) {
	root := t.TempDir()
	w := open(t, root, newClock())
	var id string
	if err := w.Do(func(tx *Tx) error { id = tx.TapeID(); return tx.Append(selectEvent(1)) }); err != nil {
		t.Fatal(err)
	}
	// A writer died in the middle of a line.
	f, err := os.OpenFile(w.TapePath(id), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"v":1,"seq":2,"ts":"2026-10-01T17:06:00.000+09:00","type":"sel`); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	// Another process (a fresh Workspace) comes along, and so does the first one.
	for i, ws := range []*Workspace{open(t, root, newClock()), w} {
		if err := ws.Do(func(tx *Tx) error {
			if tx.NextSeq() != 2+i {
				t.Errorf("NextSeq = %d, want %d: the broken line is not an event", tx.NextSeq(), 2+i)
			}
			return tx.Append(selectEvent(tx.NextSeq()))
		}); err != nil {
			t.Fatal(err)
		}
	}
	res := readTape(t, w, id)
	var seqs []int
	for _, e := range res.Events[1:] {
		seqs = append(seqs, e.Seq)
	}
	if res.Skipped != 1 || !slices.Equal(seqs, []int{1, 2, 3}) {
		t.Errorf("skipped = %d, seqs = %v; want the broken line skipped and seqs 1 2 3", res.Skipped, seqs)
	}
}

func TestReadsAgainWhenTheTapeShrinks(t *testing.T) {
	root := t.TempDir()
	c := newClock()
	w := open(t, root, c)
	var id string
	if err := w.Do(func(tx *Tx) error {
		id = tx.TapeID()
		for i := 1; i <= 3; i++ {
			if err := tx.Append(selectEvent(i)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Someone replaces the tape with a shorter one (for instance, a copy of an earlier state).
	b, _ := os.ReadFile(w.TapePath(id))
	lines := bytes.SplitAfter(b, []byte("\n"))
	if err := os.WriteFile(w.TapePath(id), bytes.Join(lines[:2], nil), 0o600); err != nil { //nolint:gosec // a path in a temporary directory
		t.Fatal(err)
	}
	if err := w.Do(func(tx *Tx) error {
		if tx.NextSeq() != 2 {
			t.Errorf("NextSeq = %d, want 2 after the tape shrank", tx.NextSeq())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestKeyIsKept(t *testing.T) {
	root := t.TempDir()
	var keys [][]byte
	for range 3 {
		w := open(t, root, newClock())
		if err := w.Do(func(tx *Tx) error { keys = append(keys, slices.Clone(tx.Key())); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(keys[0], keys[1]) || !bytes.Equal(keys[1], keys[2]) {
		t.Error("the key changed between calls")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(root, ".srwr", "key"))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("key mode = %v, %v; want 0600", info.Mode(), err)
		}
	}
}

func TestWrongKeyLength(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".srwr"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".srwr", "key"), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := open(t, root, newClock())
	if err := w.Do(func(*Tx) error { return nil }); err == nil {
		t.Error("a key of the wrong length should be an error, not replaced")
	}
}

// Creating the key must be safe even for callers that do not hold the lock: two srwr mcp
// started together once failed because one read a key the other had half written.
func TestKeyCreationRace(t *testing.T) {
	dir := t.TempDir()
	const n = 32
	keys := make([][]byte, n)
	errs := make([]error, n)
	var ready, wg sync.WaitGroup
	start := make(chan struct{})
	ready.Add(n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready.Done()
			<-start
			keys[i], errs[i] = loadOrCreateKey(dir)
		}()
	}
	ready.Wait()
	close(start)
	wg.Wait()
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
		if !bytes.Equal(keys[i], keys[0]) {
			t.Fatalf("goroutine %d got a different key", i)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("%d files left in %s, want only the key", len(entries), dir)
	}
}

func TestLockExcludes(t *testing.T) {
	root := t.TempDir()
	a, b := open(t, root, nil), open(t, root, nil)
	inside := make(chan struct{})
	release := make(chan struct{})
	var order []string
	var mu sync.Mutex
	note := func(s string) { mu.Lock(); order = append(order, s); mu.Unlock() }

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = a.Do(func(*Tx) error {
			close(inside)
			<-release
			note("a leaves")
			return nil
		})
	}()
	<-inside
	go func() {
		defer wg.Done()
		_ = b.Do(func(*Tx) error { note("b enters"); return nil })
	}()
	time.Sleep(100 * time.Millisecond) // b is now waiting for the lock; nothing to poll for
	note("released")
	close(release)
	wg.Wait()
	if want := []string{"released", "a leaves", "b enters"}; !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

// Many writers at once, each its own Workspace as if each were a process: no seq is lost or
// repeated, there is one header, and one key.
func TestConcurrentWriters(t *testing.T) {
	root := t.TempDir()
	const writers, rounds = 8, 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	ids := map[string]bool{}
	keys := map[string]bool{}
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, err := Open(root, Options{})
			if err != nil {
				t.Error(err)
				return
			}
			for range rounds {
				err := w.Do(func(tx *Tx) error {
					mu.Lock()
					ids[tx.TapeID()] = true
					keys[string(tx.Key())] = true
					mu.Unlock()
					return tx.Append(selectEvent(tx.NextSeq()))
				})
				if err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if len(ids) != 1 || len(keys) != 1 {
		t.Fatalf("%d tapes and %d keys, want 1 and 1", len(ids), len(keys))
	}
	w := open(t, root, nil)
	var id string
	for id = range ids {
	}
	res := readTape(t, w, id)
	if res.Skipped != 0 || len(res.Events) != 1+writers*rounds {
		t.Fatalf("events = %d (skipped %d), want %d", len(res.Events), res.Skipped, 1+writers*rounds)
	}
	for i, e := range res.Events[1:] {
		if e.Seq != i+1 {
			t.Fatalf("event %d has seq %d, want %d", i, e.Seq, i+1)
		}
	}
	if res.Events[0].Type != tape.TypeHeader {
		t.Errorf("first event is %q", res.Events[0].Type)
	}
	entries, _ := os.ReadDir(filepath.Join(root, ".srwr"))
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	if want := []string{"active", "key", "lock", "tapes"}; !slices.Equal(names, want) {
		t.Errorf(".srwr holds %v, want %v (no temporary files left)", names, want)
	}
}

// The vcs of the header is asked for once for each tape, when the tape is made, and goes into the header as it is.
func TestHeaderGetsTheVCS(t *testing.T) {
	root := t.TempDir()
	c := newClock()
	calls := 0
	var askedAbout string
	w, err := Open(root, Options{Now: c.Now, Version: "v-test", VCS: func(r string) json.RawMessage {
		calls++
		askedAbout = r
		return json.RawMessage(`{"type":"git","head":"` + strings.Repeat("ab", 20) + `","dirty":true}`)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Do(func(tx *Tx) error { return nil }); err != nil || calls != 0 {
		t.Fatalf("a call that writes nothing asked for the vcs: calls %d err %v", calls, err)
	}
	var id string
	for i := 1; i <= 3; i++ {
		if err := w.Do(func(tx *Tx) error { id = tx.TapeID(); return tx.Append(selectEvent(i)) }); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 || askedAbout != w.Root() {
		t.Errorf("calls = %d about %q, want 1 about %q", calls, askedAbout, w.Root())
	}
	h := readTape(t, w, id).Events[0]
	if string(h.VCS) != `{"type":"git","head":"`+strings.Repeat("ab", 20)+`","dirty":true}` {
		t.Errorf("vcs = %s", h.VCS)
	}
	// A second process on the same tape does not make another header.
	w2, _ := Open(root, Options{Now: c.Now, VCS: func(string) json.RawMessage { calls++; return nil }})
	if err := w2.Do(func(tx *Tx) error { return tx.Append(selectEvent(4)) }); err != nil || calls != 1 {
		t.Errorf("calls = %d err %v", calls, err)
	}
	// A new session (after the gap) asks again.
	c.t = c.t.Add(time.Hour)
	if err := w.Do(func(tx *Tx) error { return tx.Append(selectEvent(1)) }); err != nil || calls != 2 {
		t.Errorf("calls = %d err %v", calls, err)
	}
}

// Without a VCS function the header asks git (internal/vcs); when git cannot answer, the header says null.
func TestHeaderVCSDefaultsToAskingGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no git
	root := t.TempDir()
	w := open(t, root, newClock())
	var id string
	if err := w.Do(func(tx *Tx) error { id = tx.TapeID(); return tx.Append(selectEvent(1)) }); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(w.TapePath(id))
	first, _, _ := bytes.Cut(b, []byte("\n"))
	if !strings.Contains(string(first), `"vcs":null`) {
		t.Errorf("header line = %s", first)
	}
}

// writeOne starts a session with one event and returns the ID of its tape.
func writeOne(t *testing.T, w *Workspace) string {
	t.Helper()
	var id string
	if err := w.Do(func(tx *Tx) error { id = tx.TapeID(); return tx.Append(selectEvent(1)) }); err != nil {
		t.Fatal(err)
	}
	return id
}

func tapesDir(w *Workspace) string { return filepath.Dir(w.TapePath("x")) }

// The session that is left alone is closed by the call that starts the next one, and its tape is compressed then.
func TestTheTapeOfASessionLeftAloneIsCompressed(t *testing.T) {
	root := t.TempDir()
	c := newClock()
	w := open(t, root, c)
	first := writeOne(t, w)
	if _, err := os.Stat(w.TapePath(first)); err != nil {
		t.Fatalf("a session that goes on is not compressed: %v", err)
	}
	c.t = c.t.Add(30*time.Minute + time.Millisecond)
	if err := w.Do(func(tx *Tx) error { return nil }); err != nil { // nothing is written
		t.Fatal(err)
	}
	if _, err := os.Stat(w.TapePath(first)); !os.IsNotExist(err) {
		t.Errorf("the plain tape of the closed session is still there: %v", err)
	}
	if _, err := os.Stat(w.TapePath(first) + tape.GzSuffix); err != nil {
		t.Errorf("no compressed tape: %v", err)
	}
	if n := len(readTape(t, w, first).Events); n != 2 {
		t.Errorf("the closed tape has %d events, want 2", n)
	}
	second := writeOne(t, w)
	if _, err := os.Stat(w.TapePath(second)); err != nil {
		t.Errorf("the new session's tape is not plain: %v", err)
	}
	if got := tape.IDs(tapesDir(w)); !slices.Equal(got, []string{first, second}) && !slices.Equal(got, []string{second, first}) {
		t.Errorf("tapes = %v", got)
	}
}

func TestEndSessionCompressesTheTape(t *testing.T) {
	root := t.TempDir()
	w := open(t, root, newClock())
	first := writeOne(t, w)
	id, err := w.EndSession()
	if err != nil || id != first {
		t.Fatalf("EndSession = %q, %v", id, err)
	}
	if _, err := os.Stat(w.TapePath(first)); !os.IsNotExist(err) {
		t.Errorf("the plain tape is still there: %v", err)
	}
	if n := len(readTape(t, w, first).Events); n != 2 {
		t.Errorf("the closed tape has %d events, want 2", n)
	}
	if _, ok := w.Current(); ok {
		t.Error("a closed session is still the current one")
	}
	if id2, err := w.EndSession(); err != nil || id2 != "" {
		t.Errorf("a second EndSession = %q, %v", id2, err)
	}
}

// Compressing only saves space. When it cannot be done, the session still ends and the next one still starts.
func TestACompressFailureStopsNothing(t *testing.T) {
	root := t.TempDir()
	c := newClock()
	w := open(t, root, c)
	first := writeOne(t, w)
	// A directory where the compressed tape would go: the rename fails.
	blocker := filepath.Join(w.TapePath(first) + tape.GzSuffix)
	if err := os.MkdirAll(filepath.Join(blocker, "x"), 0o750); err != nil {
		t.Fatal(err)
	}
	id, err := w.EndSession()
	var cerr *CompressError
	if !errors.As(err, &cerr) || id != first || cerr.ID != first {
		t.Fatalf("EndSession = %q, %v; want a *CompressError for %s", id, err, first)
	}
	if _, statErr := os.Stat(w.TapePath(first)); statErr != nil {
		t.Errorf("the plain tape was lost: %v", statErr)
	}
	if n := len(readTape(t, w, first).Events); n != 2 {
		t.Errorf("the tape has %d events, want 2", n)
	}
	entries, _ := os.ReadDir(tapesDir(w))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("a temporary file is left: %s", e.Name())
		}
	}
	second := writeOne(t, w) // the next write starts a new session
	if second == first {
		t.Error("the session did not end")
	}

	// The same for a session left alone: the call that starts the next one does not fail.
	c.t = c.t.Add(31 * time.Minute)
	if err := w.Do(func(tx *Tx) error { return tx.Append(selectEvent(tx.NextSeq())) }); err != nil {
		t.Fatalf("a write after a pause stopped: %v", err)
	}
}

// Several writers meet the end of the pause together: the tape is closed once, the new session is one tape, and nobody loses an event.
func TestWritersMeetingTheEndOfAPauseTogether(t *testing.T) {
	root := t.TempDir()
	c := newClock()
	first := writeOne(t, open(t, root, c))
	later := c.t.Add(time.Hour)
	const writers, rounds = 8, 5
	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, err := Open(root, Options{Gap: 30 * time.Minute, Now: func() time.Time { return later }})
			if err != nil {
				t.Error(err)
				return
			}
			for range rounds {
				if err := w.Do(func(tx *Tx) error { return tx.Append(selectEvent(tx.NextSeq())) }); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	w := open(t, root, c)
	ids := tape.IDs(tapesDir(w))
	if len(ids) != 2 {
		t.Fatalf("tapes = %v, want the closed one and one new", ids)
	}
	if n := len(readTape(t, w, first).Events); n != 2 {
		t.Errorf("the closed tape has %d events, want 2", n)
	}
	if _, err := os.Stat(w.TapePath(first)); !os.IsNotExist(err) {
		t.Errorf("the plain tape of the closed session is still there: %v", err)
	}
	for _, id := range ids {
		if id == first {
			continue
		}
		res := readTape(t, w, id)
		if res.Skipped != 0 || len(res.Events) != 1+writers*rounds {
			t.Errorf("the new tape has %d events (skipped %d), want %d", len(res.Events), res.Skipped, 1+writers*rounds)
		}
	}
	entries, _ := os.ReadDir(tapesDir(w))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("a temporary file is left: %s", e.Name())
		}
	}
}

// A new tape never takes the ID of a closed one.
func TestNewIDAvoidsClosedTapes(t *testing.T) {
	root := t.TempDir()
	w := open(t, root, newClock())
	first := writeOne(t, w)
	if _, err := w.EndSession(); err != nil {
		t.Fatal(err)
	}
	if !w.taken(first) || w.taken("20261001-0000-zzzz") {
		t.Error("taken does not tell a closed tape from a free ID")
	}
	for range 50 {
		if id, err := w.newID(time.Now()); err != nil || id == first {
			t.Fatalf("newID = %q, %v", id, err)
		}
	}
}

// Nothing is appended to a tape that is closed, even when its pointer is still the active one and the pause is not over: the
// next write starts a new tape, and the closed one is left as it was.
func TestAClosedTapeIsNotAppendedTo(t *testing.T) {
	root := t.TempDir()
	w := open(t, root, newClock())
	first := writeOne(t, w)
	if err := tape.Compress(tapesDir(w), first); err != nil { // closed by someone else; .srwr/active still points at it
		t.Fatal(err)
	}
	second := writeOne(t, w)
	if second == first {
		t.Fatal("the closed tape was written to again")
	}
	if _, err := os.Stat(w.TapePath(first)); !os.IsNotExist(err) {
		t.Errorf("a plain tape was made again for the closed session: %v", err)
	}
	if n := len(readTape(t, w, first).Events); n != 2 {
		t.Errorf("the closed tape has %d events, want 2", n)
	}
}
