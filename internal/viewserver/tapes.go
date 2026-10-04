package viewserver

import (
	"errors"
	"io/fs"
	"os"
	"sort"
	"time"

	"github.com/amisonnet8/srwr/internal/jsonrpc"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/timeline"
)

// tapeInfo is an entry of tapes/list.
type tapeInfo struct {
	TapeID    string   `json:"tapeId"`
	StartedAt string   `json:"startedAt"`
	UpdatedAt string   `json:"updatedAt"`
	Ops       int      `json:"ops"`
	Files     []string `json:"files"`
}

// listEntry remembers what was made of a tape, so that a tape that has not changed is not read again.
type listEntry struct {
	size int64
	mod  time.Time
	info *tapeInfo // nil: not listed (no operations, or unreadable)
}

// tapeIDs returns the IDs of the tapes in the workspace, newest first by name.
func (s *Server) tapeIDs() []string {
	ids := tape.IDs(s.tapesDir())
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids
}

// startTime is when the tape started: the header's time, or the last update for a tape without a header.
func (t tapeInfo) startTime() time.Time {
	for _, s := range []string{t.StartedAt, t.UpdatedAt} {
		if v, err := time.Parse(time.RFC3339, s); err == nil {
			return v
		}
	}
	return time.Time{}
}

func (c *conn) tapesList() (any, *jsonrpc.Error) {
	infos := []tapeInfo{}
	for _, id := range c.srv.tapeIDs() {
		if info := c.info(id); info != nil {
			infos = append(infos, *info)
		}
	}
	// Newest first by the time the tape started (older tapes were named in the local zone, so the ID is not enough).
	sort.SliceStable(infos, func(i, j int) bool { return infos[i].startTime().After(infos[j].startTime()) })
	return struct {
		Tapes []tapeInfo `json:"tapes"`
	}{infos}, nil
}

func (c *conn) info(id string) *tapeInfo {
	path, found := tape.Find(c.srv.tapesDir(), id)
	if !found {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if e, ok := c.cache[id]; ok && e.size == st.Size() && e.mod.Equal(st.ModTime()) {
		return e.info
	}
	var info *tapeInfo
	if data, err := tape.ReadFile(path); err == nil {
		res := tape.Parse(data)
		b := timeline.Build(res.Events)
		if b.Ops() > 0 {
			started := ""
			for _, e := range res.Events {
				if e.Type == tape.TypeHeader {
					started = e.StartedAt
					break
				}
			}
			info = &tapeInfo{TapeID: id, StartedAt: started, UpdatedAt: tape.FormatTS(st.ModTime()), Ops: b.Ops(), Files: append([]string{}, b.Files()...)}
		}
	}
	c.cache[id] = listEntry{size: st.Size(), mod: st.ModTime(), info: info}
	return info
}

// tapeParam reads and checks a tapeId.
func tapeParam(id string) *jsonrpc.Error {
	if !tape.ValidID(id) {
		return invalidParams("invalid tapeId: " + id)
	}
	return nil
}

// readTape reads a whole tape.
func (s *Server) readTape(id string) (tape.Result, *jsonrpc.Error) {
	data, err := tape.ReadAll(s.tapesDir(), id)
	if errors.Is(err, fs.ErrNotExist) {
		return tape.Result{}, rpcError(serverError, codeTapeNotFound, "no such tape: "+id)
	}
	if err != nil {
		return tape.Result{}, rpcError(serverError, codeTapeUnreadable, "cannot read the tape: "+id+": "+err.Error())
	}
	return tape.Parse(data), nil
}

func (c *conn) tapeOpen(raw []byte) (any, *jsonrpc.Error) {
	var p struct {
		TapeID   string    `json:"tapeId"`
		WithText bool      `json:"withText"`
		Kinds    *[]string `json:"kinds"`
	}
	if err := decode(raw, &p); err != nil {
		return nil, err
	}
	if err := tapeParam(p.TapeID); err != nil {
		return nil, err
	}
	kinds, kerr := parseKinds(p.Kinds)
	if kerr != nil {
		return nil, kerr
	}
	res, rerr := c.srv.readTape(p.TapeID)
	if rerr != nil {
		return nil, rerr
	}
	b := timeline.Build(res.Events)
	frames := b.Frames()
	if c.diffFrames {
		frames = timeline.AppendFinals(frames, b.State(), b.Files(), c.srv.readCurrent)
	}
	shown, orig, hidden := timeline.Filter(frames, kinds)
	c.opened[p.TapeID] = &openTape{all: frames, orig: orig}
	return struct {
		Frames []wireFrame     `json:"frames"`
		Hidden timeline.Hidden `json:"hidden,omitempty"`
		TapeID string          `json:"tapeId"`
	}{wireAll(shown, p.WithText), hidden, p.TapeID}, nil
}

func (c *conn) tapeClose(raw []byte) (any, *jsonrpc.Error) {
	var p struct {
		TapeID string `json:"tapeId"`
	}
	if err := decode(raw, &p); err != nil {
		return nil, err
	}
	if err := tapeParam(p.TapeID); err != nil {
		return nil, err
	}
	if _, ok := c.opened[p.TapeID]; !ok {
		return nil, rpcError(serverError, codeTapeNotFound, "the tape is not open: "+p.TapeID)
	}
	delete(c.opened, p.TapeID)
	return struct{}{}, nil
}

func (c *conn) frameState(raw []byte) (any, *jsonrpc.Error) {
	var p struct {
		TapeID string `json:"tapeId"`
		Index  *int   `json:"index"`
		File   string `json:"file"`
	}
	if err := decode(raw, &p); err != nil {
		return nil, err
	}
	if err := tapeParam(p.TapeID); err != nil {
		return nil, err
	}
	if p.Index == nil {
		return nil, invalidParams("index is missing")
	}
	t, ok := c.opened[p.TapeID]
	if !ok {
		return nil, rpcError(serverError, codeTapeNotFound, "the tape is not open: "+p.TapeID)
	}
	i := *p.Index
	if i < -1 || i >= len(t.orig) {
		return nil, invalidParams("index is out of range: " + itoa(i))
	}

	res := struct {
		After   string  `json:"after"`
		Before  string  `json:"before"`
		Content *string `json:"content"`
	}{}
	file := p.File
	at := -1 // the frame among all of them
	if i >= 0 {
		at = t.orig[i]
		res.Before, res.After = t.all[at].Before, t.all[at].After
		if file == "" {
			file = t.all[at].File
		}
	}
	if file != "" {
		if text, ok := timeline.ContentAt(t.all, file, at); ok {
			res.Content = &text
		}
	}
	return res, nil
}
