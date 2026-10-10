package core

import (
	"strings"
	"unicode/utf8"
)

// MaxTitle is the longest title of a session, in characters.
const MaxTitle = 80

// SessionInput is the input of Session.
type SessionInput struct {
	Title string
	Why   string
}

// SessionResult says which tape the session writes to now and which one it closed ("" when none was open).
type SessionResult struct {
	TapeID string
	Closed string
}

// Session ends the current session and starts the next one with a title and a why. It touches no file of the workspace and
// writes nothing but the header of the new tape; a failure is not recorded on the tape (a failure is about a file).
func (c *Core) Session(in SessionInput) (*SessionResult, *Error) {
	title := strings.TrimSpace(in.Title)
	switch {
	case title == "":
		return nil, newError(CodeInvalidInput, "title is required (blank is not allowed). Give a short name of the work you are starting")
	case strings.ContainsAny(title, "\r\n"):
		return nil, newError(CodeInvalidInput, "title must be one line")
	case utf8.RuneCountInString(title) > MaxTitle:
		return nil, newError(CodeInvalidInput, "title is too long (at most 80 characters). Put the detail in why")
	}
	if err := checkWhy(in.Why); err != nil {
		return nil, err
	}
	id, closed, err := c.WS.StartSession(title, strings.TrimSpace(in.Why))
	if err != nil {
		return nil, internal(err)
	}
	return &SessionResult{TapeID: id, Closed: closed}, nil
}
