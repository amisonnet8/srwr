// Package mcp is the MCP server of srwr: initialize, ping, tools/list and tools/call over
// newline-delimited JSON-RPC, with no SDK. What the tools do is in internal/core.
package mcp

import (
	"bytes"
	"encoding/json"
	"io"
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
	case tools.Replace:
		return s.callReplace(p.Arguments), nil
	case tools.New:
		return s.callNew(p.Arguments), nil
	}
	return nil, &jsonrpc.Error{Code: jsonrpc.InvalidParams, Message: "unknown tool: " + p.Name}
}

type lookArgs struct {
	File      *string `json:"file"`
	StartLine *int    `json:"startLine"`
	EndLine   *int    `json:"endLine"`
	Expect    *string `json:"expect"`
	Why       *string `json:"why"`
}

type lookOK struct {
	OK        bool     `json:"ok"`
	Selection string   `json:"selection"`
	StartLine int      `json:"startLine"`
	EndLine   int      `json:"endLine"`
	Lines     []string `json:"lines"`
}

func (s *Server) callLook(raw json.RawMessage) toolResult {
	var a lookArgs
	if err := decodeArgs(raw, &a); err != nil {
		s.Core.RecordInputFailure(tools.Look, err.Code, err.Message)
		return failure(*err)
	}
	if missing := firstMissing(map[string]bool{"file": a.File == nil, "why": a.Why == nil}, "file", "why"); missing != "" {
		return s.rejectLook("missing required input: " + missing)
	}
	// The line numbers go together; with none of them, expect says where the range is.
	switch {
	case (a.StartLine == nil) != (a.EndLine == nil):
		return s.rejectLook("give both startLine and endLine, or neither")
	case a.StartLine == nil && a.Expect == nil:
		return s.rejectLook("missing required input: startLine and endLine (or expect, to find the range by its content)")
	}
	in := core.LookInput{File: *a.File, Why: *a.Why, Expect: a.Expect, Locate: a.StartLine == nil}
	if !in.Locate {
		in.StartLine, in.EndLine = *a.StartLine, *a.EndLine
	}
	res, cerr := s.Core.Look(in)
	if cerr != nil {
		return failure(*cerr)
	}
	return success(lookOK{OK: true, Selection: res.Selection, StartLine: res.StartLine, EndLine: res.EndLine, Lines: res.Lines})
}

// rejectLook turns a look away for its input, and records that on the tape.
func (s *Server) rejectLook(message string) toolResult {
	e := core.Error{Code: core.CodeInvalidInput, Message: message}
	s.Core.RecordInputFailure(tools.Look, e.Code, e.Message)
	return failure(e)
}

type editArgs struct {
	Selection *string `json:"selection"`
	NewText   *string `json:"newText"`
	Why       *string `json:"why"`
}

type editOK struct {
	OK        bool     `json:"ok"`
	Selection string   `json:"selection"`
	StartLine int      `json:"startLine"`
	EndLine   int      `json:"endLine"`
	Lines     []string `json:"lines"`
	Before    []string `json:"before"`
	After     []string `json:"after"`
}

func (s *Server) callEdit(raw json.RawMessage) toolResult {
	var a editArgs
	if err := decodeArgs(raw, &a); err != nil {
		s.Core.RecordInputFailure(tools.Edit, err.Code, err.Message)
		return failure(*err)
	}
	if missing := firstMissing(map[string]bool{"selection": a.Selection == nil, "newText": a.NewText == nil, "why": a.Why == nil},
		"selection", "newText", "why"); missing != "" {
		e := core.Error{Code: core.CodeInvalidInput, Message: "missing required input: " + missing}
		s.Core.RecordInputFailure(tools.Edit, e.Code, e.Message)
		return failure(e)
	}
	res, cerr := s.Core.Edit(core.EditInput{Selection: *a.Selection, NewText: *a.NewText, Why: *a.Why})
	if cerr != nil {
		return failure(*cerr)
	}
	return success(editOK{OK: true, Selection: res.Selection, StartLine: res.StartLine, EndLine: res.EndLine,
		Lines: res.Lines, Before: res.Before, After: res.After})
}

type replaceArgs struct {
	Files *[]string `json:"files"`
	Old   *string   `json:"old"`
	New   *string   `json:"new"`
	Count *int      `json:"count"`
	Why   *string   `json:"why"`
}

type replaceOK struct {
	OK    bool            `json:"ok"`
	Count int             `json:"count"`
	Files []replaceFileOK `json:"files"`
}

type replaceFileOK struct {
	File      string   `json:"file"`
	Hits      int      `json:"hits"`
	StartLine int      `json:"startLine"`
	EndLine   int      `json:"endLine"`
	Selection string   `json:"selection"`
	Lines     []string `json:"lines"`
}

func (s *Server) callReplace(raw json.RawMessage) toolResult {
	var a replaceArgs
	if err := decodeArgs(raw, &a); err != nil {
		s.Core.RecordInputFailure(tools.Replace, err.Code, err.Message)
		return failure(*err)
	}
	if missing := firstMissing(map[string]bool{"files": a.Files == nil, "old": a.Old == nil, "new": a.New == nil, "count": a.Count == nil, "why": a.Why == nil},
		"files", "old", "new", "count", "why"); missing != "" {
		e := core.Error{Code: core.CodeInvalidInput, Message: "missing required input: " + missing}
		s.Core.RecordInputFailure(tools.Replace, e.Code, e.Message)
		return failure(e)
	}
	res, cerr := s.Core.Replace(core.ReplaceInput{Files: *a.Files, Old: *a.Old, New: *a.New, Count: *a.Count, Why: *a.Why})
	if cerr != nil {
		return failure(*cerr)
	}
	out := replaceOK{OK: true, Count: res.Count, Files: []replaceFileOK{}}
	for _, f := range res.Files {
		out.Files = append(out.Files, replaceFileOK{File: f.File, Hits: f.Hits, StartLine: f.StartLine, EndLine: f.EndLine, Selection: f.Selection, Lines: f.Lines})
	}
	return success(out)
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
		return &core.Error{Code: core.CodeInvalidInput, Message: "input has the wrong type: " + err.Error()}
	}
	return nil
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
}

func failure(e core.Error) toolResult {
	body := errorBody{Error: errorInfo{Code: e.Code, Message: e.Message, Actual: e.Actual}}
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
