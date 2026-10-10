package tape

import (
	"bytes"
	"encoding/json"
)

// Result is what Parse found in a chunk of a tape.
type Result struct {
	Events []Event
	// Consumed is the number of bytes up to and including the last newline. A reader that
	// follows a growing tape continues from here: the rest is a line still being written.
	Consumed int
	// Skipped counts complete lines that could not be read as an event.
	Skipped int
}

// Parse reads the events in data, which starts at the beginning of a line.
//
// It is forgiving, because tapes are shared and may come from other versions: a line that is
// not JSON, has an unknown type, or lacks a field the type needs is skipped; a field of the
// wrong type is read as null; fields it does not know are ignored.
func Parse(data []byte) Result {
	end := bytes.LastIndexByte(data, '\n') + 1
	res := Result{Consumed: end}
	for line := range bytes.SplitSeq(data[:end], []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if e, ok := parseLine(line); ok {
			res.Events = append(res.Events, e)
		} else {
			res.Skipped++
		}
	}
	return res
}

func parseLine(line []byte) (Event, bool) {
	var m map[string]json.RawMessage
	if json.Unmarshal(line, &m) != nil || m == nil {
		return Event{}, false
	}
	typ, _ := getString(m, "type")
	// Version 1 had select and replace only. A replace of version 1 is an edit, unless its tool says it was made by new (sub is an edit too).
	v, _ := getInt(m, "v")
	oldTool, _ := getString(m, "tool")
	if v < 2 {
		typ = fromVersion1(typ, oldTool)
	}
	if typ == "replace" {
		typ = TypeEdit // the replace tool is gone: its events (version 2) read as edits
	}
	e := Event{Type: typ}
	e.TS, _ = getString(m, "ts")

	if typ == TypeHeader {
		e.Session, _ = getString(m, "session")
		e.StartedAt, _ = getString(m, "startedAt")
		e.Author = getAuthor(m)
		e.VCS = m["vcs"]
		e.Title, _ = getString(m, "title")
		e.Why = getNullableString(m, "why")
		var tool ToolInfo
		if json.Unmarshal(m["tool"], &tool) == nil && tool != (ToolInfo{}) {
			e.Tool = &tool
		}
		return e, true
	}

	var ok bool
	if e.Seq, ok = getInt(m, "seq"); !ok {
		return Event{}, false
	}
	if typ == TypeFailure {
		f := &FailureInfo{File: getNullableString(m, "file"), Selection: getNullableString(m, "selection"), Why: getNullableString(m, "why")}
		f.Tool = oldTool
		if v < 2 {
			f.Tool = toolFromVersion1(oldTool)
		}
		if f.Tool == "replace" {
			f.Tool = TypeEdit
		}
		f.Code, _ = getString(m, "code")
		f.Message, _ = getString(m, "message")
		if has(m, "startLine") {
			n, _ := getInt(m, "startLine")
			f.StartLine = &n
		}
		if has(m, "endLine") {
			n, _ := getInt(m, "endLine")
			f.EndLine = &n
		}
		e.Failure = f
		return e, true
	}
	if e.File, ok = getString(m, "file"); !ok {
		return Event{}, false
	}
	switch typ {
	case TypeSnapshot:
		text, ok := getString(m, "text")
		if !ok {
			return Event{}, false
		}
		e.Text = &text
		e.FileHash, _ = getString(m, "fileHash")
		e.Sha, _ = getString(m, "sha")
	case TypeLook, TypeEdit, TypeNew:
		if !getRange(m, &e) {
			return Event{}, false
		}
		e.Why = getNullableString(m, "why")
		e.Selection = getNullableString(m, "selection")
		e.Source, _ = getString(m, "source")
		e.HookTool = oldTool
		if v < 2 && (oldTool == "sub" || oldTool == "new") {
			e.HookTool = "" // the tool of version 1 said which kind it was
		}
		if Changes(typ) {
			if e.NewText, ok = getString(m, "newText"); !ok {
				return Event{}, false
			}
			e.From = getNullableString(m, "from")
			e.OldText, _ = getString(m, "oldText")
			e.NewStartLine, _ = getInt(m, "newStartLine")
			e.NewEndLine, _ = getInt(m, "newEndLine")
			e.FileShaBefore, _ = getString(m, "fileShaBefore")
			e.FileShaAfter, _ = getString(m, "fileShaAfter")
		}
	case TypeExternal:
		e.Author = getAuthor(m)
		e.Text = getNullableString(m, "text")
		if has(m, "hunks") {
			var hs []Hunk
			if json.Unmarshal(m["hunks"], &hs) != nil {
				return Event{}, false
			}
			e.Hunks = hs
		}
		e.DetectedBy, _ = getString(m, "detectedBy")
		e.ExpectedSha, _ = getString(m, "expectedSha")
		e.ActualSha, _ = getString(m, "actualSha")
		e.Deleted, _ = getBool(m, "deleted")
		e.Created, _ = getBool(m, "created")
	default:
		return Event{}, false
	}
	return e, true
}

// fromVersion1 is the type a version 1 event has now.
func fromVersion1(typ, tool string) string {
	switch typ {
	case "select":
		return TypeLook
	case "replace":
		switch tool {
		case "new":
			return TypeNew
		}
		return TypeEdit
	}
	return typ
}

// toolFromVersion1 is the name a version 1 failure gave the tool that failed.
func toolFromVersion1(tool string) string {
	switch tool {
	case "select":
		return TypeLook
	case "replace", "sub":
		return TypeEdit
	}
	return tool
}

func getRange(m map[string]json.RawMessage, e *Event) bool {
	var ok1, ok2 bool
	e.StartLine, ok1 = getInt(m, "startLine")
	e.EndLine, ok2 = getInt(m, "endLine")
	return ok1 && ok2
}

// has reports whether a field is there and not null. json.Unmarshal accepts null for any type and leaves the zero value.
func has(m map[string]json.RawMessage, k string) bool {
	raw := m[k]
	return len(raw) > 0 && string(raw) != "null"
}

func getString(m map[string]json.RawMessage, k string) (string, bool) {
	var s string
	if !has(m, k) || json.Unmarshal(m[k], &s) != nil {
		return "", false
	}
	return s, true
}

// getNullableString reads null, a missing field and a value of the wrong type as nil.
func getNullableString(m map[string]json.RawMessage, k string) *string {
	s, ok := getString(m, k)
	if !ok {
		return nil
	}
	return &s
}

func getInt(m map[string]json.RawMessage, k string) (int, bool) {
	var n int
	if !has(m, k) || json.Unmarshal(m[k], &n) != nil {
		return 0, false
	}
	return n, true
}

func getBool(m map[string]json.RawMessage, k string) (bool, bool) {
	var b bool
	if !has(m, k) || json.Unmarshal(m[k], &b) != nil {
		return false, false
	}
	return b, true
}

func getAuthor(m map[string]json.RawMessage) *Author {
	var a Author
	if json.Unmarshal(m["author"], &a) != nil || a == (Author{}) {
		return nil
	}
	return &a
}
