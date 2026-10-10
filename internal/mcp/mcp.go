// Package mcp is the MCP server of srwr: initialize, ping, tools/list and tools/call over
// newline-delimited JSON-RPC, with no SDK. What the tools do is in internal/core.
package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/amisonnet8/srwr/internal/core"
	"github.com/amisonnet8/srwr/internal/jsonrpc"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/tools"
)

// supportedVersions are the protocol versions srwr answers to, newest first.
var supportedVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// Server answers MCP requests for one workspace.
type Server struct {
	Core    *core.Core
	Version string // the srwr version, for serverInfo
}

// Serve reads requests from r and writes responses to w until r ends.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	return jsonrpc.Serve(r, jsonrpc.NewWriter(w), s.Handle)
}

// Handle answers one request.
func (s *Server) Handle(req jsonrpc.Request) (any, *jsonrpc.Error) {
	switch {
	case req.Method == "initialize":
		return s.initialize(req.Params), nil
	case req.Method == "ping":
		return struct{}{}, nil
	case req.Method == "tools/list":
		return struct {
			Tools []tools.Tool `json:"tools"`
		}{tools.List()}, nil
	case req.Method == "tools/call":
		return s.call(req.Params)
	case strings.HasPrefix(req.Method, "notifications/"):
		return struct{}{}, nil // nothing to do; a notification gets no response anyway
	}
	return nil, &jsonrpc.Error{Code: jsonrpc.MethodNotFound, Message: "method not found: " + req.Method}
}

// The order of the fields is the order they are written in, and docs/examples/look-edit.md shows it.
type initializeResult struct {
	Capabilities    map[string]any `json:"capabilities"`
	ProtocolVersion string         `json:"protocolVersion"`
	ServerInfo      serverInfo     `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (s *Server) initialize(params json.RawMessage) initializeResult {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
		ClientInfo      struct {
			Name string `json:"name"`
		} `json:"clientInfo"`
	}
	_ = json.Unmarshal(params, &p) // a client that sends something else still gets an answer

	version := supportedVersions[0]
	if slices.Contains(supportedVersions, p.ProtocolVersion) {
		version = p.ProtocolVersion
	}
	// Who the header of a new tape says wrote it: the client that started this server.
	if name := strings.TrimSpace(p.ClientInfo.Name); name != "" {
		s.Core.WS.SetAuthor(tape.Author{Kind: "ai", Name: truncate(name, 64)})
	}
	return initializeResult{
		Capabilities:    map[string]any{"tools": map[string]any{}},
		ProtocolVersion: version,
		ServerInfo:      serverInfo{Name: "srwr", Version: s.Version},
	}
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

type toolResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError"`
}

// textContent has text before type, as docs/examples/look-edit.md shows.
type textContent struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

func (s *Server) call(params json.RawMessage) (any, *jsonrpc.Error) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &jsonrpc.Error{Code: jsonrpc.InvalidParams, Message: "invalid params: " + err.Error()}
	}
	switch p.Name {
	case tools.Look:
		return s.callLook(p.Arguments), nil
	case tools.Edit:
		return s.callEdit(p.Arguments), nil
	case tools.New:
		return s.callNew(p.Arguments), nil
	}
	return nil, &jsonrpc.Error{Code: jsonrpc.InvalidParams, Message: "unknown tool: " + p.Name}
}

type lookArgs struct {
	File      *string         `json:"file"`
	StartLine *int            `json:"startLine"`
	EndLine   *int            `json:"endLine"`
	Expect    *string         `json:"expect"`
	Search    *string         `json:"search"`
	Include   *patterns       `json:"include"`
	Exclude   *patterns       `json:"exclude"`
	Offset    *int            `json:"offset"`
	Looks     *[]lookItemArgs `json:"looks"`
	Why       *string         `json:"why"`
}

// patterns is a list of patterns for include and exclude. One string is taken as a list of one: the AI often writes it that way, and
// there is nothing to guess.
type patterns []string

func (p *patterns) UnmarshalJSON(b []byte) error {
	var list []string
	if err := json.Unmarshal(b, &list); err == nil {
		*p = list
		return nil
	}
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*p = patterns{one}
		return nil
	}
	// The field name is not known here, so the message names both inputs that take patterns.
	return &json.UnmarshalTypeError{Value: jsonKind(b), Type: reflect.TypeOf([]string{}), Field: "include or exclude"}
}

// jsonKind names the kind of a JSON value the way encoding/json does in its type errors.
func jsonKind(b []byte) string {
	switch t := bytes.TrimSpace(b); {
	case len(t) == 0:
		return "value"
	case t[0] == '{':
		return "object"
	case t[0] == '[':
		return "array"
	case t[0] == 't' || t[0] == 'f':
		return "bool"
	case t[0] == '"':
		return "string"
	}
	return "number"
}

// lookItemArgs is one of the looks of a look call with looks.
type lookItemArgs struct {
	File      *string `json:"file"`
	StartLine *int    `json:"startLine"`
	EndLine   *int    `json:"endLine"`
}

type looksOK struct {
	OK    bool          `json:"ok"`
	Looks []looksItemOK `json:"looks"`
}

type looksItemOK struct {
	File      string   `json:"file"`
	Selection string   `json:"selection"`
	StartLine int      `json:"startLine"`
	EndLine   int      `json:"endLine"`
	Lines     []string `json:"lines"`
}

type lookOK struct {
	OK        bool     `json:"ok"`
	Selection string   `json:"selection"`
	StartLine int      `json:"startLine"`
	EndLine   int      `json:"endLine"`
	Lines     []string `json:"lines"`
	LineCount int      `json:"lineCount,omitempty"`
	Note      string   `json:"note,omitempty"`
}

func (s *Server) callLook(raw json.RawMessage) toolResult {
	var a lookArgs
	if err := decodeArgs(raw, &a); err != nil {
		s.Core.RecordInputFailure(tools.Look, err.Code, err.Message)
		return failure(*err)
	}
	if a.Looks != nil {
		return s.callLooks(a)
	}
	if missing := firstMissing(map[string]bool{"file": a.File == nil, "why": a.Why == nil}, "file", "why"); missing != "" {
		return s.rejectLook("missing required input: " + missing)
	}
	if a.Search != nil {
		return s.callSearch(a)
	}
	if a.Include != nil || a.Exclude != nil || a.Offset != nil {
		return s.rejectLook("include, exclude and offset go with search")
	}
	// The line numbers go together; with none of them, expect says where the range is.
	switch {
	case (a.StartLine == nil) != (a.EndLine == nil):
		return s.rejectLook("give both startLine and endLine, or neither")
	}
	// With no range at all, the whole file is read.
	in := core.LookInput{File: *a.File, Why: *a.Why, Expect: a.Expect, Locate: a.StartLine == nil && a.Expect != nil, Whole: a.StartLine == nil && a.Expect == nil}
	if a.StartLine != nil {
		in.StartLine, in.EndLine = *a.StartLine, *a.EndLine
	}
	res, cerr := s.Core.Look(in)
	if cerr != nil {
		return failure(*cerr)
	}
	return success(lookOK{OK: true, Selection: res.Selection, StartLine: res.StartLine, EndLine: res.EndLine, Lines: res.Lines, LineCount: res.LineCount, Note: res.Note})
}

type searchMatchOK struct {
	File      string   `json:"file,omitempty"`
	Selection string   `json:"selection"`
	StartLine int      `json:"startLine"`
	EndLine   int      `json:"endLine"`
	Lines     []string `json:"lines"`
	Above     []string `json:"above"`
	Below     []string `json:"below"`
	Cut       bool     `json:"cut,omitempty"`
}

type fileCountOK struct {
	File  string `json:"file"`
	Count int    `json:"count"`
}

type searchOK struct {
	OK      bool            `json:"ok"`
	Count   int             `json:"count"`
	Matches []searchMatchOK `json:"matches"`
	More    int             `json:"more,omitempty"`
	Note    string          `json:"note,omitempty"`
	ByFile  []fileCountOK   `json:"byFile,omitempty"`
}

// callSearch is look with search: the lines of the file that hold a text, each with a token.
func (s *Server) callSearch(a lookArgs) toolResult {
	if a.StartLine != nil || a.EndLine != nil || a.Expect != nil {
		return s.rejectLook("search is not given with startLine, endLine or expect: it finds the lines by itself")
	}
	in := core.SearchInput{File: *a.File, Search: *a.Search, Why: *a.Why}
	if a.Include != nil {
		in.Include = []string(*a.Include)
		for _, p := range in.Include {
			if strings.HasPrefix(p, "!") {
				return s.rejectLook(fmt.Sprintf("include %q starts with !, which include does not take. To leave files out, put the pattern in exclude (without the !)", p))
			}
		}
	}
	if a.Exclude != nil {
		in.Exclude = []string(*a.Exclude)
		if len(in.Exclude) > 0 && strings.HasPrefix(in.Exclude[0], "!") {
			return s.rejectLook(fmt.Sprintf("exclude %q starts with !: a ! pattern takes back an earlier pattern of exclude, so it cannot come first. To leave files out, write the pattern without the !", in.Exclude[0]))
		}
	}
	if a.Offset != nil {
		in.Offset = *a.Offset
	}
	res, cerr := s.Core.Search(in)
	if cerr != nil {
		return failure(*cerr)
	}
	out := searchOK{OK: true, Count: res.Count, Matches: make([]searchMatchOK, len(res.Matches)), More: max(res.Count-in.Offset-len(res.Matches), 0), Note: res.Note}
	for i, m := range res.Matches {
		out.Matches[i] = searchMatchOK{File: m.File, Selection: m.Selection, StartLine: m.StartLine, EndLine: m.EndLine, Lines: m.Lines, Above: m.Above, Below: m.Below, Cut: m.Cut}
	}
	for _, f := range res.ByFile {
		out.ByFile = append(out.ByFile, fileCountOK{File: f.File, Count: f.Count})
	}
	return success(out)
}

// callLooks is look with looks: several files in one call, each with a token.
func (s *Server) callLooks(a lookArgs) toolResult {
	switch {
	case a.Why == nil:
		return s.rejectLook("missing required input: why")
	case a.File != nil || a.StartLine != nil || a.EndLine != nil || a.Expect != nil || a.Search != nil || a.Include != nil || a.Exclude != nil || a.Offset != nil:
		return s.rejectLook("looks is not given with file, startLine, endLine, expect or search: give file (and the lines) in each item of looks")
	}
	in := core.LooksInput{Why: *a.Why}
	for i, it := range *a.Looks {
		switch {
		case it.File == nil:
			return s.rejectLook(fmt.Sprintf("looks[%d]: missing required input: file. Nothing was looked at: fix that item and send all the items again", i))
		case (it.StartLine == nil) != (it.EndLine == nil):
			return s.rejectLook(fmt.Sprintf("looks[%d]: give both startLine and endLine, or neither. Nothing was looked at: fix that item and send all the items again", i))
		}
		item := core.LooksItem{File: *it.File, HasLines: it.StartLine != nil}
		if item.HasLines {
			item.StartLine, item.EndLine = *it.StartLine, *it.EndLine
		}
		in.Items = append(in.Items, item)
	}
	res, cerr := s.Core.Looks(in)
	if cerr != nil {
		return failure(*cerr)
	}
	out := looksOK{OK: true, Looks: make([]looksItemOK, len(res.Items))}
	for i, r := range res.Items {
		out.Looks[i] = looksItemOK{File: r.File, Selection: r.Selection, StartLine: r.StartLine, EndLine: r.EndLine, Lines: r.Lines}
	}
	return success(out)
}

// rejectLook turns a look away for its input, and records that on the tape.
func (s *Server) rejectLook(message string) toolResult {
	e := core.Error{Code: core.CodeInvalidInput, Message: message}
	s.Core.RecordInputFailure(tools.Look, e.Code, e.Message)
	return failure(e)
}

type editArgs struct {
	Selection *string         `json:"selection"`
	File      *string         `json:"file"`
	StartLine *int            `json:"startLine"`
	EndLine   *int            `json:"endLine"`
	Expect    *string         `json:"expect"`
	NewText   *string         `json:"newText"`
	Insert    *string         `json:"insert"`
	Old       *string         `json:"old"`
	New       *string         `json:"new"`
	Edits     *[]editItemArgs `json:"edits"`
	Brief     *bool           `json:"brief"`
	Why       *string         `json:"why"`
}

// editItemArgs is one of the edits of an edit call with edits.
type editItemArgs struct {
	Selection *string `json:"selection"`
	File      *string `json:"file"`
	StartLine *int    `json:"startLine"`
	EndLine   *int    `json:"endLine"`
	Expect    *string `json:"expect"`
	NewText   *string `json:"newText"`
	Insert    *string `json:"insert"`
	Old       *string `json:"old"`
	New       *string `json:"new"`
	Content   *string `json:"content"` // not an input: only to turn it away with the way to make a file
	Why       *string `json:"why"`     // not used: the why of the call is the why of every item
}

type editsOK struct {
	OK    bool         `json:"ok"`
	Edits []editItemOK `json:"edits"`
	Note  string       `json:"note,omitempty"`
}

// itemWhyNote says which items of edits gave a why of their own, which is not used. It is empty when none did.
func itemWhyNote(items []editItemArgs) string {
	var idx []string
	for i, it := range items {
		if it.Why != nil {
			idx = append(idx, fmt.Sprintf("%d", i))
		}
	}
	if len(idx) == 0 {
		return ""
	}
	return fmt.Sprintf("why in edits[%s] was not used: the why of the call is the why of every edit", strings.Join(idx, "], edits["))
}

// editItemOK is the result of one of the edits of a call with edits.
type editItemOK struct {
	Selection string   `json:"selection"`
	StartLine int      `json:"startLine"`
	EndLine   int      `json:"endLine"`
	Lines     []string `json:"lines"`
	Above     []string `json:"above"`
	Below     []string `json:"below"`
	Cut       bool     `json:"cut,omitempty"`
	Hint      string   `json:"hint,omitempty"`
}

// briefOK is the result of an edit with brief: where the new range is and its token, without the lines.
type briefOK struct {
	Selection string `json:"selection"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Hint      string `json:"hint,omitempty"`
}

// briefEditsMin is how many edits in one call make the short result the default (brief: false asks for the long one).
const briefEditsMin = 6

type briefEditsOK struct {
	OK    bool      `json:"ok"`
	Edits []briefOK `json:"edits"`
	Note  string    `json:"note,omitempty"`
}

type briefEditOK struct {
	OK bool `json:"ok"`
	briefOK
}

type editOK struct {
	OK        bool     `json:"ok"`
	Selection string   `json:"selection"`
	StartLine int      `json:"startLine"`
	EndLine   int      `json:"endLine"`
	Lines     []string `json:"lines"`
	Above     []string `json:"above"`
	Below     []string `json:"below"`
	Cut       bool     `json:"cut,omitempty"`
	Hint      string   `json:"hint,omitempty"`
}

func (s *Server) callEdit(raw json.RawMessage) toolResult {
	var a editArgs
	if err := decodeArgs(raw, &a); err != nil {
		s.Core.RecordInputFailure(tools.Edit, err.Code, err.Message)
		return failure(*err)
	}
	if a.Edits != nil {
		return s.callEdits(a)
	}
	if a.Why == nil {
		return s.rejectEdit("missing required input: why")
	}
	in, msg := editInputOf(editItemArgs{Selection: a.Selection, File: a.File, StartLine: a.StartLine, EndLine: a.EndLine,
		Expect: a.Expect, NewText: a.NewText, Insert: a.Insert, Old: a.Old, New: a.New})
	if msg != "" {
		return s.rejectEdit(msg)
	}
	in.Why = *a.Why
	res, cerr := s.Core.Edit(in)
	if cerr != nil {
		return failure(*cerr)
	}
	if a.Brief != nil && *a.Brief {
		return success(briefEditOK{OK: true, briefOK: briefOK{Selection: res.Selection, StartLine: res.StartLine, EndLine: res.EndLine, Hint: res.Hint}})
	}
	return success(editOK{OK: true, Selection: res.Selection, StartLine: res.StartLine, EndLine: res.EndLine,
		Lines: res.Lines, Above: res.Above, Below: res.Below, Cut: res.Cut, Hint: res.Hint})
}

// callEdits is edit with edits: several edits in one call, with one why.
func (s *Server) callEdits(a editArgs) toolResult {
	if a.Why == nil {
		return s.rejectEdit("missing required input: why")
	}
	if a.Selection != nil || a.File != nil || a.StartLine != nil || a.EndLine != nil || a.Expect != nil || a.NewText != nil || a.Insert != nil || a.Old != nil || a.New != nil {
		return s.rejectEdit("with edits, give selection, file, startLine, endLine, expect, newText, insert, old and new inside each of the edits, not beside it")
	}
	in := core.EditsInput{Why: *a.Why, Edits: make([]core.EditInput, len(*a.Edits))}
	for i, item := range *a.Edits {
		e, msg := editInputOf(item)
		if msg != "" {
			return s.rejectEdit(fmt.Sprintf("edits[%d]: %s. Nothing was changed: fix that item and send all the items again", i, msg))
		}
		in.Edits[i] = e
	}
	res, cerr := s.Core.Edits(in)
	if cerr != nil {
		return failure(*cerr)
	}
	// Many edits are not read back one by one: from briefEditsMin items on, the short form is the default.
	if brief := len(res.Edits) >= briefEditsMin; (a.Brief == nil && brief) || (a.Brief != nil && *a.Brief) {
		b := briefEditsOK{OK: true, Edits: make([]briefOK, len(res.Edits)), Note: itemWhyNote(*a.Edits)}
		for i, r := range res.Edits {
			b.Edits[i] = briefOK{Selection: r.Selection, StartLine: r.StartLine, EndLine: r.EndLine, Hint: r.Hint}
		}
		return success(b)
	}
	out := editsOK{OK: true, Edits: make([]editItemOK, len(res.Edits)), Note: itemWhyNote(*a.Edits)}
	for i, r := range res.Edits {
		out.Edits[i] = editItemOK{Selection: r.Selection, StartLine: r.StartLine, EndLine: r.EndLine, Lines: r.Lines, Above: r.Above, Below: r.Below, Cut: r.Cut, Hint: r.Hint}
	}
	return success(out)
}

// editInputOf makes the input of one edit out of its arguments. msg is not empty when the arguments are not an edit.
func editInputOf(a editItemArgs) (in core.EditInput, msg string) {
	if a.Content != nil {
		return in, "content is not an input of edit: make a new file with new"
	}
	hasOld := a.Old != nil || a.New != nil
	switch {
	case hasOld && (a.Old == nil || a.New == nil):
		return in, "give old and new together"
	case hasOld && a.NewText != nil:
		return in, "give old and new, or newText; not both"
	case !hasOld && a.NewText == nil:
		return in, "missing required input: newText (or old and new)"
	case (a.StartLine == nil) != (a.EndLine == nil) && !hasOld:
		return in, "give both startLine and endLine, or neither"
	}
	in = core.EditInput{Expect: a.Expect, HasLines: a.StartLine != nil || a.EndLine != nil, Old: a.Old, New: a.New,
		OpenStart: hasOld && a.StartLine == nil && a.EndLine != nil, OpenEnd: hasOld && a.EndLine == nil && a.StartLine != nil}
	if a.NewText != nil {
		in.NewText = *a.NewText
	}
	if a.Selection != nil {
		in.Selection = *a.Selection
	}
	if a.File != nil {
		in.File = *a.File
	}
	if a.Insert != nil {
		in.Insert = *a.Insert
	}
	if a.StartLine != nil {
		in.StartLine = *a.StartLine
	}
	if a.EndLine != nil {
		in.EndLine = *a.EndLine
	}
	if a.Selection != nil && strings.TrimSpace(*a.Selection) != "" && (a.File != nil || in.HasLines || a.Expect != nil || hasOld) {
		return in, "give selection, or file with expect (or old and new); not both"
	}
	if a.Selection == nil && a.File == nil {
		return in, "missing required input: selection (from look), or file with expect (or old and new)"
	}
	return in, ""
}

// rejectEdit turns an edit away for its input, and records that on the tape.
func (s *Server) rejectEdit(message string) toolResult {
	e := core.Error{Code: core.CodeInvalidInput, Message: message}
	s.Core.RecordInputFailure(tools.Edit, e.Code, e.Message)
	return failure(e)
}

type newArgs struct {
	File    *string `json:"file"`
	Content *string `json:"content"`
	Why     *string `json:"why"`
}

type newOK struct {
	OK        bool   `json:"ok"`
	Selection string `json:"selection"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}

func (s *Server) callNew(raw json.RawMessage) toolResult {
	var a newArgs
	if err := decodeArgs(raw, &a); err != nil {
		s.Core.RecordInputFailure(tools.New, err.Code, err.Message)
		return failure(*err)
	}
	if missing := firstMissing(map[string]bool{"file": a.File == nil, "content": a.Content == nil, "why": a.Why == nil},
		"file", "content", "why"); missing != "" {
		e := core.Error{Code: core.CodeInvalidInput, Message: "missing required input: " + missing}
		s.Core.RecordInputFailure(tools.New, e.Code, e.Message)
		return failure(e)
	}
	res, cerr := s.Core.New(core.NewInput{File: *a.File, Content: *a.Content, Why: *a.Why})
	if cerr != nil {
		return failure(*cerr)
	}
	return success(newOK{OK: true, Selection: res.Selection, StartLine: res.StartLine, EndLine: res.EndLine})
}

// decodeArgs reads the arguments of a call. A value of the wrong type is invalid_input, like a missing one.
func decodeArgs(raw json.RawMessage, into any) *core.Error {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		raw = json.RawMessage("{}")
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return &core.Error{Code: core.CodeInvalidInput, Message: typeMessage(err)}
	}
	return nil
}

// typeMessage says what is wrong with an input that does not decode, in the names of the input (not of Go's types).
func typeMessage(err error) string {
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		field := ute.Field
		if field == "" {
			field = "the input"
		}
		want := "a value of another type"
		switch k := ute.Type; {
		case k == nil:
		case k.Kind() == reflect.Slice && k.Elem().Kind() == reflect.String:
			want = "an array of strings"
		case k.Kind() == reflect.Slice:
			want = "an array of objects"
		case k.Kind() == reflect.Struct || k.Kind() == reflect.Map:
			want = "an object"
		case k.Kind() == reflect.String:
			want = "a string"
		case k.Kind() == reflect.Bool:
			want = "true or false"
		case k.Kind() >= reflect.Int && k.Kind() <= reflect.Uint64:
			want = "an integer"
		}
		msg := fmt.Sprintf("%s must be %s, got %s.", field, want, ute.Value)
		if ute.Value == "string" && want == "true or false" {
			msg += " Write true or false without quotes"
		}
		if ute.Value == "string" && strings.HasPrefix(want, "an ") && !strings.HasSuffix(want, "integer") {
			msg += " Pass it as JSON, not as a string that holds JSON"
		}
		return msg
	}
	var se *json.SyntaxError
	if errors.As(err, &se) {
		return "the input is not valid JSON"
	}
	return "the input has the wrong type"
}

func firstMissing(missing map[string]bool, order ...string) string {
	for _, name := range order {
		if missing[name] {
			return name
		}
	}
	return ""
}

func success(v any) toolResult { return toolResult{Content: []textContent{text(v)}} }

type errorBody struct {
	OK    bool      `json:"ok"`
	Error errorInfo `json:"error"`
}

type errorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Actual  any    `json:"actual,omitempty"`

	NearMatches []core.NearMatch `json:"nearMatches,omitempty"`
	Retry       map[string]any   `json:"retry,omitempty"`
}

func failure(e core.Error) toolResult {
	body := errorBody{Error: errorInfo{Code: e.Code, Message: e.Message, Actual: e.Actual, NearMatches: e.NearMatches, Retry: e.Retry}}
	return toolResult{Content: []textContent{text(body)}, IsError: true}
}

// text encodes the body of a result: JSON in a string, as MCP wants it, with the source code in it left readable.
func text(v any) textContent {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) // the values are plain structs
	return textContent{Text: strings.TrimSuffix(buf.String(), "\n"), Type: "text"}
}
