package core

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

func (e *env) failures() []tape.FailureInfo {
	var got []tape.FailureInfo
	for _, ev := range e.events() {
		if ev.Type == tape.TypeFailure {
			got = append(got, *ev.Failure)
		}
	}
	return got
}

func TestFailureIsRecordedForEachCode(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n2\n3\n")
	e.write("crlf.txt", "a\r\nb\r\n")
	e.write(".env", secret)
	s := e.sel(e.c, "f.txt", 1, 1)
	e.rep(e.c, s.Selection, "one") // the token is stale after this one

	calls := []struct {
		name string
		do   func() *Error
		code string
		tool string
		file string // "" is null
	}{
		{"empty file", func() *Error {
			_, err := e.c.Look(LookInput{File: "", StartLine: 1, EndLine: 1, Why: "w"})
			return err
		}, CodeInvalidInput, "look", ""},
		{"blank why", func() *Error {
			_, err := e.c.Look(LookInput{File: "f.txt", StartLine: 1, EndLine: 1, Why: " "})
			return err
		}, CodeInvalidInput, "look", "f.txt"},
		{"range", func() *Error {
			_, err := e.c.Look(LookInput{File: "f.txt", StartLine: 9, EndLine: 9, Why: "w"})
			return err
		}, CodeInvalidRange, "look", "f.txt"},
		{"missing", func() *Error {
			_, err := e.c.Look(LookInput{File: "nope.txt", StartLine: 1, EndLine: 1, Why: "w"})
			return err
		}, CodeFileNotFound, "look", "nope.txt"},
		{"crlf", func() *Error {
			_, err := e.c.Look(LookInput{File: "crlf.txt", StartLine: 1, EndLine: 1, Why: "w"})
			return err
		}, CodeUnsupportedFile, "look", "crlf.txt"},
		{"ignored", func() *Error {
			_, err := e.c.Look(LookInput{File: ".env", StartLine: 1, EndLine: 1, Why: "w"})
			return err
		}, CodeIgnoredFile, "look", ""},
		{"stale", func() *Error {
			_, err := e.c.Edit(EditInput{Selection: s.Selection, NewText: "NEW TEXT", Why: "w"})
			return err
		}, CodeSelectionStale, "edit", "f.txt"},
		{"garbage token", func() *Error {
			_, err := e.c.Edit(EditInput{Selection: "sel_nonsense", NewText: "x", Why: "w"})
			return err
		}, CodeInvalidSelection, "edit", ""},
	}
	for _, c := range calls {
		before := len(e.failures())
		wantCode(t, c.do(), c.code)
		got := e.failures()
		if len(got) != before+1 {
			t.Errorf("%s: %d failures, want %d", c.name, len(got), before+1)
			continue
		}
		f := got[len(got)-1]
		if f.Tool != c.tool || f.Code != c.code || f.Message == "" || (f.File == nil) != (c.file == "") || (f.File != nil && *f.File != c.file) {
			t.Errorf("%s: failure = %+v (file %v)", c.name, f, f.File)
		}
	}
	e.noSecretOnTape(".env", "NEW TEXT") // the new text of a replace is not written
	e.checkTape()
}

func TestFailureLeavesOutTheRealPath(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n")
	home := filepath.Join(e.root, "secret-home", "f.txt")
	for name, tc := range map[string]struct {
		file string
		want string
	}{
		"absolute":  {home, "The path is absolute. Give a path relative to the workspace"},
		"drive":     {"C:/Users/someone/f.txt", "The path is absolute. Give a path relative to the workspace"},
		"outside":   {"../someone-else/f.txt", "The path points outside the workspace"},
		"hidden up": {"a/../../someone-else/f.txt", "The path points outside the workspace"},
	} {
		_, err := e.c.Look(LookInput{File: tc.file, StartLine: 1, EndLine: 1, Why: "w"})
		wantCode(t, err, CodeInvalidRange)
		f := e.failures()[len(e.failures())-1]
		if f.File != nil || f.Message != tc.want {
			t.Errorf("%s: file %v, message %q, want null and %q", name, f.File, f.Message, tc.want)
		}
	}
	// What the client is told is not changed: it still names the path.
	_, err := e.c.Look(LookInput{File: home, StartLine: 1, EndLine: 1, Why: "w"})
	if err == nil || !strings.Contains(err.Message, "secret-home") {
		t.Errorf("the error to the client = %+v", err)
	}
	e.noSecretOnTape("secret-home", "someone", "Users")
}

func TestFailureStartsATapeAndKeepsTheSeqsInOrder(t *testing.T) {
	e := newEnv(t)
	_, err := e.c.Look(LookInput{File: "nope.txt", StartLine: 1, EndLine: 1, Why: "w"})
	wantCode(t, err, CodeFileNotFound)
	e.write("a.txt", "x\n")
	e.sel(e.c, "a.txt", 1, 1)
	if got := e.kinds(); len(got) != 3 || got[0] != tape.TypeFailure {
		t.Errorf("kinds = %v", got)
	}
	e.checkTape()
}

func TestSuccessfulCallsRecordNoFailure(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n")
	s := e.sel(e.c, "f.txt", 1, 1)
	e.rep(e.c, s.Selection, "one")
	if got := e.failures(); len(got) != 0 {
		t.Errorf("failures = %+v", got)
	}
}

func TestInputFailureIsRecordedWithoutAFile(t *testing.T) {
	e := newEnv(t)
	e.c.RecordInputFailure("look", CodeInvalidInput, "missing required input: why")
	got := e.failures()
	if len(got) != 1 || got[0].Tool != "look" || got[0].File != nil || got[0].Code != CodeInvalidInput || got[0].Message != "missing required input: why" {
		t.Errorf("failures = %+v", got)
	}
}

func TestFailureMessageIsCut(t *testing.T) {
	e := newEnv(t)
	e.c.RecordInputFailure("look", CodeInvalidInput, strings.Repeat("あ", 1000))
	if m := e.failures()[0].Message; len([]rune(m)) != maxFailureMessage {
		t.Errorf("message has %d characters", len([]rune(m)))
	}
}
