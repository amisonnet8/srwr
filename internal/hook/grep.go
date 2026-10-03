package hook

import (
	"regexp"
	"sort"
	"strconv"
)

var (
	bareNumbered = regexp.MustCompile(`^(\d+)[:-]`)    // 12:text, from one file
	fileMatch    = regexp.MustCompile(`^(.+?):(\d+):`) // path:12:text
	fileContext  = regexp.MustCompile(`^(.+?)-(\d+)-`) // path-13-text (a line of context)
)

// numberedLines reads the lines of a numbered grep output: the line numbers it shows, by file. A line of output that has no
// number is skipped. file is the file when the output comes from one file alone (the lines have no file name).
func numberedLines(output, file string) map[string][]int {
	out := map[string][]int{}
	for _, line := range splitLines(output) {
		var name string
		var n int
		switch {
		case file != "" && bareNumbered.MatchString(line):
			n, _ = strconv.Atoi(bareNumbered.FindStringSubmatch(line)[1])
			name = file
		case fileMatch.MatchString(line):
			m := fileMatch.FindStringSubmatch(line)
			name, n = m[1], atoi(m[2])
		case fileContext.MatchString(line):
			m := fileContext.FindStringSubmatch(line)
			name, n = m[1], atoi(m[2])
		default:
			continue
		}
		if n > 0 {
			out[name] = append(out[name], n)
		}
	}
	for k := range out {
		sort.Ints(out[k])
	}
	return out
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// runsOf joins consecutive line numbers into ranges [first, last]. The numbers are sorted; duplicates are fine.
func runsOf(nums []int) [][2]int {
	var out [][2]int
	for _, n := range nums {
		if k := len(out); k > 0 && n <= out[k-1][1]+1 {
			out[k-1][1] = max(out[k-1][1], n)
			continue
		}
		out = append(out, [2]int{n, n})
	}
	return out
}
