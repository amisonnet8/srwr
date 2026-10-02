package tape

import "strings"

// File is what the tape knows about one file.
type File struct {
	Text    string
	Deleted bool // the last word on the file was an external change that removed it
}

// State is what can be worked out from a tape alone. It is built only from events, never kept
// in memory between calls: another process may have written more (.claude/rules/go-code.md).
type State struct {
	LastSeq int
	// Files holds every file the tape has a snapshot of.
	Files map[string]*File
	// Replaces lists the replace events in order, for line number correction.
	Replaces []Event
}

// NewState returns an empty state.
func NewState() *State { return &State{Files: map[string]*File{}} }

// Build returns the state after all events.
func Build(events []Event) *State {
	s := NewState()
	for _, e := range events {
		s.Apply(e)
	}
	return s
}

// Apply adds one event, which must come after those already applied.
func (s *State) Apply(e Event) {
	s.LastSeq = max(s.LastSeq, e.Seq)
	switch e.Type {
	case TypeSnapshot:
		s.Files[e.File] = &File{Text: deref(e.Text)}
	case TypeReplace:
		f := s.Files[e.File]
		if f == nil {
			f = &File{}
			s.Files[e.File] = f
		}
		f.Text = SpliceLines(f.Text, e.StartLine, e.EndLine, NewLines(e))
		s.Replaces = append(s.Replaces, e)
	case TypeExternal:
		switch {
		case e.Deleted:
			s.Files[e.File] = &File{Deleted: true}
		case e.Text != nil:
			s.Files[e.File] = &File{Text: *e.Text}
		}
		// The old format has no text: the snapshot that follows gives the new content.
	}
}

// NewLines returns the lines a replace wrote. NewText is the lines joined by "\n", which cannot say
// by itself whether it ends in a blank line, so the count comes from newStartLine and newEndLine.
// A tape without those, or whose count does not fit NewText (an old one that ends the text with
// a newline), is read the way mcp.md counts the lines of newText.
func NewLines(e Event) []string {
	if e.NewStartLine == 0 && e.NewEndLine == 0 {
		return Lines(e.NewText)
	}
	n := e.NewEndLine - e.NewStartLine + 1
	if n <= 0 {
		return nil
	}
	if parts := strings.Split(e.NewText, "\n"); len(parts) == n {
		return parts
	}
	return Lines(e.NewText)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
