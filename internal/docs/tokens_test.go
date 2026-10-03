package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var selectionRe = regexp.MustCompile("sel_[0-9A-Za-z]+")

// A selection token in a document must look like a real one: the smallest real token is 15 bytes, which is 24 characters of
// Crockford Base32 (docs/design/token.md), and the alphabet has no I, L, O or U. An example that is too short teaches the
// wrong shape to the AI.
func tokenProblems(root string) []string {
	var problems []string
	for _, f := range markdownFiles(root, "docs") {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))) //nolint:gosec // a document of this repository
		if err != nil {
			problems = append(problems, f+": "+err.Error())
			continue
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, tok := range selectionRe.FindAllString(line, -1) {
				body := strings.TrimPrefix(tok, "sel_")
				if len(body) < 24 || len(body) > 32 || strings.ContainsAny(strings.ToUpper(body), "ILOU") || strings.ToUpper(body) != body {
					problems = append(problems, f+":"+strconv.Itoa(i+1)+": "+tok+" is not shaped like a real selection token (24 to 32 upper-case Base32 characters)")
				}
			}
		}
	}
	return problems
}

func TestSelectionTokensInDocumentsLookReal(t *testing.T) {
	for _, p := range tokenProblems("../..") {
		t.Error(p)
	}
}

func TestTokenProblemsFindsShortToken(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	doc := "ok sel_0410R3GZE4KV11C6325D32S7 and short sel_7K3M9QX2F4HD8R1WTB\n"
	if err := os.WriteFile(filepath.Join(root, "docs", "a.md"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	got := tokenProblems(root)
	if len(got) != 1 || !strings.Contains(got[0], "sel_7K3M9QX2F4HD8R1WTB") {
		t.Errorf("problems = %v", got)
	}
}
