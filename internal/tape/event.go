// Package tape reads and writes tapes (docs/reference/tape.md): append-only JSONL, one event per line.
//
// Everything that touches the tape format lives here, so no other package builds tape JSON by hand.
package tape

import "encoding/json"

// Event types.
const (
	TypeHeader   = "header"
	TypeSnapshot = "snapshot"
	TypeLook     = "look"
	TypeEdit     = "edit"
	TypeNew      = "new"
	TypeExternal = "external"
	TypeFailure  = "failure"
)

// Sources of a look or an edit.
const (
	SourceMCP  = "mcp"
	SourceHook = "hook"
)

// Version is the tape format version written in every event ("v"). Version 1 had select and replace (sub and new were a replace
// with a tool), and version 2 had a replace tool too; Parse reads them as look, edit and new.
const Version = 2

// Author says who made a change.
type Author struct {
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
}

// ToolInfo is the "tool" of a header.
type ToolInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Event is one line of a tape. Which fields are used depends on Type:
//
//	header:   Session, StartedAt, Author, VCS, Tool, and Title and Why when the AI started the session with the session tool
//	snapshot: Seq, TS, File, FileHash, Text (never nil), Sha
//	look:     Seq, TS, File, StartLine, EndLine, Why, Selection, Source, HookTool
//	edit, new:
//	          Seq, TS, File, From, StartLine, EndLine, OldText, NewText, NewStartLine, NewEndLine,
//	          Selection, Why, FileShaBefore, FileShaAfter, Source, HookTool
//	failure:  Seq, TS, Failure
//	external: Seq, TS, File, Author, DetectedBy, ExpectedSha, ActualSha, Hunks or Text (both nil when unknown), Created, Deleted
//
// A field that is null on the tape is nil here. TS is kept as the text on the tape, because
// a reader has to cope with a value it cannot parse.
type Event struct {
	Type string
	Seq  int
	TS   string

	// failure
	Failure *FailureInfo

	// header
	Session   string
	StartedAt string
	VCS       json.RawMessage // null, or {"type":"git","head":…,"dirty":…} (internal/vcs)
	Tool      *ToolInfo
	Title     string // header only: the title the AI gave with the session tool; "" when there is none (then Why is nil too)

	// Author is also set on external events.
	Author *Author

	File string

	// snapshot
	FileHash string
	Sha      string

	// snapshot and external: the whole file. nil on an external means it has Hunks, or (the old format) no text.
	Text *string

	// external: the lines that changed, against the content the tape held before. nil when Text is written.
	Hunks []Hunk

	// look, edit, replace and new
	StartLine int
	EndLine   int
	Why       *string
	Selection *string
	From      *string
	Source    string // "" is read as "mcp"
	HookTool  string // the original tool name of a hook event, such as "Read" or "Edit"

	// edit, replace and new
	OldText       string
	NewText       string
	NewStartLine  int
	NewEndLine    int
	FileShaBefore string
	FileShaAfter  string

	// external
	DetectedBy  string
	ExpectedSha string
	ActualSha   string
	Deleted     bool
	Created     bool // a new file: the tape held nothing of it, and Hunks (or Text) is the whole file
}

// FailureInfo is what a failure event holds: a look, edit or new that gave the AI an error. A value that is not there is nil
// (null on the tape). File is nil when it is not known or is left out, since the real path of an absolute path is not written.
type FailureInfo struct {
	Tool      string
	File      *string
	StartLine *int
	EndLine   *int
	Selection *string
	Why       *string
	Code      string
	Message   string
}

// Changes reports whether the event of this type changes a file: edit and new.
func Changes(typ string) bool { return typ == TypeEdit || typ == TypeNew }

// Str returns a pointer to s, for the nullable fields.
func Str(s string) *string { return &s }
