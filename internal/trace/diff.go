// Package trace finds, for the lines a unified diff adds, the operations on the tapes that wrote them (srwr trace). It reads
// text and tapes only: it does not run git, and it writes nothing.
package trace

import (
	"regexp"
	"strconv"
	"strings"
)

// Commit is the part of a text that belongs to one commit (a "commit <sha>" line of git log -p or git show). A text without
// such lines (git diff) is one Commit with an empty SHA.
type Commit struct {
	SHA     string
	Subject string
	Files   []FileDiff
}

// FileDiff is the lines one file gets in a commit.
type FileDiff struct {
	Path  string // as the new side names it, relative to the workspace; "" for a deleted file
	Hunks []Hunk
}

// Hunk is one @@ section. Line is the 0-based index in the text of its @@ line, for the output that marks the text.
type Hunk struct {
	Line     int
	NewStart int
	Runs     []Run // the added lines, as runs of consecutive lines
}

// Run is added lines that follow one another in the new file.
type Run struct {
	Start int // line number in the new file of Lines[0]
	Lines []string
}

var (
	commitRe = regexp.MustCompile(`^commit ([0-9a-f]{7,40})\b`)
	hunkRe   = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
)

// ParseDiff reads the commits, files and added lines in any text that holds unified diffs. What it does not understand it passes
// over: it never fails.
func ParseDiff(text string) []Commit {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var commits []Commit
	ci := -1 // index in commits of the commit being read
	var file *FileDiff
	ensure := func() {
		if ci < 0 {
			commits = append(commits, Commit{})
			ci = len(commits) - 1
		}
	}
	finishFile := func() {
		if file != nil && ci >= 0 {
			commits[ci].Files = append(commits[ci].Files, *file)
		}
		file = nil
	}
	finishCommit := func() {
		finishFile()
		ci = -1
	}
	msgState := 0 // 0 none, 1 in the commit headers, 2 waiting for the subject
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if m := commitRe.FindStringSubmatch(l); m != nil {
			finishCommit()
			commits = append(commits, Commit{SHA: m[1]})
			ci = len(commits) - 1
			msgState = 1
			continue
		}
		switch msgState {
		case 1:
			if strings.TrimSpace(l) == "" {
				msgState = 2
			}
			continue
		case 2:
			if strings.HasPrefix(l, "    ") {
				commits[ci].Subject = strings.TrimSpace(l)
				msgState = 0
				continue
			}
			if strings.TrimSpace(l) != "" {
				msgState = 0
			}
		}
		switch {
		case strings.HasPrefix(l, "diff --git "):
			ensure()
			finishFile()
			file = &FileDiff{Path: pathOfGitLine(l)}
		case strings.HasPrefix(l, "+++ ") && file != nil && len(file.Hunks) == 0:
			file.Path = newPath(l[4:])
		case strings.HasPrefix(l, "+++ ") && file == nil:
			// a plain diff -u without "diff --git"
			ensure()
			file = &FileDiff{Path: newPath(l[4:])}
		default:
			m := hunkRe.FindStringSubmatch(l)
			if m == nil || file == nil {
				continue
			}
			oldN, newN := 1, 1
			if m[2] != "" {
				oldN, _ = strconv.Atoi(m[2])
			}
			if m[4] != "" {
				newN, _ = strconv.Atoi(m[4])
			}
			start, _ := strconv.Atoi(m[3])
			h := Hunk{Line: i, NewStart: start}
			n := start
			var run *Run
			for i+1 < len(lines) && (oldN > 0 || newN > 0) {
				b := lines[i+1]
				if b == "" && oldN+newN > 0 && i+2 >= len(lines) {
					break // the empty last line of the text
				}
				switch {
				case strings.HasPrefix(b, "+"):
					if newN <= 0 {
						goto done
					}
					if run == nil {
						h.Runs = append(h.Runs, Run{Start: n})
						run = &h.Runs[len(h.Runs)-1]
					}
					run.Lines = append(run.Lines, b[1:])
					n++
					newN--
				case strings.HasPrefix(b, "-"):
					if oldN <= 0 {
						goto done
					}
					run = nil
					oldN--
				case strings.HasPrefix(b, " ") || b == "":
					if oldN <= 0 || newN <= 0 {
						goto done
					}
					run = nil
					n++
					oldN--
					newN--
				case strings.HasPrefix(b, `\`):
					// "\ No newline at end of file"
				default:
					goto done
				}
				i++
			}
		done:
			file.Hunks = append(file.Hunks, h)
		}
	}
	finishCommit()
	return commits
}

// pathOfGitLine takes the path of the new side from "diff --git a/x b/x" (the + + + line, when there is one, says it better).
func pathOfGitLine(l string) string {
	rest := strings.TrimPrefix(l, "diff --git ")
	if i := strings.LastIndex(rest, " b/"); i >= 0 {
		return rest[i+3:]
	}
	return ""
}

// newPath cleans the name on a +++ line: no b/ prefix, no tab and time after it, "" for /dev/null.
func newPath(s string) string {
	s, _, _ = strings.Cut(s, "\t")
	s = strings.Trim(s, `"`)
	if s == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(s, "b/")
}
