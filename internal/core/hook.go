package core

import (
	"fmt"
	"sort"
	"strings"

	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/vcs"
)

// What srwr hook records (docs/reference/cli.md): what the agent did with its own tools, written to the same tape as
// select and replace, with no why and no token. Nothing here fails the agent: a file that cannot be recorded is left out
// and told in the notes.

// Hook range modes.
const (
	RangeAll   = "all"   // the whole file
	RangeLines = "lines" // lines A to B; B <= 0 means to the end
	RangeTail  = "tail"  // the last A lines
)

// HookRange says which lines of a file a hook select covers.
type HookRange struct {
	Mode string
	A, B int
}

// HookSelect is something the agent looked at.
type HookSelect struct {
	File  string // relative to the workspace
	Range HookRange
	Tool  string // Read, Bash or Grep
}

// HookEdit is an Edit the agent has made. The file already has the new content.
type HookEdit struct {
	File       string
	OldString  string
	NewString  string
	ReplaceAll bool
	Original   *string // the file before the edit, if the agent told
}

// HookRequest is everything one hook call records, in this order: the files read again, the selects, the edit.
type HookRequest struct {
	ObserveAll bool // read every file the tape has content of again, and record new files git lists (after Bash)
	Selects    []HookSelect
	Edit       *HookEdit
}

// Hook records a request. The returned notes say what was left out and why; the error is a failure of srwr itself.
func (c *Core) Hook(req HookRequest) (notes []string, err error) {
	err = c.WS.Do(func(tx *session.Tx) error {
		if req.ObserveAll {
			n, err := c.observeAll(tx)
			notes = append(notes, n...)
			if err != nil {
				return err
			}
			n, err = c.observeNew(tx)
			notes = append(notes, n...)
			if err != nil {
				return err
			}
		}
		for _, s := range req.Selects {
			note, err := c.hookSelect(tx, s)
			if note != "" {
				notes = append(notes, note)
			}
			if err != nil {
				return err
			}
		}
		if req.Edit != nil {
			note, err := c.hookEdit(tx, *req.Edit)
			if note != "" {
				notes = append(notes, note)
			}
			return err
		}
		return nil
	})
	return notes, err
}

// observeAll reads again every file the tape has content of, and records what changed (external).
func (c *Core) observeAll(tx *session.Tx) (notes []string, err error) {
	var names []string
	for name := range tx.State().Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t, cerr := c.readTarget(name)
		if cerr != nil && cerr.Code == CodeIgnoredFile {
			continue // a file that is not recorded is not looked at either
		}
		if cerr != nil {
			notes = append(notes, fmt.Sprintf("%s cannot be read again: %s", name, cerr.Message))
			continue
		}
		var cur *string
		if t.exists {
			cur = &t.text
		}
		if err := Observe(tx, name, "hook", cur); err != nil {
			return notes, err
		}
	}
	return notes, nil
}

// Limits of the new files one Bash command may add to the tape, so that a command that writes a lot (a generator, an unpacked
// archive) does not swell it.
const (
	maxNewFiles    = 50
	maxNewFileSize = 256 << 10
)

// observeNew records the new files a Bash command made: files git lists as untracked (and does not ignore) that the tape has no
// content of. Outside a git work tree there are none to find, and that is not worth stopping the agent for. Every file goes
// through readTarget, so the files that are not recorded, binary files and links out of the workspace are left out.
func (c *Core) observeNew(tx *session.Tx) (notes []string, err error) {
	files, verr := vcs.Untracked(c.WS.Root())
	if verr != nil {
		return nil, nil //nolint:nilerr // no git work tree means no list of new files; that must not stop the agent
	}
	recorded, over := 0, 0
	for _, name := range files {
		if _, known := tx.State().Files[name]; known {
			continue
		}
		if recorded >= maxNewFiles {
			over++
			continue
		}
		t, cerr := c.readTarget(name)
		if cerr != nil || !t.exists || len(t.text) > maxNewFileSize {
			continue // not recorded, not a text file, or too big: it is left out
		}
		if err := observeCreated(tx, name, "hook", t.text); err != nil {
			return notes, err
		}
		recorded++
	}
	if over > 0 {
		notes = append(notes, fmt.Sprintf("%d more new files were not recorded (at most %d new files are recorded for one command)", over, maxNewFiles))
	}
	return notes, nil
}

// readForHook finds a file for a hook call and notes the ones that cannot be recorded.
func (c *Core) readForHook(file string) (rel string, t target, note string) {
	rel, cerr := cleanPath(file)
	if cerr != nil {
		return "", target{}, fmt.Sprintf("%s is not recorded: %s", file, cerr.Message)
	}
	t, cerr = c.readTarget(rel)
	if cerr != nil && cerr.Code == CodeIgnoredFile {
		return rel, target{}, rel + " is a file that is not recorded, so it is not recorded"
	}
	if cerr != nil {
		return rel, target{}, fmt.Sprintf("%s is not recorded: %s", rel, cerr.Message)
	}
	return rel, t, ""
}

func (c *Core) hookSelect(tx *session.Tx, s HookSelect) (string, error) {
	rel, t, note := c.readForHook(s.File)
	if note != "" {
		return note, nil
	}
	if !t.exists {
		return rel + " does not exist, so it is not recorded", Observe(tx, rel, "hook", nil)
	}
	if err := Observe(tx, rel, "hook", &t.text); err != nil {
		return "", err
	}
	n := len(tape.Lines(t.text))
	a, b := hookLines(s.Range, n)
	if a > b {
		return fmt.Sprintf("the range of %s is empty, so it is not recorded", rel), nil
	}
	return "", tx.Append(tape.Event{
		Type: tape.TypeSelect, Seq: tx.NextSeq(), File: rel, StartLine: a, EndLine: b,
		Source: tape.SourceHook, HookTool: s.Tool,
	})
}

// hookLines turns a range into line numbers of a file of n lines. An empty result has a > b.
func hookLines(r HookRange, n int) (a, b int) {
	switch r.Mode {
	case RangeLines:
		a, b = max(r.A, 1), r.B
		if b <= 0 || b > n {
			b = n
		}
	case RangeTail:
		a, b = max(n-r.A+1, 1), n
	default:
		a, b = 1, n
	}
	if n == 0 {
		return 1, 0
	}
	return a, b
}

func (c *Core) hookEdit(tx *session.Tx, e HookEdit) (string, error) {
	rel, t, note := c.readForHook(e.File)
	if note != "" {
		return note, nil
	}
	if !t.exists {
		return rel + " does not exist, so it is not recorded", Observe(tx, rel, "hook", nil)
	}
	known := tx.State().Files[rel]
	haveKnown := known != nil && !known.Deleted
	var before *string
	switch {
	case haveKnown:
		before = &known.Text
	case e.Original != nil:
		before = e.Original
	}
	if before == nil {
		return rel + ": the content before the edit is not known, so only the current content is recorded", Observe(tx, rel, "hook", &t.text)
	}
	steps, final, ok := editSteps(*before, e.OldString, e.NewString, e.ReplaceAll)
	if !ok || final != t.text {
		// Not what the edit says it did (changed by something else as well): the tape shows it as external.
		return rel + ": the Edit does not match the current file, so it is recorded as external", Observe(tx, rel, "hook", &t.text)
	}
	if !haveKnown {
		if err := appendSnapshot(tx, rel, *before); err != nil {
			return "", err
		}
	}
	for _, s := range steps {
		err := tx.Append(tape.Event{
			Type: tape.TypeReplace, Seq: tx.NextSeq(), File: rel,
			StartLine: s.a, EndLine: s.b, OldText: s.oldText, NewText: strings.Join(s.newLines, "\n"),
			NewStartLine: s.a, NewEndLine: s.a + len(s.newLines) - 1,
			FileShaBefore: s.shaBefore, FileShaAfter: s.shaAfter, Source: tape.SourceHook, HookTool: "Edit",
		})
		if err != nil {
			return "", err
		}
	}
	return "", nil
}

// editStep is one replacement of an Edit, as lines.
type editStep struct {
	a, b                int // the lines (whole lines) of the text before this step
	oldText             string
	newLines            []string
	shaBefore, shaAfter string
}

// editSteps applies old -> new to before the way Edit does (the first match, or every match with all) and describes each
// replacement as whole lines. ok is false when the edit does not apply or cannot be told in lines.
func editSteps(before, old, replacement string, all bool) (steps []editStep, final string, ok bool) {
	if old == "" {
		return nil, "", false
	}
	cur, from := before, 0
	for {
		i := strings.Index(cur[from:], old)
		if i < 0 {
			break
		}
		i += from
		a := 1 + strings.Count(cur[:i], "\n")
		b := 1 + strings.Count(cur[:i+len(old)-1], "\n")
		rs := lineStart(cur, a)
		re := lineEnd(cur, b)
		region := cur[rs:re]
		newRegion := region[:i-rs] + replacement + region[i-rs+len(old):]
		newLines := tape.Lines(newRegion)
		after := tape.SpliceLines(cur, a, b, newLines)
		if after != cur[:i]+replacement+cur[i+len(old):] {
			return nil, "", false // for example a newline added at the end of the file: lines cannot tell it
		}
		steps = append(steps, editStep{a: a, b: b, oldText: tape.RangeText(cur, a, b), newLines: newLines, shaBefore: tape.Sha(cur), shaAfter: tape.Sha(after)})
		cur, from = after, i+len(replacement)
		if !all {
			break
		}
	}
	if len(steps) == 0 {
		return nil, "", false
	}
	return steps, cur, true
}

// lineStart is the offset of the start of line n (1-based) of text.
func lineStart(text string, n int) int {
	off := 0
	for ; n > 1; n-- {
		i := strings.IndexByte(text[off:], '\n')
		if i < 0 {
			return len(text)
		}
		off += i + 1
	}
	return off
}

// lineEnd is the offset after line n of text, including its newline.
func lineEnd(text string, n int) int {
	off := lineStart(text, n)
	if i := strings.IndexByte(text[off:], '\n'); i >= 0 {
		return off + i + 1
	}
	return len(text)
}
