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

// The order of the fields is the order they are written in, and docs/examples/select-replace.md shows it.
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

// textContent has text before type, as docs/examples/select-replace.md shows.
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
	case tools.Select:
		return s.callSelect(p.Arguments), nil
	case tools.Replace:
		return s.callReplace(p.Arguments), nil
	}
	return nil, &jsonrpc.Error{Code: jsonrpc.InvalidParams, Message: "unknown tool: " + p.Name}
}

type selectArgs struct {
	File      *string `json:"file"`
	StartLine *int    `json:"startLine"`
	EndLine   *int    `json:"endLine"`
	Why       *string `json:"why"`
}

type selectOK struct {
	OK        bool     `json:"ok"`
	Selection string   `json:"selection"`
	Lines     []string `json:"lines"`
}

func (s *Server) callSelect(raw json.RawMessage) toolResult {
	var a selectArgs
	if err := decodeArgs(raw, &a); err != nil {
		return failure(*err)
	}
	if missing := firstMissing(map[string]bool{"file": a.File == nil, "startLine": a.StartLine == nil, "endLine": a.EndLine == nil, "why": a.Why == nil},
		"file", "startLine", "endLine", "why"); missing != "" {
		return failure(core.Error{Code: core.CodeInvalidInput, Message: "missing required input: " + missing})
	}
	res, cerr := s.Core.Select(core.SelectInput{File: *a.File, StartLine: *a.StartLine, EndLine: *a.EndLine, Why: *a.Why})
	if cerr != nil {
		return failure(*cerr)
	}
	return success(selectOK{OK: true, Selection: res.Selection, Lines: res.Lines})
}

type replaceArgs struct {
	Selection *string `json:"selection"`
	NewText   *string `json:"newText"`
	Why       *string `json:"why"`
}

type replaceOK struct {
	OK        bool   `json:"ok"`
	Selection string `json:"selection"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}

func (s *Server) callReplace(raw json.RawMessage) toolResult {
	var a replaceArgs
	if err := decodeArgs(raw, &a); err != nil {
		return failure(*err)
	}
	if missing := firstMissing(map[string]bool{"selection": a.Selection == nil, "newText": a.NewText == nil, "why": a.Why == nil},
		"selection", "newText", "why"); missing != "" {
		return failure(core.Error{Code: core.CodeInvalidInput, Message: "missing required input: " + missing})
	}
	res, cerr := s.Core.Replace(core.ReplaceInput{Selection: *a.Selection, NewText: *a.NewText, Why: *a.Why})
	if cerr != nil {
		return failure(*cerr)
	}
	return success(replaceOK{OK: true, Selection: res.Selection, StartLine: res.StartLine, EndLine: res.EndLine})
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
