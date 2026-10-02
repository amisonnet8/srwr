// Package docs only holds tests that check the documents under docs/ and the
// links and paths that point at them (.claude/rules/documentation.md).
package docs

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
)

var (
	linkRe     = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	codeSpanRe = regexp.MustCompile("`([^`]+)`")
	headingRe  = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
)

// check returns one message per problem found under root, as "file:line: text".
// Paths are slash-separated and relative to root.
func check(root string) []string {
	var problems []string
	linkFiles := markdownFiles(root, "docs", "dev")
	if _, err := os.Stat(filepath.Join(root, "README.md")); err == nil {
		linkFiles = append(linkFiles, "README.md")
	}
	pathFiles := append([]string{}, linkFiles...)
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err == nil {
		pathFiles = append(pathFiles, "CLAUDE.md")
	}
	pathFiles = append(pathFiles, markdownFiles(root, ".claude/rules")...)

	for _, f := range linkFiles {
		scan(root, f, func(line int, text string, _ []string) {
			for _, m := range linkRe.FindAllStringSubmatch(stripCode(text), -1) {
				if msg := checkLink(root, f, m[1]); msg != "" {
					problems = append(problems, fmt.Sprintf("%s:%d: %s", f, line, msg))
				}
			}
		})
	}
	for _, f := range pathFiles {
		scan(root, f, func(line int, text string, spans []string) {
			for _, s := range spans {
				if msg := checkPathSpan(root, f, s); msg != "" {
					problems = append(problems, fmt.Sprintf("%s:%d: %s", f, line, msg))
				}
			}
		})
	}
	sort.Strings(problems)
	return problems
}

// markdownFiles lists the .md files under the given directories of root.
func markdownFiles(root string, dirs ...string) []string {
	var files []string
	for _, d := range dirs {
		base := filepath.Join(root, filepath.FromSlash(d))
		_ = filepath.WalkDir(base, func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.HasSuffix(p, ".md") {
				return nil //nolint:nilerr // a missing directory just has no files
			}
			rel, relErr := filepath.Rel(root, p)
			if relErr == nil {
				files = append(files, filepath.ToSlash(rel))
			}
			return nil
		})
	}
	return files
}

// scan calls fn for every line outside fenced code blocks, with the contents of the inline code spans.
func scan(root, file string, fn func(line int, text string, spans []string)) {
	for i, l := range proseLines(root, file) {
		if l == "" {
			continue
		}
		var spans []string
		for _, m := range codeSpanRe.FindAllStringSubmatch(l, -1) {
			spans = append(spans, m[1])
		}
		fn(i+1, l, spans)
	}
}

// proseLines returns the lines of a file with the lines of fenced code blocks blanked out.
func proseLines(root, file string) []string {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file))) //nolint:gosec // the path comes from a directory walk
	if err != nil {
		return nil
	}
	lines := strings.Split(string(b), "\n")
	inFence := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inFence = !inFence
			lines[i] = ""
			continue
		}
		if inFence {
			lines[i] = ""
		}
	}
	return lines
}

func stripCode(s string) string { return codeSpanRe.ReplaceAllString(s, "") }

func checkLink(root, file, target string) string {
	if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
		return ""
	}
	p, anchor, _ := strings.Cut(target, "#")
	dest := file
	if p != "" {
		dest = path.Join(path.Dir(file), p)
	}
	if dest == ".." || strings.HasPrefix(dest, "../") {
		return fmt.Sprintf("link %q points outside the repository", target)
	}
	if strings.HasPrefix(file, "docs/") && (strings.HasPrefix(dest, "handoff/") || strings.HasPrefix(dest, "dev/")) {
		return fmt.Sprintf("link %q: docs/ must not refer to handoff/ or dev/", target)
	}
	st, err := os.Stat(filepath.Join(root, filepath.FromSlash(dest)))
	if err != nil {
		return fmt.Sprintf("link %q: %s does not exist", target, dest)
	}
	if anchor != "" && !st.IsDir() && strings.HasSuffix(dest, ".md") {
		if !anchors(root, dest)[strings.ToLower(anchor)] {
			return fmt.Sprintf("link %q: no heading with the anchor #%s in %s", target, anchor, dest)
		}
	}
	return ""
}

func checkPathSpan(root, file, span string) string {
	if strings.ContainsAny(span, "…<>*{} \t") {
		return ""
	}
	if strings.HasPrefix(file, "docs/") && (strings.HasPrefix(span, "handoff/") || strings.HasPrefix(span, "dev/")) {
		return fmt.Sprintf("`%s`: docs/ must not refer to handoff/ or dev/", span)
	}
	if !strings.HasPrefix(span, "docs/") {
		return ""
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(span))); err != nil {
		return fmt.Sprintf("`%s` does not exist", span)
	}
	return ""
}

// anchors returns the GitHub-style heading anchors of a Markdown file.
func anchors(root, file string) map[string]bool {
	set := map[string]bool{}
	seen := map[string]int{}
	for _, l := range proseLines(root, file) {
		m := headingRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		slug := slugify(m[1])
		if n := seen[slug]; n > 0 {
			set[fmt.Sprintf("%s-%d", slug, n)] = true
		} else {
			set[slug] = true
		}
		seen[slug]++
	}
	return set
}

func slugify(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.ReplaceAll(h, "`", "")) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || isLetterOrDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isLetterOrDigit(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func TestRepositoryDocs(t *testing.T) {
	for _, p := range check(filepath.Join("..", "..")) {
		t.Error(p)
	}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string // substrings, one per expected problem; empty means no problems
	}{
		{
			name: "valid links and anchors",
			files: map[string]string{
				"docs/a.md": "# A\n\nsee [b](b.md#section-two) and [self](#a)\n",
				"docs/b.md": "# B\n\n## Section two\n",
			},
		},
		{
			name:  "broken link",
			files: map[string]string{"docs/a.md": "see [x](missing.md)\n"},
			want:  []string{"docs/a.md:1: link \"missing.md\": docs/missing.md does not exist"},
		},
		{
			name: "broken anchor",
			files: map[string]string{
				"docs/a.md": "[b](b.md#nothing)\n",
				"docs/b.md": "# B\n",
			},
			want: []string{"docs/a.md:1: link \"b.md#nothing\": no heading"},
		},
		{
			name: "directory link",
			files: map[string]string{
				"docs/a.md":     "[dir](sub/)\n",
				"docs/sub/b.md": "# B\n",
			},
		},
		{
			name:  "link outside the repository",
			files: map[string]string{"README.md": "[x](../x.md)\n"},
			want:  []string{"README.md:1: link \"../x.md\" points outside"},
		},
		{
			name: "links in code and external URLs are ignored",
			files: map[string]string{
				"docs/a.md": "```\n[x](nope.md)\n```\n`[y](nope.md)` and [z](https://example.com/nope.md) [m](mailto:a@b.c)\n",
			},
		},
		{
			name: "docs must not refer to handoff or dev",
			files: map[string]string{
				"docs/a.md":      "[h](../handoff/x.md) and `dev/roadmap.md`\n",
				"handoff/x.md":   "# X\n",
				"dev/roadmap.md": "# R\n",
			},
			want: []string{
				"docs/a.md:1: link \"../handoff/x.md\": docs/ must not refer",
				"docs/a.md:1: `dev/roadmap.md`: docs/ must not refer",
			},
		},
		{
			name: "dev may refer to docs and handoff",
			files: map[string]string{
				"dev/a.md":     "[d](../docs/b.md) `docs/b.md` [h](../handoff/x.md)\n",
				"docs/b.md":    "# B\n",
				"handoff/x.md": "# X\n",
			},
		},
		{
			name: "docs paths in rules",
			files: map[string]string{
				"docs/ok.md":         "# OK\n",
				".claude/rules/r.md": "`docs/ok.md` `docs/gone.md` `docs/…` `docs/<name>.md` `docs/*.md`\n",
				"CLAUDE.md":          "`docs/` `docs/also-gone.md`\n",
			},
			want: []string{
				".claude/rules/r.md:1: `docs/gone.md` does not exist",
				"CLAUDE.md:1: `docs/also-gone.md` does not exist",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, body := range tt.files {
				p := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got := check(root)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d problems, want %d\ngot:  %q\nwant: %q", len(got), len(tt.want), got, tt.want)
			}
			sort.Strings(tt.want)
			for i := range got {
				if !strings.Contains(got[i], tt.want[i]) {
					t.Errorf("problem %d = %q, want it to contain %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSlugify(t *testing.T) {
	tests := map[string]string{
		"external":             "external",
		"ファイル名とセッション":          "ファイル名とセッション",
		"Section two":          "section-two",
		"`select` / `replace`": "select--replace",
		"A (b), c.":            "a-b-c",
	}
	for in, want := range tests {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
