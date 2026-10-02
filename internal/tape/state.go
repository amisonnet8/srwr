package tape

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
		f.Text = Splice(f.Text, e.StartLine, e.EndLine, e.NewText)
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

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
