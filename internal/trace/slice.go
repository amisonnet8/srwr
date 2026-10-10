package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"

	"github.com/amisonnet8/srwr/internal/tape"
)

// Cut is a tape made of operations of other tapes (srwr trace --as-tape): its ID, and its events with the header first.
type Cut struct {
	ID     string
	Events []tape.Event
}

// Slice makes one replayable tape of the operations ops (which wrote the lines of one commit).
//
// Taking only those operations would break the replay: their line numbers assume the other edits of the file before them. So for
// every file of every source tape, the tape holds everything that happened to the file between the first and the last of the
// operations (looks, edits, news, external changes), preceded by a snapshot of the file as it was before the first. The sources
// follow one another, oldest first. Seq starts again from 1, and the tokens (selection, from) of the sources are dropped: they
// are not valid on this tape. key (the commit) and the time of the first operation make the ID, so the same input makes the same tape.
//
// ok is false when ops is empty.
func Slice(srcs []Source, ops []*Op, key, version string) (Cut, bool) {
	if len(ops) == 0 {
		return Cut{}, false
	}
	type span struct{ lo, hi int }
	type fileKey struct {
		src  int
		file string
	}
	spans := map[fileKey]span{}
	first := map[int]time.Time{} // the time of the first operation of each source
	for _, o := range ops {
		k := fileKey{o.Source, o.File}
		sp, ok := spans[k]
		if !ok {
			sp = span{lo: o.Index, hi: o.Index}
		}
		sp.lo, sp.hi = min(sp.lo, o.Index), max(sp.hi, o.Index)
		spans[k] = sp
		if t, ok := first[o.Source]; !ok || (!o.Time.IsZero() && o.Time.Before(t)) {
			first[o.Source] = o.Time
		}
	}
	var order []int
	for si := range first {
		order = append(order, si)
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := first[order[i]], first[order[j]]
		if !a.Equal(b) {
			return a.Before(b)
		}
		return srcs[order[i]].ID < srcs[order[j]].ID
	})

	var body []tape.Event
	seq := 0
	emit := func(e tape.Event) {
		seq++
		e.Seq = seq
		body = append(body, e)
	}
	for _, si := range order {
		st := tape.NewState()
		for i, e := range srcs[si].Events {
			if e.Type != tape.TypeHeader {
				if sp, ok := spans[fileKey{si, e.File}]; ok && i >= sp.lo && i <= sp.hi && cuttable(e.Type) {
					if i == sp.lo { // the first event of the file in the cut: the file as it was before it
						if f := st.Files[e.File]; f != nil && !f.Deleted {
							text := f.Text
							emit(tape.Event{Type: tape.TypeSnapshot, TS: e.TS, File: e.File, FileHash: tape.FileHash(e.File), Text: &text, Sha: tape.Sha(text)})
						}
					}
					c := e
					c.Selection, c.From = nil, nil
					emit(c)
				}
			}
			st.Apply(e)
		}
	}
	if len(body) == 0 {
		return Cut{}, false
	}

	start, _ := time.Parse(time.RFC3339, body[0].TS) // the first event of the cut, whatever the time of the operations
	short := hashPrefix(key)
	id := start.UTC().Format("20060102-1504") + "-" + short
	head := tape.Event{
		Type: tape.TypeHeader, Session: short, StartedAt: body[0].TS,
		Author: &tape.Author{Kind: DerivedKind, Name: "srwr trace"},
		VCS:    srcs[order[0]].Header.VCS, Tool: &tape.ToolInfo{Name: "srwr", Version: version},
	}
	return Cut{ID: id, Events: append([]tape.Event{head}, body...)}, true
}

func cuttable(t string) bool {
	switch t {
	case tape.TypeLook, tape.TypeEdit, tape.TypeNew, tape.TypeExternal, tape.TypeSnapshot:
		return true
	}
	return false
}

// hashPrefix is the 4 hexadecimal characters that end an ID: the start of the commit when key is one, else of its hash.
func hashPrefix(key string) string {
	if len(key) >= 4 && isHex(key[:4]) {
		return key[:4]
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:2])
}

func isHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
