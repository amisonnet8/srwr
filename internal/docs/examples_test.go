package docs

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/core"
	"github.com/amisonnet8/srwr/internal/mcp"
	"github.com/amisonnet8/srwr/internal/session"
)

var (
	tokenRe   = regexp.MustCompile(`sel_[0-9A-Za-z]+`)
	versionRe = regexp.MustCompile(`"version":"[^"]*"`)
)

// codeBlocks returns the contents of the fenced code blocks of a document that start with the given language.
func codeBlocks(t *testing.T, path, lang string) [][]string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // a path inside the repository
	if err != nil {
		t.Fatal(err)
	}
	var blocks [][]string
	var cur []string
	in := false
	for _, l := range strings.Split(string(b), "\n") {
		switch {
		case !in && strings.HasPrefix(l, "```"+lang):
			in, cur = true, nil
		case in && strings.HasPrefix(l, "```"):
			in = false
			blocks = append(blocks, cur)
		case in:
			cur = append(cur, l)
		}
	}
	return blocks
}

// normalize hides what changes from run to run: the tokens and the version.
func normalize(s string) string {
	return versionRe.ReplaceAllString(tokenRe.ReplaceAllString(s, "sel_TOKEN"), `"version":"VERSION"`)
}

// TestSelectReplaceExample runs the exchange in docs/examples/select-replace.md against the real
// server, in one workspace, and checks that every response is the one the document shows.
func TestSelectReplaceExample(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "examples", "select-replace.md")
	goBlocks := codeBlocks(t, path, "go")
	rpcBlocks := codeBlocks(t, path, "jsonrpc")
	if len(goBlocks) == 0 || len(rpcBlocks) == 0 {
		t.Fatalf("found %d go blocks and %d jsonrpc blocks", len(goBlocks), len(rpcBlocks))
	}

	root := t.TempDir()
	file := filepath.Join(root, "cmd", "main.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		t.Fatal(err)
	}
	// The first go block is the file the example starts from; the second is what it ends as.
	if err := os.WriteFile(file, []byte(strings.Join(goBlocks[0], "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ws, err := session.Open(root, session.Options{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	server := &mcp.Server{Core: &core.Core{WS: ws}, Version: "test"}

	tokens := map[string]string{} // token in the document -> token this run made
	var pending []string          // responses not yet matched with a line of the document
	exchanges := 0
	for _, block := range rpcBlocks {
		for _, line := range block {
			switch {
			case strings.HasPrefix(line, "→ "):
				req := strings.TrimPrefix(line, "→ ")
				for doc, actual := range tokens {
					req = strings.ReplaceAll(req, doc, actual)
				}
				var out bytes.Buffer
				if err := server.Serve(strings.NewReader(req+"\n"), &out); err != nil {
					t.Fatal(err)
				}
				for l := range strings.SplitSeq(out.String(), "\n") {
					if l != "" {
						pending = append(pending, l)
					}
				}
			case strings.HasPrefix(line, "← "):
				want := strings.TrimPrefix(line, "← ")
				if len(pending) == 0 {
					t.Fatalf("the document shows a response the server did not give:\n%s", want)
				}
				got := pending[0]
				pending = pending[1:]
				docTokens, gotTokens := tokenRe.FindAllString(want, -1), tokenRe.FindAllString(got, -1)
				if len(docTokens) != len(gotTokens) {
					t.Errorf("tokens: the document has %d, the server gave %d\n got %s\nwant %s", len(docTokens), len(gotTokens), got, want)
				}
				for i := range min(len(docTokens), len(gotTokens)) {
					tokens[docTokens[i]] = gotTokens[i]
				}
				if normalize(got) != normalize(want) {
					t.Errorf("response differs from the document\n got %s\nwant %s", normalize(got), normalize(want))
				}
				exchanges++
			}
		}
	}
	if len(pending) != 0 {
		t.Errorf("the server gave responses the document does not show: %q", pending)
	}
	if exchanges < 5 {
		t.Errorf("only %d exchanges were checked", exchanges)
	}

	// The document also shows the file after the replace.
	if len(goBlocks) > 1 {
		b, err := os.ReadFile(file) //nolint:gosec // a path in a temporary directory
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(b), strings.Join(goBlocks[1], "\n")+"\n"; got != want {
			t.Errorf("file after the example:\n%s\nthe document says:\n%s", got, want)
		}
	}
}
