package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func load(t *testing.T, srwrignore string) *Matcher {
	t.Helper()
	root := t.TempDir()
	if srwrignore != "" {
		if err := os.WriteFile(filepath.Join(root, FileName), []byte(srwrignore), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestBuiltin(t *testing.T) {
	m := load(t, "")
	for rel, want := range map[string]bool{
		".env": true, "a/.env": true, ".env.local": true, "sub/dir/.env.production": true,
		"k/x.pem": true, "x.key": true, "id_rsa": true, "id_rsa.pub": true, "home/id_ed25519": true, "id_ed25519.pub": true,
		"cert.p12": true, "cert.pfx": true, ".srwr/key": true, ".srwr/tapes/a.tape.jsonl": true, "a/.srwr/x": true,
		".ENV": true, "A/X.PEM": true, ".Srwr/Key": true,
		"env.txt": false, "keys.go": false, "a.pem.txt": false, "main.go": false, "dotenv": false, "x/.environment": false,
		".srwrignore": false, "srwr/key": false, "my_id_rsa": false,
	} {
		if got := m.Match(rel); got != want {
			t.Errorf("Match(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestSrwrignoreSyntax(t *testing.T) {
	tests := []struct {
		name  string
		lines string
		rel   string
		want  bool
	}{
		{"plain name, deep", "secret.txt\n", "a/b/secret.txt", true},
		{"plain name, other", "secret.txt\n", "a/secret.txt.bak", false},
		{"comment", "# secret.txt\n", "secret.txt", false},
		{"blank lines", "\n\n   \nsecret.txt\n", "secret.txt", true},
		{"escaped hash", "\\#note\n", "#note", true},
		{"escaped bang", "\\!note\n", "!note", true},
		{"trailing spaces dropped", "secret.txt   \n", "secret.txt", true},
		{"escaped trailing space", "name\\ \n", "name ", true},
		{"crlf file", "secret.txt\r\n", "secret.txt", true},
		{"directory only: the directory", "build/\n", "build/out.js", true},
		{"directory only: deep", "build/\n", "x/build/out.js", true},
		{"directory only: a file of that name", "build/\n", "build", false},
		{"leading slash anchors", "/top.txt\n", "top.txt", true},
		{"leading slash does not match deeper", "/top.txt\n", "a/top.txt", false},
		{"middle slash anchors", "a/b.txt\n", "a/b.txt", true},
		{"middle slash does not match deeper", "a/b.txt\n", "x/a/b.txt", false},
		{"star in a segment", "docs/*.md\n", "docs/x.md", true},
		{"star does not cross a slash", "docs/*.md\n", "docs/sub/x.md", false},
		{"question mark", "f?.txt\n", "fa.txt", true},
		{"question mark needs one char", "f?.txt\n", "f.txt", false},
		{"bracket", "f[ab].txt\n", "fb.txt", true},
		{"bracket miss", "f[ab].txt\n", "fc.txt", false},
		{"negated bracket", "f[!ab].txt\n", "fc.txt", true},
		{"negated bracket miss", "f[!ab].txt\n", "fa.txt", false},
		{"leading double star", "**/x.txt\n", "x.txt", true},
		{"leading double star, deep", "**/x.txt\n", "a/b/x.txt", true},
		{"trailing double star", "a/**\n", "a/b/c.txt", true},
		{"trailing double star is not the directory itself", "a/**\n", "a", false},
		{"middle double star, zero", "a/**/b\n", "a/b", true},
		{"middle double star, many", "a/**/b\n", "a/x/y/b", true},
		{"middle double star, miss", "a/**/b\n", "a/x/y/c", false},
		{"negation brings a file back", "*.txt\n!keep.txt\n", "keep.txt", false},
		{"negation leaves others", "*.txt\n!keep.txt\n", "other.txt", true},
		{"last rule wins", "!keep.txt\n*.txt\n", "keep.txt", true},
		{"cannot bring back from an ignored directory", "build/\n!build/keep.txt\n", "build/keep.txt", true},
		{"case does not matter", "Secret.TXT\n", "sub/secret.txt", true},
		{"nothing", "", "main.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := load(t, tt.lines).Match(tt.rel); got != tt.want {
				t.Errorf("Match(%q) with %q = %v, want %v", tt.rel, tt.lines, got, tt.want)
			}
		})
	}
}

func TestBuiltinCannotBeUndone(t *testing.T) {
	m := load(t, "!.env\n!*.pem\n!.srwr/\n")
	for _, rel := range []string{".env", "k/x.pem", ".srwr/key"} {
		if !m.Match(rel) {
			t.Errorf("Match(%q) = false: a ! line brought back a built-in pattern", rel)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, FileName), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Error("Load with a directory named .srwrignore = nil error, want an error")
	}
}
