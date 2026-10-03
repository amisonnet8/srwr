// Package viewserver is the display server (srwr view-server): it reads tapes and answers editors
// with frames and the text of each frame (docs/reference/protocol.md). It only reads: it takes no
// lock and writes no tape.
package viewserver

import (
	"encoding/json"
	"io"
	"path/filepath"
	"time"

	"github.com/amisonnet8/srwr/internal/jsonrpc"
	"github.com/amisonnet8/srwr/internal/timeline"
)

// ProtocolVersion is the protocolVersion of docs/reference/protocol.md.
const ProtocolVersion = 1

// DefaultPollInterval is how often a live view looks at the tape.
const DefaultPollInterval = 200 * time.Millisecond

// Codes of error.data.code, and the JSON-RPC code each comes with.
const (
	codeProtocolMismatch = "protocol_mismatch"
	codeNotInitialized   = "not_initialized"
	codeTapeNotFound     = "tape_not_found"
	codeTapeUnreadable   = "tape_unreadable"
	codeInvalidParams    = "invalid_params"

	serverError = -32000
)

// Server answers editors for one workspace.
type Server struct {
	Root         string
	Version      string
	PollInterval time.Duration // default DefaultPollInterval
}

func (s *Server) tapesDir() string { return filepath.Join(s.Root, ".srwr", "tapes") }

func (s *Server) poll() time.Duration {
	if s.PollInterval > 0 {
		return s.PollInterval
	}
	return DefaultPollInterval
}

// Serve talks to one client until its input ends or it sends shutdown. Everything the live view
// started is stopped before it returns.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	c := &conn{
		srv:    s,
		out:    jsonrpc.NewWriter(w),
		opened: map[string]*openTape{},
		cache:  map[string]listEntry{},
	}
	defer c.stopLive()
	return jsonrpc.Serve(r, c.out, c.handle)
}

// conn is the state of one client.
type conn struct {
	srv *Server
	out *jsonrpc.Writer

	initialized bool
	diffFrames  bool

	opened map[string]*openTape
	cache  map[string]listEntry
	live   *liveWatcher
}

// openTape is a tape the client has opened: the frames, for frame/state.
type openTape struct {
	frames []timeline.Frame
}

func rpcError(rpcCode int, code, message string) *jsonrpc.Error {
	return &jsonrpc.Error{Code: rpcCode, Message: message, Data: map[string]string{"code": code}}
}

func invalidParams(message string) *jsonrpc.Error {
	return rpcError(jsonrpc.InvalidParams, codeInvalidParams, message)
}

// decode reads the params, which are an object (or nothing at all, which is the same as {}).
func decode(raw json.RawMessage, into any) *jsonrpc.Error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return invalidParams("invalid parameters: " + err.Error())
	}
	return nil
}

func (c *conn) handle(req jsonrpc.Request) (any, *jsonrpc.Error) {
	switch req.Method {
	case "initialize":
		return c.initialize(req.Params)
	case "tapes/list", "tape/open", "frame/state", "tape/close", "live/start", "live/stop", "shutdown":
		if !c.initialized {
			return nil, rpcError(serverError, codeNotInitialized, "a request came before initialize: "+req.Method)
		}
	default:
		return nil, &jsonrpc.Error{Code: jsonrpc.MethodNotFound, Message: "method not found: " + req.Method}
	}
	switch req.Method {
	case "tapes/list":
		return c.tapesList()
	case "tape/open":
		return c.tapeOpen(req.Params)
	case "frame/state":
		return c.frameState(req.Params)
	case "tape/close":
		return c.tapeClose(req.Params)
	case "live/start":
		return c.liveStart(req.Params)
	case "live/stop":
		c.stopLive()
		return struct{}{}, nil
	default: // shutdown
		c.stopLive()
		return shutdownResult{}, nil
	}
}

// shutdownResult ends Serve once its response has been written.
type shutdownResult struct{}

func (shutdownResult) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }
func (shutdownResult) After() bool                  { return true }

type initializeResult struct {
	ProtocolVersion int    `json:"protocolVersion"`
	ServerVersion   string `json:"serverVersion"`
}

func (c *conn) initialize(raw json.RawMessage) (any, *jsonrpc.Error) {
	var p struct {
		Client          string `json:"client"`
		ProtocolVersion *int   `json:"protocolVersion"`
		Options         struct {
			DiffFrames *bool `json:"diffFrames"`
		} `json:"options"`
	}
	if err := decode(raw, &p); err != nil {
		return nil, err
	}
	if p.ProtocolVersion == nil {
		return nil, invalidParams("protocolVersion is missing")
	}
	if *p.ProtocolVersion != ProtocolVersion {
		return nil, rpcError(serverError, codeProtocolMismatch, "srwr and the editor side do not match (the server is protocolVersion "+itoa(ProtocolVersion)+")")
	}
	c.initialized = true
	c.diffFrames = p.Options.DiffFrames == nil || *p.Options.DiffFrames
	return initializeResult{ProtocolVersion: ProtocolVersion, ServerVersion: c.srv.Version}, nil
}

// wireFrame is a frame as it is sent. The text is there only when the client asked for it.
type wireFrame struct {
	timeline.Frame
	Before *string `json:"before,omitempty"`
	After  *string `json:"after,omitempty"`
}

func wire(f timeline.Frame, withText bool) wireFrame {
	w := wireFrame{Frame: f}
	if withText {
		w.Before, w.After = &f.Before, &f.After
	}
	return w
}

func wireAll(frames []timeline.Frame, withText bool) []wireFrame {
	out := make([]wireFrame, 0, len(frames))
	for _, f := range frames {
		out = append(out, wire(f, withText))
	}
	return out
}
