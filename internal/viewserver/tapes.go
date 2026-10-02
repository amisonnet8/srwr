package viewserver

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	entries, err := os.ReadDir(s.tapesDir())
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), tape.FileSuffix)
		if ok && !e.IsDir() && tape.ValidID(id) {
			ids = append(ids, id)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids
}

func (c *conn) tapesList() (any, *jsonrpc.Error) {
	infos := []tapeInfo{}
	for _, id := range c.srv.tapeIDs() {
		if info := c.info(id); info != nil {
			infos = append(infos, *info)
		}
	}
	return struct {
		Tapes []tapeInfo `json:"tapes"`
	}{infos}, nil
}

func (c *conn) info(id string) *tapeInfo {
	path := filepath.Join(c.srv.tapesDir(), tape.FileName(id))
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if e, ok := c.cache[id]; ok && e.size == st.Size() && e.mod.Equal(st.ModTime()) {
		return e.info
	}
	var info *tapeInfo
	if data, err := os.ReadFile(path); err == nil { //nolint:gosec // id passed tape.ValidID
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
		return invalidParams("tapeId が不正: " + id)
	}
	return nil
}

// readTape reads a whole tape.
func (s *Server) readTape(id string) (tape.Result, *jsonrpc.Error) {
	data, err := os.ReadFile(filepath.Join(s.tapesDir(), tape.FileName(id)))
	if errors.Is(err, fs.ErrNotExist) {
		return tape.Result{}, rpcError(serverError, codeTapeNotFound, "テープがない: "+id)
	}
	if err != nil {
		return tape.Result{}, rpcError(serverError, codeTapeUnreadable, "テープを読めない: "+id+": "+err.Error())
	}
	return tape.Parse(data), nil
}

func (c *conn) tapeOpen(raw []byte) (any, *jsonrpc.Error) {
	var p struct {
		TapeID   string `json:"tapeId"`
		WithText bool   `json:"withText"`
	}
	if err := decode(raw, &p); err != nil {
		return nil, err
	}
	if err := tapeParam(p.TapeID); err != nil {
		return nil, err
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
	c.opened[p.TapeID] = &openTape{frames: frames}
	return struct {
		Frames []wireFrame `json:"frames"`
		TapeID string      `json:"tapeId"`
	}{wireAll(frames, p.WithText), p.TapeID}, nil
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
		return nil, rpcError(serverError, codeTapeNotFound, "開いていないテープ: "+p.TapeID)
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
		return nil, invalidParams("index がない")
	}
	t, ok := c.opened[p.TapeID]
	if !ok {
		return nil, rpcError(serverError, codeTapeNotFound, "開いていないテープ: "+p.TapeID)
	}
	i := *p.Index
	if i < -1 || i >= len(t.frames) {
		return nil, invalidParams("index が範囲外: " + itoa(i))
	}

	res := struct {
		After   string  `json:"after"`
		Before  string  `json:"before"`
		Content *string `json:"content"`
	}{}
	file := p.File
	if i >= 0 {
		res.Before, res.After = t.frames[i].Before, t.frames[i].After
		if file == "" {
			file = t.frames[i].File
		}
	}
	if file != "" {
		if text, ok := timeline.ContentAt(t.frames, file, i); ok {
			res.Content = &text
		}
	}
	return res, nil
}
