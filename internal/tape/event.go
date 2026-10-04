// Package tape reads and writes tapes (docs/reference/tape.md): append-only JSONL, one event per line.
//
// Everything that touches the tape format lives here, so no other package builds tape JSON by hand.
package tape

import "encoding/json"

// Event types.
const (
	TypeHeader   = "header"
	TypeSnapshot = "snapshot"
	TypeSelect   = "select"
	TypeReplace  = "replace"
	TypeExternal = "external"
)

// Sources of a select or replace.
const (
	SourceMCP  = "mcp"
	SourceHook = "hook"
)

// Version is the tape format version written in every event ("v").
const Version = 1

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
//	header:   Session, StartedAt, Author, VCS, Tool
//	snapshot: Seq, TS, File, FileHash, Text (never nil), Sha
//	select:   Seq, TS, File, StartLine, EndLine, Why, Selection, Source, HookTool
//	replace:  Seq, TS, File, From, StartLine, EndLine, OldText, NewText, NewStartLine, NewEndLine,
//	          Selection, Why, FileShaBefore, FileShaAfter, Source, HookTool
//	external: Seq, TS, File, Author, DetectedBy, ExpectedSha, ActualSha, Hunks or Text (both nil when unknown), Created, Deleted
//
// A field that is null on the tape is nil here. TS is kept as the text on the tape, because
// a reader has to cope with a value it cannot parse.
type Event struct {
	Type string
	Seq  int
	TS   string

	// header
	Session   string
	StartedAt string
	VCS       json.RawMessage // null, or {"type":"git","head":…,"dirty":…} (internal/vcs)
	Tool      *ToolInfo

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

	// select and replace
	StartLine int
	EndLine   int
	Why       *string
	Selection *string
	From      *string
	Source    string // "" is read as "mcp"
	HookTool  string // the original tool name of a hook select, such as "Read"

	// replace
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

// Str returns a pointer to s, for the nullable fields.
func Str(s string) *string { return &s }
