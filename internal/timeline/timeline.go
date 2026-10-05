// Package timeline turns a tape into frames: the steps a viewer moves through, each with the text
// of its file before and after. It reads tapes and never writes them. The shape of a frame is the
// contract with the editors (docs/reference/protocol.md).
package timeline

import (
	"time"

	"github.com/amisonnet8/srwr/internal/tape"
)

// Kinds of frames.
const (
	KindSelect   = tape.TypeSelect
	KindReplace  = tape.TypeReplace
	KindExternal = tape.TypeExternal
	KindFinal    = "final"
	KindSub      = "sub" // a replace made by the sub tool: shown as a diff, with its reason
	KindNew      = "new" // a replace made by the new tool: the whole file, shown like a replace
	KindFailure  = "failure"
)

// Range is a range of lines, 1-based and inclusive. End < Start is an empty range.
type Range struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Frame is one step. The fields are written in the order docs/examples/protocol-session.md shows.
// Before and After are the whole file before and after the step; they are sent only when the
// client asks for text.
type Frame struct {
	Index     int     `json:"index"`
	Kind      string  `json:"kind"`
	Seq       int     `json:"seq"`
	TS        int64   `json:"ts"` // Unix milliseconds
	File      string  `json:"file"`
	Range     Range   `json:"range"`
	OldRange  *Range  `json:"oldRange,omitempty"`
	Why       *string `json:"why"`
	Selection *string `json:"selection"`
	From      *string `json:"from"`
	Parent    *int    `json:"parent"`
	Deleted   bool    `json:"deleted,omitempty"`
	Hits      int     `json:"hits,omitempty"` // sub frames only: how many places it changed in the file

	// failure frames only: the tool that failed, the error code, and the message (the real path is left out of the tape).
	Tool    string `json:"tool,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`

	Before string `json:"-"`
	After  string `json:"-"`
}

// Builder builds frames one event at a time. A tape that is read whole and a tape that grows
// while it is watched go the same way through it.
type Builder struct {
	state    *tape.State
	frames   []Frame
	files    []string
	seenFile map[string]bool
	ops      int
	lastTS   int64
	bySel    map[string]int // selection token -> index of the frame that issued it
	pending  int            // index of an external frame of the old format waiting for its snapshot, or -1
}

// NewBuilder returns an empty Builder.
func NewBuilder() *Builder {
	return &Builder{state: tape.NewState(), seenFile: map[string]bool{}, bySel: map[string]int{}, pending: -1}
}

// Build returns a Builder that has been given all of events.
func Build(events []tape.Event) *Builder {
	b := NewBuilder()
	for _, e := range events {
		b.Add(e)
	}
	return b
}

// Frames returns the frames so far.
func (b *Builder) Frames() []Frame { return b.frames }

// Files returns the files the frames are about, in the order they were first touched. A file the
// tape only has a snapshot of is not among them.
func (b *Builder) Files() []string { return b.files }

// Ops returns how many select, replace and external events there were.
func (b *Builder) Ops() int { return b.ops }

// State returns what the tape says of its files after the last event.
func (b *Builder) State() *tape.State { return b.state }

func (b *Builder) text(file string) string {
	if f := b.state.Files[file]; f != nil {
		return f.Text
	}
	return ""
}

// WholeRange returns the range of all the lines of text.
func WholeRange(text string) Range { return Range{Start: 1, End: len(tape.Lines(text))} }

// Add takes the next event of the tape. It reports whether that made a new frame, which is then the last of Frames.
func (b *Builder) Add(e tape.Event) bool {
	// An external event of the old format has no text: the snapshot right after it, of the same file, is the new content.
	if b.pending >= 0 {
		f := &b.frames[b.pending]
		if e.Type == tape.TypeSnapshot && e.File == f.File && e.Text != nil {
			f.After = *e.Text
			f.Range = WholeRange(f.After)
		}
		b.pending = -1
	}

	switch e.Type {
	case tape.TypeSnapshot:
		b.state.Apply(e)
		return false
	case tape.TypeFailure:
		// A failure is a frame, but not an operation: it has no file to open, so it is not counted in Ops or Files.
		b.frames = append(b.frames, failureFrame(len(b.frames), e, b.timestamp(e)))
		return true
	case tape.TypeSelect, tape.TypeReplace, tape.TypeExternal:
	default:
		return false
	}

	before := b.text(e.File)
	f := Frame{Index: len(b.frames), Kind: e.Type, Seq: e.Seq, TS: b.timestamp(e), File: e.File, Before: before}
	switch e.Type {
	case tape.TypeSelect:
		f.Range = Range{Start: e.StartLine, End: e.EndLine}
		f.After = before
		f.Why, f.Selection = cloneString(e.Why), cloneString(e.Selection)
		b.state.Apply(e)
	case tape.TypeReplace:
		f.OldRange = &Range{Start: e.StartLine, End: e.EndLine}
		f.Range = Range{Start: e.NewStartLine, End: e.NewEndLine}
		if e.NewStartLine == 0 && e.NewEndLine == 0 { // a tape without the new range
			f.Range = Range{Start: e.StartLine, End: e.StartLine + len(tape.NewLines(e)) - 1}
		}
		f.Why, f.Selection = cloneString(e.Why), cloneString(e.Selection)
		b.state.Apply(e)
		f.After = b.text(e.File)
		switch e.HookTool {
		case "sub":
			f.Kind, f.Hits = KindSub, e.Hits
		case "new":
			f.Kind = KindNew
		}
	case tape.TypeExternal:
		b.state.Apply(e)
		switch {
		case e.Deleted:
			f.Deleted = true
		case e.Hunks != nil:
			f.After = b.text(e.File)
		case e.Text != nil:
			f.After = *e.Text
		default:
			f.After = before
			b.pending = f.Index
		}
		f.Range = WholeRange(f.After)
	}
	f.From = cloneString(e.From)
	if f.From != nil {
		if i, ok := b.bySel[*f.From]; ok {
			f.Parent = &i
		}
	}
	if f.Selection != nil {
		b.bySel[*f.Selection] = f.Index
	}

	b.ops++
	if !b.seenFile[e.File] {
		b.seenFile[e.File] = true
		b.files = append(b.files, e.File)
	}
	b.frames = append(b.frames, f)
	return true
}

// timestamp reads the time of an event as Unix milliseconds. One that cannot be read gets the time
// of the frame before it, or 0 for the first.
func (b *Builder) timestamp(e tape.Event) int64 {
	if t, err := time.Parse(time.RFC3339, e.TS); err == nil {
		b.lastTS = t.UnixMilli()
	}
	return b.lastTS
}

func cloneString(s *string) *string {
	if s == nil {
		return nil
	}
	c := *s
	return &c
}

// AppendFinals adds the last diff to frames: for every file the frames are about, how the tape left it
// compared with how it is now. read returns the file as it is now, and whether it exists. Only the
// files that differ get a frame.
func AppendFinals(frames []Frame, state *tape.State, files []string, read func(rel string) (text string, exists bool)) []Frame {
	var seq int
	var ts int64
	if n := len(frames); n > 0 {
		seq, ts = frames[n-1].Seq, frames[n-1].TS
	}
	for _, file := range files {
		before, present := "", true // a file the tape has no content of counts as there, and empty
		if f := state.Files[file]; f != nil {
			before, present = f.Text, !f.Deleted
		}
		after, exists := read(file)
		if present == exists && before == after {
			continue
		}
		frames = append(frames, Frame{
			Index: len(frames), Kind: KindFinal, Seq: seq, TS: ts, File: file,
			Range: WholeRange(after), Deleted: !exists, Before: before, After: after,
		})
	}
	return frames
}

// ContentAt returns what file holds while frame i is shown: after the last frame at or before i that touched it,
// or, if none did yet, before the first frame after i that does. ok is false if no frame touches the file.
func ContentAt(frames []Frame, file string, i int) (text string, ok bool) {
	for j := min(i, len(frames)-1); j >= 0; j-- {
		if frames[j].File == file && frames[j].Kind != KindFailure { // a failure names a file but holds no text
			return frames[j].After, true
		}
	}
	for j := max(i+1, 0); j < len(frames); j++ {
		if frames[j].File == file && frames[j].Kind != KindFailure {
			return frames[j].Before, true
		}
	}
	return "", false
}

// failureFrame makes the frame of a failure event. File is "" when the tape left it out. Range is what a select was given, or
// an empty range at 0 when there is none.
func failureFrame(index int, e tape.Event, ts int64) Frame {
	f := Frame{Index: index, Kind: KindFailure, Seq: e.Seq, TS: ts, Range: Range{Start: 0, End: -1}}
	if fi := e.Failure; fi != nil {
		if fi.File != nil {
			f.File = *fi.File
		}
		if fi.StartLine != nil && fi.EndLine != nil {
			f.Range = Range{Start: *fi.StartLine, End: *fi.EndLine}
		}
		f.Why, f.Tool, f.Code, f.Message = cloneString(fi.Why), fi.Tool, fi.Code, fi.Message
	}
	return f
}
