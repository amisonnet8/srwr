package core

import (
	"path/filepath"
	"strings"

	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/token"
)

// maxFailureMessage is where the message of a failure on the tape is cut.
const maxFailureMessage = 300

// failedCall is a select or replace that gave the client an error, as far as the tape wants to know it.
type failedCall struct {
	tool      string // toolSelect or toolReplace
	file      string // select: the path as given; "" when there is none
	startLine *int
	endLine   *int
	selection *string // replace: the token as given
	why       *string
	err       *Error
}

// Tools a failure is about.
const (
	toolSelect  = "select"
	toolReplace = "replace"
)

// recordFailure writes a failure event for a call that failed (docs/reference/tape.md). It never changes the answer the client
// gets: a failure to write is dropped. What goes on the tape leaves out the real path of an absolute path, of a path outside
// the workspace and of a file that is not recorded, and never has the new text of a replace.
func (c *Core) recordFailure(f failedCall) {
	_ = c.WS.Do(func(tx *session.Tx) error {
		return tx.Append(tape.Event{Type: tape.TypeFailure, Seq: tx.NextSeq(), Failure: f.info(tx)})
	})
}

// RecordInputFailure records a call that was turned away before it reached select or replace: a required input is missing, or a
// value has the wrong type. Nothing of the input but the error is known.
func (c *Core) RecordInputFailure(tool, code, message string) {
	c.recordFailure(failedCall{tool: tool, err: &Error{Code: code, Message: message}})
}

func (f failedCall) info(tx *session.Tx) *tape.FailureInfo {
	info := &tape.FailureInfo{
		Tool: f.tool, StartLine: f.startLine, EndLine: f.endLine, Selection: f.selection, Why: f.why,
		Code: f.err.Code, Message: f.err.Message,
	}
	var file string
	switch f.tool {
	case toolSelect:
		if f.file != "" {
			rel, perr := cleanPath(f.file)
			switch {
			case perr == nil:
				file = rel
			case perr.Code == CodeInvalidRange:
				if isAbsolute(f.file) {
					info.Message = "The path is absolute. Give a path relative to the workspace"
				} else {
					info.Message = "The path points outside the workspace"
				}
			}
		}
	case toolReplace:
		if f.selection != nil {
			if tok, err := token.Decode(*f.selection, tx.TapeID(), tx.Key()); err == nil {
				file, _ = findFile(tx.State(), tok.FileHash)
			}
		}
	}
	switch f.err.Code {
	case CodeIgnoredFile:
		file = ""
		info.Message = "The file is not recorded"
	case CodeInternalError:
		// A .srwrignore that cannot be read leaves us unable to say what is secret, and the system's message names real paths.
		file = ""
		info.Message = "An internal error happened"
	}
	if file != "" {
		info.File = &file
	}
	if r := []rune(info.Message); len(r) > maxFailureMessage {
		info.Message = string(r[:maxFailureMessage])
	}
	return info
}

// isAbsolute says whether a path a client gave is absolute on any system (a drive letter counts).
func isAbsolute(in string) bool {
	p := filepath.ToSlash(in)
	return strings.HasPrefix(p, "/") || (len(p) >= 2 && p[1] == ':')
}
