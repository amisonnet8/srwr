package hook

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/amisonnet8/srwr/internal/core"
)

// bashRead is what a Bash command read: one file, and which lines of it.
type bashRead struct {
	File    string
	Range   core.HookRange
	Numbers bool // the lines are the numbers in the output (grep -n)
}

// parseBashRead understands the commands of docs/reference/cli.md that only read one file: cat, nl, head, tail, sed -n 'A,Bp'
// and grep -n. Only the first command of a pipeline is looked at. ok is false for everything else, and for anything that
// writes or may run something else ($( ), backticks, redirects, sed -i, tail -f).
func parseBashRead(command string) (r bashRead, ok bool) {
	words, ok := firstCommand(command)
	if !ok || len(words) == 0 {
		return bashRead{}, false
	}
	name, args := words[0], words[1:]
	switch name {
	case "cat", "nl":
		file, ok := oneFile(args, nil)
		return bashRead{File: file, Range: core.HookRange{Mode: core.RangeAll}}, ok
	case "head", "tail":
		return parseHeadTail(name, args)
	case "sed":
		return parseSed(args)
	case "grep":
		return parseGrep(args)
	}
	return bashRead{}, false
}

// firstCommand splits the command into words up to the first control operator. It refuses a command with command
// substitution, or with a redirect other than to /dev/null or between descriptors (2>&1).
func firstCommand(command string) (words []string, ok bool) {
	var cur strings.Builder
	has := false // a word is being read (it may be empty: '')
	flush := func() {
		if has {
			words = append(words, cur.String())
			cur.Reset()
			has = false
		}
	}
	rs := []rune(command)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\'':
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				cur.WriteRune(rs[j])
				j++
			}
			if j >= len(rs) {
				return nil, false
			}
			has = true
			i = j
		case c == '"':
			j := i + 1
			for j < len(rs) && rs[j] != '"' {
				if rs[j] == '\\' && j+1 < len(rs) {
					j++
				} else if rs[j] == '`' || (rs[j] == '$' && j+1 < len(rs) && rs[j+1] == '(') {
					return nil, false
				}
				cur.WriteRune(rs[j])
				j++
			}
			if j >= len(rs) {
				return nil, false
			}
			has = true
			i = j
		case c == '\\' && i+1 < len(rs):
			i++
			cur.WriteRune(rs[i])
			has = true
		case c == '`' || (c == '$' && i+1 < len(rs) && rs[i+1] == '('):
			return nil, false
		case c == ' ' || c == '\t':
			flush()
		case c == '|' || c == ';' || c == '&' || c == '\n':
			if c == '&' && i+1 < len(rs) && rs[i+1] == '>' {
				return nil, false
			}
			if c == '&' && cur.Len() == 0 && !has && len(words) > 0 && strings.HasSuffix(words[len(words)-1], ">") {
				return nil, false
			}
			flush()
			return words, true
		case c == '>' || c == '<':
			rest := string(rs[i:])
			switch {
			case c == '<':
				return nil, false // a here-document or a file as input: not a plain read of one file
			case strings.HasPrefix(rest, ">/dev/null"), strings.HasPrefix(rest, ">&"):
				// harmless: the output goes nowhere, or to another descriptor
				if cur.Len() == 1 && (cur.String() == "1" || cur.String() == "2") {
					cur.Reset()
					has = false
				}
				j := i + 1
				for j < len(rs) && rs[j] != ' ' && rs[j] != '\t' {
					j++
				}
				i = j - 1
			default:
				return nil, false
			}
		default:
			cur.WriteRune(c)
			has = true
		}
	}
	flush()
	return words, true
}

// oneFile returns the only word that is not an option. flagsWithValue are the options that take a value as the next word.
func oneFile(args []string, flagsWithValue map[string]bool) (string, bool) {
	file := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			continue
		case strings.HasPrefix(a, "-") && a != "-":
			if flagsWithValue[a] {
				i++
			}
		case a == "-":
			return "", false
		default:
			if file != "" {
				return "", false
			}
			file = a
		}
	}
	return file, file != ""
}

var digits = regexp.MustCompile(`^\d+$`)

func parseHeadTail(name string, args []string) (bashRead, bool) {
	count, from := 10, false
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-n" || a == "--lines":
			if i+1 >= len(args) {
				return bashRead{}, false
			}
			i++
			v := args[i]
			if strings.HasPrefix(v, "+") && name == "tail" {
				from, v = true, v[1:]
			}
			if !digits.MatchString(v) {
				return bashRead{}, false
			}
			count, _ = strconv.Atoi(v)
		case strings.HasPrefix(a, "-n") && len(a) > 2:
			v := a[2:]
			if strings.HasPrefix(v, "+") && name == "tail" {
				from, v = true, v[1:]
			}
			if !digits.MatchString(v) {
				return bashRead{}, false
			}
			count, _ = strconv.Atoi(v)
		case digits.MatchString(strings.TrimPrefix(a, "-")) && strings.HasPrefix(a, "-"):
			count, _ = strconv.Atoi(a[1:])
		case strings.HasPrefix(a, "-") && a != "-" && a != "--":
			return bashRead{}, false // -f, -F, -c, -q ...: not a plain read of the first or last lines
		default:
			rest = append(rest, a)
		}
	}
	file, ok := oneFile(rest, nil)
	if !ok || count <= 0 {
		return bashRead{}, false
	}
	switch {
	case name == "head":
		return bashRead{File: file, Range: core.HookRange{Mode: core.RangeLines, A: 1, B: count}}, true
	case from:
		return bashRead{File: file, Range: core.HookRange{Mode: core.RangeLines, A: count}}, true
	}
	return bashRead{File: file, Range: core.HookRange{Mode: core.RangeTail, A: count}}, true
}

var sedScript = regexp.MustCompile(`^(\d+)(?:,(\d+|\$))?p$`)

func parseSed(args []string) (bashRead, bool) {
	quiet, script := false, ""
	var rest []string
	for _, a := range args {
		switch {
		case a == "-n" || a == "--quiet" || a == "--silent":
			quiet = true
		case strings.HasPrefix(a, "-") && a != "-":
			return bashRead{}, false // -i and --in-place edit the file; nothing but -n is a plain read
		case script == "":
			script = a
		default:
			rest = append(rest, a)
		}
	}
	m := sedScript.FindStringSubmatch(script)
	file, ok := oneFile(rest, nil)
	if !quiet || m == nil || !ok {
		return bashRead{}, false
	}
	a, _ := strconv.Atoi(m[1])
	b := a
	switch m[2] {
	case "":
	case "$":
		b = 0
	default:
		b, _ = strconv.Atoi(m[2])
	}
	if a < 1 || (b != 0 && b < a) {
		return bashRead{}, false
	}
	return bashRead{File: file, Range: core.HookRange{Mode: core.RangeLines, A: a, B: b}}, true
}

func parseGrep(args []string) (bashRead, bool) {
	hasN := false
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--line-number":
			hasN = true
		case a == "-e" || a == "-f" || a == "-m" || a == "-A" || a == "-B" || a == "-C":
			i++ // the value of the option
			if a == "-e" {
				rest = append(rest, "pattern")
			}
		case strings.HasPrefix(a, "--"):
			// long options other than --line-number do not change what a numbered line is
		case strings.HasPrefix(a, "-") && a != "-":
			flags := a[1:]
			if strings.ContainsAny(flags, "rRlLcqH") {
				return bashRead{}, false // recursive, only names, only counts, or prefixed with the file name
			}
			if strings.ContainsRune(flags, 'n') {
				hasN = true
			}
		default:
			rest = append(rest, a)
		}
	}
	if !hasN || len(rest) != 2 {
		return bashRead{}, false
	}
	return bashRead{File: rest[1], Range: core.HookRange{Mode: core.RangeAll}, Numbers: true}, true
}
