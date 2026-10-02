package tape

import "strings"

// Lines splits text into lines the way docs/reference/mcp.md counts them:
// "" is no lines, and a final "\n" does not start another line.
func Lines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// RangeText returns lines start..end (1-based, both included) joined with "\n" and no final newline.
// An empty range (end = start-1) gives "". It is the text a range token and oldText are about.
// Lines outside the text are ignored.
func RangeText(text string, start, end int) string {
	lines := Lines(text)
	start = max(start, 1)
	end = min(end, len(lines))
	if start > end {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}

// Splice replaces lines start..end of text with newText and returns the new text.
// start-1 = end inserts before line start; an empty newText deletes the range.
// Whether the text ends with a newline is kept, except that a text that was empty gets one.
func Splice(text string, start, end int, newText string) string {
	return SpliceLines(text, start, end, Lines(newText))
}

// SpliceLines is Splice with the new lines already split. A caller that knows how many lines it
// writes uses this, because Lines cannot tell a blank last line from the end of the text.
func SpliceLines(text string, start, end int, newLines []string) string {
	lines := Lines(text)
	start = max(start, 1)
	start = min(start, len(lines)+1)
	end = min(max(end, start-1), len(lines))

	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:start-1]...)
	out = append(out, newLines...)
	out = append(out, lines[end:]...)
	if len(out) == 0 {
		return ""
	}
	res := strings.Join(out, "\n")
	if text == "" || strings.HasSuffix(text, "\n") {
		res += "\n"
	}
	return res
}
