package core

import (
	"fmt"
	"slices"
	"strings"
)

// maxNear is how many near matches an error lists.
const maxNear = 5

// NearMatch is a place that differs from what the client asked for only in spaces and tabs: the file (for replace), the lines
// as they are, and their numbers.
type NearMatch struct {
	File      string   `json:"file,omitempty"`
	StartLine int      `json:"startLine"`
	EndLine   int      `json:"endLine"`
	Lines     []string `json:"lines"`
}

// normLine folds every run of spaces and tabs into one space and drops the spaces at the end of the line.
func normLine(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			space = true
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

func normLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = normLine(l)
	}
	return out
}

// nearLines returns the first line of every place where lines want are in lines but for spaces and tabs. The caller has found no
// place where they are exactly.
func nearLines(lines, want []string) []NearMatch {
	if len(want) == 0 {
		return nil
	}
	nl, nw := normLines(lines), normLines(want)
	var out []NearMatch
	for i := 0; i+len(nw) <= len(nl); i++ {
		if slices.Equal(nl[i:i+len(nw)], nw) {
			out = append(out, NearMatch{StartLine: i + 1, EndLine: i + len(nw), Lines: slices.Clone(lines[i : i+len(nw)])})
		}
	}
	return out
}

// nearText returns the places of old in text but for spaces and tabs, leaving out the lines where old is exactly.
func nearText(text, old string) []NearMatch {
	nt, no := strings.Join(normLines(strings.Split(text, "\n")), "\n"), strings.Join(normLines(strings.Split(old, "\n")), "\n")
	if no == "" {
		return nil
	}
	exact := map[int]bool{}
	for from := 0; ; {
		i := strings.Index(text[from:], old)
		if i < 0 {
			break
		}
		exact[1+strings.Count(text[:from+i], "\n")] = true
		from += i + len(old)
	}
	lines := strings.Split(text, "\n")
	span := strings.Count(no, "\n")
	var out []NearMatch
	for from := 0; ; {
		i := strings.Index(nt[from:], no)
		if i < 0 {
			break
		}
		start := 1 + strings.Count(nt[:from+i], "\n")
		from += i + len(no)
		if exact[start] || start+span > len(lines) {
			continue
		}
		out = append(out, NearMatch{StartLine: start, EndLine: start + span, Lines: slices.Clone(lines[start-1 : start+span])})
	}
	return out
}

// partLines returns the places where every line of want is in the lines of the file as a part of the line (spaces and tabs
// folded). The caller has found no place where they are exactly or but for spaces and tabs.
func partLines(lines, want []string) []NearMatch {
	nw := make([]string, len(want))
	blank := true
	for i, w := range want {
		nw[i] = strings.TrimSpace(normLine(w))
		if nw[i] != "" {
			blank = false
		}
	}
	if len(want) == 0 || blank {
		return nil
	}
	nl := normLines(lines)
	var out []NearMatch
	for i := 0; i+len(nw) <= len(nl); i++ {
		ok := true
		for k, w := range nw {
			if !strings.Contains(nl[i+k], w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, NearMatch{StartLine: i + 1, EndLine: i + len(nw), Lines: slices.Clone(lines[i : i+len(nw)])})
		}
	}
	return out
}

// nearOrPart adds to the error the places that differ from want only in spaces or tabs or, if there are none, the places that
// hold want as parts of their lines.
func nearOrPart(e *Error, file string, lines, want []string) *Error {
	if near := nearLines(lines, want); len(near) > 0 {
		return withNear(e, file, near, "expect")
	}
	return withPart(e, file, partLines(lines, want))
}

// nearWhere keeps at most maxNear of the matches, sets their file, and names their places ("line 3", "lines 3, 9").
func nearWhere(file string, near []NearMatch) ([]NearMatch, string) {
	if len(near) > maxNear {
		near = near[:maxNear]
	}
	at := make([]int, len(near))
	for i := range near {
		near[i].File = file
		at[i] = near[i].StartLine
	}
	where := places(at, maxNear)
	if len(near) == 1 && near[0].EndLine > near[0].StartLine {
		where = fmt.Sprintf("lines %d to %d", near[0].StartLine, near[0].EndLine)
	}
	return near, where
}

// withNear adds the near matches to the error: the first maxNear of them, and a sentence of the message that says where, without
// any of the file (the message goes on the tape). what is "expect" or "old".
func withNear(e *Error, file string, near []NearMatch, what string) *Error {
	if len(near) == 0 {
		return e
	}
	near, where := nearWhere(file, near)
	e.Message += fmt.Sprintf(". %s differ%s from %s only in spaces or tabs: see nearMatches", capFirst(where), plural(where), what)
	e.NearMatches = near
	return e
}

// withPart adds the places that hold expect only as parts of lines.
func withPart(e *Error, file string, part []NearMatch) *Error {
	if len(part) == 0 {
		return e
	}
	part, where := nearWhere(file, part)
	verb := "hold"
	if plural(where) == "s" {
		verb = "holds"
	}
	e.Message += fmt.Sprintf(". %s %s expect only as part of the line: expect must be whole lines. Copy them from nearMatches", capFirst(where), verb)
	e.NearMatches = part
	return e
}

func capFirst(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

func plural(where string) string {
	if strings.HasPrefix(where, "line ") && !strings.Contains(where, " to ") {
		return "s"
	}
	return ""
}
