package core

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/amisonnet8/srwr/internal/ignore"
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/token"
)

// NewInput is the input of new: a file to create, with its content.
type NewInput struct {
	File    string
	Content string
	Why     string
}

// NewResult is what new returns: a token for the whole content, and the lines it spans.
type NewResult struct {
	Selection string
	StartLine int
	EndLine   int
}

// New creates a file that does not exist yet. A failure is also written to the tape (without the content).
func (c *Core) New(in NewInput) (*NewResult, *Error) {
	res, cerr := c.doNew(in)
	if cerr != nil {
		c.recordFailure(failedCall{tool: toolNew, file: in.File, why: &in.Why, err: cerr})
	}
	return res, cerr
}

func (c *Core) doNew(in NewInput) (*NewResult, *Error) {
	if err := checkWhy(in.Why); err != nil {
		return nil, err
	}
	if strings.ContainsRune(in.Content, '\r') {
		return nil, newError(CodeInvalidInput, "content must not contain CR (line breaks are LF only)")
	}
	rel, cerr := cleanPath(in.File)
	if cerr != nil {
		return nil, cerr
	}
	var res *NewResult
	cerr = c.run(func(tx *session.Tx) error {
		var err error
		res, err = c.newIn(tx, rel, in)
		return err
	})
	if cerr != nil {
		return nil, cerr
	}
	return res, nil
}

func (c *Core) newIn(tx *session.Tx, rel string, in NewInput) (*NewResult, error) {
	t, cerr := c.readTarget(rel)
	if cerr != nil {
		return nil, cerr
	}
	if t.exists {
		return nil, newError(CodeFileExists, "%s already exists. Use look and edit to change it", rel)
	}
	// A file the tape knew and that is gone now is recorded as deleted before it is made again.
	if err := Observe(tx, rel, "new", nil); err != nil {
		return nil, err
	}
	dest, cerr := c.prepareParent(rel)
	if cerr != nil {
		return nil, cerr
	}

	lines := tape.Lines(in.Content)
	text := tape.SpliceLines("", 1, 0, lines)
	// The file first, then the tape (see editIn).
	if cerr := createFile(dest, text, rel); cerr != nil {
		return nil, cerr
	}
	return appendNew(tx, rel, lines, text, in.Why)
}

// appendNew puts the new event of a file that has just been made on the tape.
func appendNew(tx *session.Tx, rel string, lines []string, text, why string) (*NewResult, error) {
	end := len(lines)
	sel := token.Encode(token.Token{
		Seq:       uint64(tx.NextSeq()), //nolint:gosec // NextSeq is at least 1
		StartLine: 1,
		EndLine:   uint64(end),
		FileHash:  token.Hash4(rel),
		TextHash:  token.Hash4(tape.RangeText(text, 1, end)),
	}, tx.TapeID(), tx.Key())
	err := tx.Append(tape.Event{
		Type: tape.TypeNew, Seq: tx.NextSeq(), File: rel,
		StartLine: 1, EndLine: 0, OldText: "", NewText: strings.Join(lines, "\n"),
		NewStartLine: 1, NewEndLine: end, Selection: &sel, Why: &why,
		FileShaBefore: "", FileShaAfter: tape.Sha(text),
		Source: tape.SourceMCP,
	})
	if err != nil {
		return nil, err
	}
	return &NewResult{Selection: sel, StartLine: 1, EndLine: end}, nil
}

// prepareParent makes the directories above rel and returns the path to create the file at. The nearest directory that exists is
// resolved first, so that a link in the way cannot lead out of the workspace or into a place that is not recorded.
func (c *Core) prepareParent(rel string) (string, *Error) {
	m, err := ignore.Load(c.WS.Root())
	if err != nil {
		return "", internal(err)
	}
	realRoot, err := filepath.EvalSymlinks(c.WS.Root())
	if err != nil {
		return "", internal(err)
	}
	parts := strings.Split(rel, "/")
	dir, rest := filepath.Join(c.WS.Root(), filepath.FromSlash(strings.Join(parts[:len(parts)-1], "/"))), []string{}
	var base string
	// Walk up to the nearest directory that exists.
	for cur, i := dir, len(parts)-1; ; i-- {
		real, err := filepath.EvalSymlinks(cur)
		if err == nil {
			info, serr := os.Stat(real)
			if serr != nil {
				return "", internal(serr)
			}
			if !info.IsDir() {
				return "", newError(CodeInvalidInput, "%s cannot be created: %s is not a directory", rel, strings.Join(parts[:i], "/"))
			}
			base = real
			break
		}
		if errors.Is(err, syscall.ENOTDIR) {
			return "", newError(CodeInvalidInput, "%s cannot be created: a directory above it is a file", rel)
		}
		if !errors.Is(err, fs.ErrNotExist) || i == 0 {
			return "", internal(err)
		}
		rest = append([]string{parts[i-1]}, rest...)
		cur = filepath.Dir(cur)
	}
	within, err := filepath.Rel(realRoot, base)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", newError(CodeInvalidRange, "%s points outside the workspace", rel)
	}
	final := append(append([]string{}, strings.Split(filepath.ToSlash(within), "/")...), rest...)
	final = append(final, parts[len(parts)-1])
	if m.Match(strings.TrimPrefix(strings.Join(final, "/"), "./")) {
		return "", ignoredError(rel)
	}
	dirPath := filepath.Join(append([]string{base}, rest...)...)
	if err := os.MkdirAll(dirPath, 0o755); err != nil { //nolint:gosec // a directory of the project, like any a person makes (the umask applies)
		return "", internal(err)
	}
	return filepath.Join(dirPath, parts[len(parts)-1]), nil
}

// createFile writes text to a temporary file next to dest and links it into place, so that a reader never sees half a file, and a file
// that appeared in the meantime is not overwritten.
func createFile(dest, text, rel string) *Error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".srwr-*.tmp")
	if err != nil {
		return internal(err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(text); err != nil {
		_ = tmp.Close()
		return internal(err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return internal(err)
	}
	if err := tmp.Close(); err != nil {
		return internal(err)
	}
	if err := os.Link(tmp.Name(), dest); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return newError(CodeFileExists, "%s already exists. Use look and edit to change it", rel)
		}
		return internal(err)
	}
	return nil
}
