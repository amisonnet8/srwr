package session

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
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
	b, err := os.ReadFile(w.TapePath(id))
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
		if !idRe.MatchString(id) || id[:13] != "20261001-1706" {
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
	if h.Type != tape.TypeHeader || h.Session != id[len(id)-4:] || h.StartedAt != "2026-10-01T17:06:00.000+09:00" ||
		h.Author == nil || h.Author.Name != "demo" || h.Tool == nil || h.Tool.Name != "srwr" || h.Tool.Version != "v-test" {
		t.Errorf("header = %+v", h)
	}
	if e := res.Events[1]; e.Seq != 1 || e.TS != "2026-10-01T17:06:00.000+09:00" {
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
		// The old tape is not touched until something is written.
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
