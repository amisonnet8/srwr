// Package hook is srwr hook: it reads what Claude Code tells a hook (a JSON on the standard input) and records what the
// agent did with its own tools in the tape srwr mcp writes (docs/reference/cli.md).
//
// It must never get in the agent's way: whatever goes wrong, Run's caller tells the agent nothing and exits with 0.
package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/amisonnet8/srwr/internal/core"
)

// maxSelects is how many selects one hook call may record. A search over a big tree can match thousands of lines.
const maxSelects = 100

// input is the part of Claude Code's hook JSON that is used.
type input struct {
	Event    string          `json:"hook_event_name"`
	Tool     string          `json:"tool_name"`
	CWD      string          `json:"cwd"`
	Input    json.RawMessage `json:"tool_input"`
	Response json.RawMessage `json:"tool_response"`
}

// flexInt reads a JSON number or a string of digits.
type flexInt int

func (n *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*n = flexInt(f)
	return nil
}

// LookAdvice is what the agent is told after it read one file with Read, cat, head, tail or sed -n (not after a search over
// several files): look returns the same lines with a token for edit.
const LookAdvice = "srwr: to read part of a file and then edit it, call look (with search, or with startLine and endLine): it returns those lines with a selection token that edit takes as it is."

// OutsideAdvice is what the agent is told after a Bash command made or changed files: they are on the tape as changes made outside
// srwr, with no why. It is calm on purpose: files a tool writes (a generator, a formatter) are fine as they are. It is empty when
// the command changed nothing.
func OutsideAdvice(o core.OutsideChanges) string {
	var parts []string
	if s := nameList(o.Created); s != "" {
		parts = append(parts, "created "+s)
	}
	if s := nameList(o.Changed); s != "" {
		parts = append(parts, "changed "+s)
	}
	if len(parts) == 0 {
		return ""
	}
	return "srwr: this command " + strings.Join(parts, " and ") + ". They are on the tape as changes made outside srwr, with no why. When you write a file yourself, new (a new file) or edit (a change) records why with it; files a tool writes (a generator, a formatter) are fine as they are."
}

// nameList names up to 5 files, and counts the rest.
func nameList(names []string) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) > 5 {
		return strings.Join(names[:5], ", ") + fmt.Sprintf(" and %d more", len(names)-5)
	}
	return strings.Join(names, ", ")
}

// Run records one hook call. root is the workspace. The returned notes say what was left out and why.
func Run(stdin io.Reader, c *core.Core) (notes []string, err error) {
	notes, _, err = RunWithAdvice(stdin, c)
	return notes, err
}

// RunWithAdvice is Run that also returns what the agent is to be told (advice): to use look after it read one file.
func RunWithAdvice(stdin io.Reader, c *core.Core) (notes, advice []string, err error) {
	var in input
	dec := json.NewDecoder(stdin)
	if err := dec.Decode(&in); err != nil {
		return nil, nil, fmt.Errorf("the hook input is not valid JSON: %w", err)
	}
	if in.Event != "" && in.Event != "PostToolUse" {
		return nil, nil, nil
	}
	req, notes := request(in, c.WS.Root())
	if len(req.Looks) == 0 && req.Edit == nil && !req.ObserveAll {
		return notes, nil, nil
	}
	more, outside, err := c.HookChanges(req)
	notes = append(notes, more...)
	if err == nil && len(notes) == 0 && len(req.Looks) == 1 && readsOneFile(in) {
		advice = []string{LookAdvice}
	}
	if err == nil && in.Tool == "Bash" {
		if a := OutsideAdvice(outside); a != "" {
			advice = append(advice, a)
		}
	}
	return notes, advice, err
}

// readsOneFile tells whether the tool call read one file as a whole or by lines: a Read, or a Bash cat, nl, head, tail or sed -n. A
// search (Grep, grep -n) is not that.
func readsOneFile(in input) bool {
	switch in.Tool {
	case "Read":
		return true
	case "Bash":
		var t struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(in.Input, &t) != nil {
			return false
		}
		r, ok := parseBashRead(t.Command)
		return ok && !r.Numbers
	}
	return false
}

// request makes what to record out of a hook call.
func request(in input, root string) (req core.HookRequest, notes []string) {
	rel := func(p string) (string, bool) { return toRel(root, in.CWD, p) }
	switch in.Tool {
	case "Read":
		var t struct {
			File   string  `json:"file_path"`
			Offset flexInt `json:"offset"`
			Limit  flexInt `json:"limit"`
		}
		if json.Unmarshal(in.Input, &t) != nil {
			return req, []string{"cannot read the Read input"}
		}
		f, ok := rel(t.File)
		if !ok {
			return req, []string{t.File + " is outside the workspace, so it is not recorded"}
		}
		r := core.HookRange{Mode: core.RangeAll}
		if t.Offset > 0 || t.Limit > 0 {
			a := max(int(t.Offset), 1)
			b := 0
			if t.Limit > 0 {
				b = a + int(t.Limit) - 1
			}
			r = core.HookRange{Mode: core.RangeLines, A: a, B: b}
		}
		req.Looks = []core.HookLook{{File: f, Range: r, Tool: "Read"}}
	case "Edit":
		var t struct {
			File       string `json:"file_path"`
			Old        string `json:"old_string"`
			New        string `json:"new_string"`
			ReplaceAll bool   `json:"replace_all"`
		}
		if json.Unmarshal(in.Input, &t) != nil {
			return req, []string{"cannot read the Edit input"}
		}
		f, ok := rel(t.File)
		if !ok {
			return req, []string{t.File + " is outside the workspace, so it is not recorded"}
		}
		e := &core.HookEdit{File: f, OldString: t.Old, NewString: t.New, ReplaceAll: t.ReplaceAll}
		var resp struct {
			Original *string `json:"originalFile"`
		}
		if json.Unmarshal(in.Response, &resp) == nil {
			e.Original = resp.Original
		}
		req.Edit = e
	case "Bash":
		var t struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(in.Input, &t) != nil {
			return req, []string{"cannot read the Bash input"}
		}
		req.ObserveAll = true // whatever the command was, it may have changed the files
		if r, ok := parseBashRead(t.Command); ok {
			f, ok := rel(r.File)
			if !ok {
				return req, []string{r.File + " is outside the workspace, so it is not recorded"}
			}
			if !r.Numbers {
				req.Looks = []core.HookLook{{File: f, Range: r.Range, Tool: "Bash"}}
			} else {
				req.Looks, notes = grepSelects(numberedLines(responseText(in.Response), f), "Bash", func(p string) (string, bool) { return p, true })
			}
		}
	case "Grep":
		var t struct {
			Path string `json:"path"`
			Mode string `json:"output_mode"`
		}
		if json.Unmarshal(in.Input, &t) != nil {
			return req, []string{"cannot read the Grep input"}
		}
		if t.Mode != "content" {
			return req, nil // only the files or the counts were shown: no lines to record
		}
		// With one file as the path the lines have no file name.
		single := ""
		if p, ok := rel(t.Path); ok {
			if st, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err == nil && st.Mode().IsRegular() {
				single = p
			}
		}
		req.Looks, notes = grepSelects(numberedLines(responseText(in.Response), single), "Grep", rel)
	}
	return req, notes
}

// grepSelects makes a select of each run of numbered lines, by file.
func grepSelects(byFile map[string][]int, tool string, rel func(string) (string, bool)) ([]core.HookLook, []string) {
	var out []core.HookLook
	var notes []string
	names := make([]string, 0, len(byFile))
	for n := range byFile {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		f, ok := rel(name)
		if !ok {
			notes = append(notes, name+" is outside the workspace, so it is not recorded")
			continue
		}
		for _, r := range runsOf(byFile[name]) {
			if len(out) >= maxSelects {
				return out, append(notes, fmt.Sprintf("at most %d are recorded at a time; the rest are not recorded", maxSelects))
			}
			out = append(out, core.HookLook{File: f, Range: core.HookRange{Mode: core.RangeLines, A: r[0], B: r[1]}, Tool: tool})
		}
	}
	return out, notes
}

// responseText is the text of a tool's answer: the answer itself when it is a string, or the first of the fields that hold
// the output of a tool (content, stdout, output).
func responseText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for _, k := range []string{"content", "stdout", "output"} {
		if json.Unmarshal(m[k], &s) == nil && s != "" {
			return s
		}
	}
	return ""
}

// toRel turns the path of a tool into one relative to the workspace. A relative path is relative to where the agent was (cwd), and
// without a cwd to the workspace. A path outside the workspace gives false.
func toRel(root, cwd, p string) (string, bool) {
	if strings.TrimSpace(p) == "" {
		return "", false
	}
	if !filepath.IsAbs(p) {
		base := cwd
		if base == "" {
			base = root
		}
		p = filepath.Join(base, p)
	}
	r, err := filepath.Rel(realPath(root), realPath(p))
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || r == "." {
		return "", false
	}
	return filepath.ToSlash(r), true
}

// realPath follows symbolic links as far as the path exists (a path may not exist yet, or any more).
func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	if dir := filepath.Dir(p); dir != p {
		return filepath.Join(realPath(dir), filepath.Base(p))
	}
	return p
}
