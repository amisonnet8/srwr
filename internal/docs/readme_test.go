package docs

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The READMEs are the front door: the root one and the one of the extension, each in English and in Japanese
// (.claude/rules/documentation.md). They start with a centered block (a picture, badges), so the switch to the other language
// sits in that block and not on the third line like the documents under docs/.

var readmeFiles = []string{"README.md", "README_ja.md", "extension/README.md", "extension/README_ja.md"}

var (
	htmlRefRe  = regexp.MustCompile(`(?:src|srcset|href)="([^"]+)"`)
	switchEnRe = regexp.MustCompile(`日本語</a> \| <b>English</b>`)
	switchJaRe = regexp.MustCompile(`English</a> \| <b>日本語</b>`)
)

// readmeProblems checks the four READMEs under root: they exist, the switch is near the top, the English ones have no
// Japanese outside the switch and code, and the pictures and relative links they use exist. It returns "file:line: text".
func readmeProblems(root string) []string {
	var problems []string
	for _, f := range readmeFiles {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))) //nolint:gosec // a README of this repository
		if err != nil {
			problems = append(problems, f+": "+err.Error())
			continue
		}
		ja := strings.HasSuffix(f, "_ja.md")
		lines := strings.Split(string(b), "\n")
		switchRe := switchEnRe
		if ja {
			switchRe = switchJaRe
		}
		found := false
		for i, line := range lines {
			if i < 12 && switchRe.MatchString(line) {
				found = true
			}
		}
		if !found {
			problems = append(problems, f+": no switch to the other language in the first 12 lines")
		}
		fence := false
		for i, line := range lines {
			at := fmt.Sprintf("%s:%d: ", f, i+1)
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				fence = !fence
			}
			if !ja && !fence && !switchRe.MatchString(line) && cjk.MatchString(stripCode(line)) && !strings.HasPrefix(strings.TrimSpace(line), "```") {
				problems = append(problems, at+"Japanese in an English README: "+strings.TrimSpace(line))
			}
			for _, ref := range refsOf(line) {
				if p := checkReadmeRef(root, f, ref); p != "" {
					problems = append(problems, at+p)
				}
			}
		}
	}
	return problems
}

// refsOf returns the targets of the Markdown links and images and of the HTML attributes src, srcset and href in a line.
func refsOf(line string) []string {
	var out []string
	for _, m := range linkRe.FindAllStringSubmatch(stripCode(line), -1) {
		out = append(out, m[1])
	}
	for _, m := range htmlRefRe.FindAllStringSubmatch(line, -1) {
		out = append(out, m[1])
	}
	return out
}

// checkReadmeRef checks one target: a file or a picture of the repository, relative to the README; or an anchor in the README.
func checkReadmeRef(root, file, ref string) string {
	if strings.Contains(ref, "://") || strings.HasPrefix(ref, "mailto:") || ref == "#" {
		return ""
	}
	target, frag, _ := strings.Cut(ref, "#")
	if target == "" {
		if !anchors(root, file)[frag] {
			return "no heading for the anchor #" + frag
		}
		return ""
	}
	full := filepath.Join(root, filepath.FromSlash(path.Join(path.Dir(file), target)))
	if _, err := os.Stat(full); err != nil {
		return "missing: " + ref
	}
	return ""
}

func TestREADMEsAreInTwoLanguagesAndTheirPicturesExist(t *testing.T) {
	for _, p := range readmeProblems(filepath.Join("..", "..")) {
		t.Error(p)
	}
}

func TestReadmeProblems(t *testing.T) {
	write := func(root, name, text string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	en := `<p><a href="README_ja.md">日本語</a> | <b>English</b></p>` + "\n"
	ja := `<p><a href="README.md">English</a> | <b>日本語</b></p>` + "\n"
	good := t.TempDir()
	write(good, "README.md", en+"# A\n[a](#a) ![p](pic.svg) <img src=\"pic.svg\">\n```\n日本語 in code\n```\n")
	write(good, "README_ja.md", ja+"日本語\n")
	write(good, "extension/README.md", en)
	write(good, "extension/README_ja.md", ja)
	write(good, "pic.svg", "<svg/>")
	if got := readmeProblems(good); len(got) != 0 {
		t.Errorf("a good set has problems: %v", got)
	}
	bad := t.TempDir()
	write(bad, "README.md", "no switch\n日本語が混ざった\n<img src=\"missing.png\"> [x](#nope)\n")
	write(bad, "README_ja.md", ja)
	write(bad, "extension/README.md", en)
	for _, want := range []string{"no switch", "Japanese in an English README", "missing: missing.png", "no heading for the anchor #nope", "extension/README_ja.md: "} {
		found := false
		for _, p := range readmeProblems(bad) {
			if strings.Contains(p, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("no problem mentioning %q in %v", want, readmeProblems(bad))
		}
	}
}
