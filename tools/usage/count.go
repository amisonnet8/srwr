package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Report is what one conversation file holds: how many times each form was used.
type Report struct {
	File   string         `json:"file"`
	Counts map[string]int `json:"counts"`
}

// line is the part of a line of a conversation file this tool reads.
type line struct {
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type block struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     map[string]any  `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

type call struct {
	name  string
	input map[string]any
}

var codeRe = regexp.MustCompile(`"code":\s*"([a-z_]+)"`)

// ReadFile counts one conversation file.
func ReadFile(path string) (Report, error) {
	f, err := os.Open(path) //nolint:gosec // a conversation file the person named
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = f.Close() }()
	r := Report{File: path, Counts: map[string]int{}}
	calls := map[string]call{}
	var order []string
	results := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 256<<20)
	for sc.Scan() {
		var l line
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		var blocks []block
		if json.Unmarshal(l.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			switch b.Type {
			case "tool_use":
				calls[b.ID] = call{b.Name, b.Input}
				order = append(order, b.ID)
			case "tool_result":
				results[b.ToolUseID] = resultText(b.Content)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return r, err
	}
	for _, id := range order {
		c := calls[id]
		Count(r.Counts, c.name, c.input, results[id])
	}
	return r, nil
}

func resultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var sb strings.Builder
		for _, p := range parts {
			sb.WriteString(p.Text)
		}
		return sb.String()
	}
	return ""
}

// srwrTool returns look, edit, replace or new when name is a tool of srwr (mcp__srwr__look and the like).
func srwrTool(name string) string {
	if !strings.HasPrefix(name, "mcp__srwr") {
		return ""
	}
	i := strings.LastIndex(name, "__")
	switch t := name[i+2:]; t {
	case "look", "edit", "replace", "new":
		return t
	}
	return ""
}

// Count adds one call (and what it returned) to counts.
func Count(counts map[string]int, name string, in map[string]any, result string) {
	t := srwrTool(name)
	if t == "" {
		counts["other: "+name]++
		if name == "Bash" {
			if cmd, _ := in["command"].(string); cmd != "" {
				counts["bash: "+strings.Fields(cmd)[0]]++
			}
		}
		return
	}
	counts["srwr: "+t]++
	for _, s := range shapes(t, in) {
		counts[t+": "+s]++
	}
	if strings.Contains(result, `"ok":false`) || strings.Contains(result, `\"ok\":false`) {
		code := "?"
		if m := codeRe.FindStringSubmatch(strings.ReplaceAll(result, `\"`, `"`)); m != nil {
			code = m[1]
		}
		counts["failed: "+t+" "+code]++
		if strings.Contains(result, "retry") {
			counts["failed: with retry"]++
		}
		if strings.Contains(result, "nearMatches") {
			counts["failed: with nearMatches"]++
		}
	}
}

func has(in map[string]any, k string) bool { _, ok := in[k]; return ok }

// shapes names the forms of the arguments one call used.
func shapes(tool string, in map[string]any) []string {
	var out []string
	switch tool {
	case "look":
		file, _ := in["file"].(string)
		isDir := file == "." || strings.HasSuffix(file, "/")
		switch {
		case has(in, "looks"):
			n := 0
			if a, ok := in["looks"].([]any); ok {
				n = len(a)
			}
			out = append(out, fmt.Sprintf("looks (%d items)", n))
		case has(in, "search"):
			s := "search of a file"
			if isDir {
				s = "search of a directory"
			}
			out = append(out, s)
			for _, k := range []string{"include", "exclude", "offset"} {
				if has(in, k) {
					out = append(out, "search with "+k)
				}
			}
		case has(in, "startLine") && has(in, "expect"):
			out = append(out, "range with expect")
		case has(in, "startLine"):
			out = append(out, "range")
		case has(in, "expect"):
			out = append(out, "expect only")
		default:
			out = append(out, "whole file (no range)")
		}
	case "edit":
		if a, ok := in["edits"].([]any); ok {
			bucket := "1 to 5 items"
			if len(a) >= 6 {
				bucket = "6 or more items"
			}
			out = append(out, "edits ("+bucket+")")
			for _, it := range a {
				if m, ok := it.(map[string]any); ok {
					out = append(out, "edits item: "+itemShape(m))
					if has(m, "why") {
						out = append(out, "edits item with a why")
					}
				}
			}
		} else {
			out = append(out, "one edit: "+itemShape(in))
		}
		if has(in, "brief") {
			out = append(out, fmt.Sprintf("brief=%v", in["brief"]))
		}
	case "replace":
		out = append(out, fmt.Sprintf("count=%v", in["count"]))
	}
	return out
}

// itemShape names what an edit (or an item of edits) points at and what it puts.
func itemShape(in map[string]any) string {
	var parts []string
	switch {
	case has(in, "content"):
		return "content (a new file)"
	case has(in, "selection"):
		parts = append(parts, "selection")
	case has(in, "insert"):
		parts = append(parts, fmt.Sprintf("insert %v", in["insert"]))
	case has(in, "old"):
		parts = append(parts, "file with old/new")
	case has(in, "expect"):
		parts = append(parts, "file with expect")
	case has(in, "startLine"):
		parts = append(parts, "file with line numbers")
	default:
		parts = append(parts, "other")
	}
	return strings.Join(parts, " ")
}

// Sorted returns the keys of counts, by group (the text before ": ") and then by count, most first.
func Sorted(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		gi, gj := group(keys[i]), group(keys[j])
		if gi != gj {
			return gi < gj
		}
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}

func group(k string) string {
	if i := strings.Index(k, ": "); i >= 0 {
		return k[:i]
	}
	return k
}
