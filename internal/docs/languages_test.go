package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var cjk = regexp.MustCompile(`[\p{Hiragana}\p{Katakana}\p{Han}]`)

// languageProblems checks the two languages of the documents under docs/ (.claude/rules/documentation.md): each document is in
// English (name.md) and in Japanese (name_ja.md), each has a link to the other at the top, and the English one has no Japanese
// except in code and in that link. It returns one message per problem, as "file: text".
func languageProblems(root string) []string {
	var problems []string
	files := markdownFiles(root, "docs")
	has := map[string]bool{}
	for _, f := range files {
		has[f] = true
	}
	for _, f := range files {
		ja := strings.HasSuffix(f, "_ja.md")
		other := strings.TrimSuffix(f, ".md") + "_ja.md"
		if ja {
			other = strings.TrimSuffix(f, "_ja.md") + ".md"
		}
		if !has[other] {
			problems = append(problems, f+": no "+filepath.Base(other)+" next to it")
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))) //nolint:gosec // a document of this repository
		if err != nil {
			problems = append(problems, f+": "+err.Error())
			continue
		}
		lines := strings.Split(string(b), "\n")
		if len(lines) < 3 || !strings.Contains(lines[2], "]("+filepath.Base(other)+")") {
			problems = append(problems, f+": the third line must link to "+filepath.Base(other))
		}
		if ja {
			continue
		}
		inFence := false
		for i, l := range lines {
			if strings.HasPrefix(l, "```") {
				inFence = !inFence
				continue
			}
			if inFence || i == 2 { // code blocks may hold a Japanese text; line 3 is the link to the Japanese version
				continue
			}
			if cjk.MatchString(stripCode(l)) {
				problems = append(problems, f+":"+strconv.Itoa(i+1)+": Japanese in an English document: "+strings.TrimSpace(l))
			}
		}
	}
	return problems
}

func TestEveryDocumentIsInTwoLanguages(t *testing.T) {
	for _, p := range languageProblems(filepath.Join("..", "..")) {
		t.Error(p)
	}
}

func TestLanguageProblems(t *testing.T) {
	root := t.TempDir()
	write := func(name, text string) {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/a.md", "# A\n\n[日本語](a_ja.md)\n\nplain `行` text\n\n```\n日本語 in a code block\n```\n")
	write("docs/a_ja.md", "# あ\n\n[English](a.md)\n")
	write("docs/b.md", "# B\n\n[日本語](b_ja.md)\n\nこれは日本語\n")
	write("docs/b_ja.md", "# び\n\nno link\n")
	write("docs/c.md", "# C\n\n[日本語](c_ja.md)\n")
	got := strings.Join(languageProblems(root), "\n")
	for _, want := range []string{"docs/b.md:5: Japanese in an English document", "docs/b_ja.md: the third line must link to b.md", "docs/c.md: no c_ja.md next to it"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, not := range []string{"docs/a.md", "docs/a_ja.md"} {
		if strings.Contains(got, not) {
			t.Errorf("%s is fine but reported:\n%s", not, got)
		}
	}
}
