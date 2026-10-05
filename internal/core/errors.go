package core

import "fmt"

// Error codes of docs/reference/mcp.md. They go to the client as they are.
const (
	CodeInvalidSelection  = "invalid_selection"
	CodeSelectionStale    = "selection_stale"
	CodeSelectionMismatch = "selection_mismatch"
	CodeContentMismatch   = "content_mismatch"
	CodeContentNotFound   = "content_not_found"
	CodeContentAmbiguous  = "content_ambiguous"
	CodeCountMismatch     = "count_mismatch"
	CodeUseEdit           = "use_edit"
	CodeFileNotFound      = "file_not_found"
	CodeFileExists        = "file_exists"
	CodeInvalidRange      = "invalid_range"
	CodeIgnoredFile       = "ignored_file"
	CodeInvalidInput      = "invalid_input"
	CodeUnsupportedFile   = "unsupported_file"
	CodeInternalError     = "internal_error"
)

// Error is a failure the client is told about. Actual, when set, is the current content the
// client needs to try again; what it holds depends on the code (docs/reference/mcp.md).
type Error struct {
	Code    string
	Message string
	Actual  any
	// NearMatches are places that differ from expect (or old) only in spaces and tabs. Not on the tape.
	NearMatches []NearMatch
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func newError(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
