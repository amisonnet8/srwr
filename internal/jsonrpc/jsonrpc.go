// Package jsonrpc reads and writes newline-delimited JSON-RPC 2.0 over a byte stream.
// srwr mcp and srwr view-server share it, so the framing lives in one place.
package jsonrpc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

// Error codes defined by JSON-RPC 2.0.
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// Request is a message from the client. A request without an ID is a notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification reports whether no response is wanted.
func (r Request) IsNotification() bool { return len(r.ID) == 0 }

// Error is the error object of a response.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string { return e.Message }

// Response is a message to the client.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Handler answers one request. It is called one request at a time, in the order they arrive.
// The result must not be nil unless the error is set.
type Handler func(req Request) (any, *Error)

// Writer writes one message per line. It is safe for use by several goroutines.
type Writer struct {
	mu sync.Mutex
	w  io.Writer
}

// NewWriter returns a Writer that writes to w.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// Write encodes v as one line and writes it with a single call to the underlying writer.
func (w *Writer) Write(v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// The text of source files goes through here; <, > and & should stay readable.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.w.Write(buf.Bytes())
	return err
}

// Serve reads requests from r until it ends and answers each one through out.
// It returns nil when r ends, and an error only when reading or writing fails.
func Serve(r io.Reader, out *Writer, h Handler) error {
	br := bufio.NewReader(r)
	for {
		// ReadBytes has no limit on the length of a line, which a request with a whole file in it needs.
		line, readErr := br.ReadBytes('\n')
		if err := handleLine(bytes.TrimSpace(line), out, h); err != nil {
			return err
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

func handleLine(line []byte, out *Writer, h Handler) error {
	if len(line) == 0 {
		return nil
	}
	if line[0] == '[' {
		// Batches are not supported (they were dropped from MCP in 2025-06-18).
		return reply(out, nil, nil, &Error{Code: InvalidRequest, Message: "batch requests are not supported"})
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return reply(out, nil, nil, &Error{Code: ParseError, Message: "parse error: " + err.Error()})
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		return reply(out, req.ID, nil, &Error{Code: InvalidRequest, Message: "not a JSON-RPC 2.0 request"})
	}
	result, rpcErr := h(req)
	if req.IsNotification() {
		return nil
	}
	return reply(out, req.ID, result, rpcErr)
}

func reply(out *Writer, id json.RawMessage, result any, rpcErr *Error) error {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	resp := Response{JSONRPC: "2.0", ID: id}
	switch {
	case rpcErr != nil:
		resp.Error = rpcErr
	case result == nil:
		resp.Result = struct{}{}
	default:
		resp.Result = result
	}
	return out.Write(resp)
}
