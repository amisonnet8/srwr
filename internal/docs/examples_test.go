package docs

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/core"
	"github.com/amisonnet8/srwr/internal/hook"
	"github.com/amisonnet8/srwr/internal/mcp"
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/viewserver"
)

var (
	tokenRe   = regexp.MustCompile(`sel_[0-9A-Za-z]+`)
	versionRe = regexp.MustCompile(`"(version|serverVersion)":"[^"]*"`)
	updatedRe = regexp.MustCompile(`"updatedAt":"[^"]*"`)
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

// normalize hides what changes from run to run: the tokens, the versions and the time a tape file was last written.
func normalize(s string) string {
	s = tokenRe.ReplaceAllString(s, "sel_TOKEN")
	s = updatedRe.ReplaceAllString(s, `"updatedAt":"TIME"`)
	return versionRe.ReplaceAllString(s, `"$1":"VERSION"`)
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

// TestProtocolSessionExample runs the exchanges in docs/examples/protocol-session.md against the real
// display server. Each block is a connection of its own, on the workspace in testdata/demo, which
// is the tape that the exchange in select-replace.md leaves (made by the real core, run with a fixed clock).
func TestProtocolSessionExample(t *testing.T) {
	blocks := codeBlocks(t, filepath.Join("..", "..", "docs", "examples", "protocol-session.md"), "jsonrpc")
	if len(blocks) < 2 {
		t.Fatalf("found %d jsonrpc blocks, want at least 2", len(blocks))
	}
	root, err := filepath.Abs(filepath.Join("testdata", "demo"))
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for n, block := range blocks {
		var requests, expected []string
		for _, line := range block {
			switch {
			case strings.HasPrefix(line, "→ "):
				requests = append(requests, strings.TrimPrefix(line, "→ "))
			case strings.HasPrefix(line, "← "):
				expected = append(expected, strings.TrimPrefix(line, "← "))
			}
		}
		var out bytes.Buffer
		srv := &viewserver.Server{Root: root, Version: "test"}
		if err := srv.Serve(strings.NewReader(strings.Join(requests, "\n")+"\n"), &out); err != nil {
			t.Fatalf("block %d: %v", n, err)
		}
		var got []string
		for l := range strings.SplitSeq(out.String(), "\n") {
			if l != "" {
				got = append(got, l)
			}
		}
		if len(got) != len(expected) {
			t.Fatalf("block %d: the server gave %d responses, the document shows %d\n%q", n, len(got), len(expected), got)
		}
		for i := range got {
			if normalize(got[i]) != normalize(expected[i]) {
				t.Errorf("block %d, response %d differs from the document\n got %s\nwant %s", n, i, normalize(got[i]), normalize(expected[i]))
			}
			checked++
		}
	}
	if checked < 8 {
		t.Errorf("only %d responses were checked", checked)
	}
}

// tapeTS hides the time of an event.
var tapeTS = regexp.MustCompile(`"ts":"[^"]*"`)

// TestHookExample runs the hook calls in docs/examples/hook.md against the real srwr hook, in one workspace, and checks that the
// tape they leave is the one the document shows. An Edit has already changed the file when the hook runs, so the test makes
// the change first, as Claude Code does.
func TestHookExample(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "examples", "hook.md")
	goBlocks := codeBlocks(t, path, "go")
	hookBlocks := codeBlocks(t, path, "hook")
	tapeBlocks := codeBlocks(t, path, "jsonl")
	if len(goBlocks) != 2 || len(hookBlocks) != 1 || len(tapeBlocks) != 1 {
		t.Fatalf("found %d go blocks, %d hook blocks and %d jsonl blocks", len(goBlocks), len(hookBlocks), len(tapeBlocks))
	}
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	file := filepath.Join(root, "cmd", "main.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(strings.Join(goBlocks[0], "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ws, err := session.Open(root, session.Options{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	ws.SetAuthor(tape.Author{Kind: "ai", Name: "claude"})
	c := &core.Core{WS: ws}
	calls := 0
	for _, line := range hookBlocks[0] {
		if !strings.HasPrefix(line, "→ ") {
			continue
		}
		in := strings.ReplaceAll(strings.TrimPrefix(line, "→ "), "/work", filepath.ToSlash(root))
		var msg struct {
			Tool  string `json:"tool_name"`
			Input struct {
				Old string `json:"old_string"`
				New string `json:"new_string"`
			} `json:"tool_input"`
		}
		if err := json.Unmarshal([]byte(in), &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Tool == "Edit" {
			b, err := os.ReadFile(file) //nolint:gosec // a path in a temporary directory
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte(strings.Replace(string(b), msg.Input.Old, msg.Input.New, 1)), 0o600); err != nil { //nolint:gosec // a path in a temporary directory
				t.Fatal(err)
			}
		}
		if _, err := hook.Run(strings.NewReader(in), c); err != nil {
			t.Fatal(err)
		}
		calls++
	}
	if calls != 3 {
		t.Fatalf("the document shows %d calls", calls)
	}

	active, err := os.ReadFile(filepath.Join(root, ".srwr", "active")) //nolint:gosec // a path in a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".srwr", "tapes", tape.FileName(strings.TrimSpace(string(active))))) //nolint:gosec // a path in a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for i, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if i > 0 { // the header holds the time and the session of this run
			got = append(got, tapeTS.ReplaceAllString(l, `"ts":"TS"`))
		}
	}
	var want []string
	for _, l := range tapeBlocks[0] {
		want = append(want, tapeTS.ReplaceAllString(l, `"ts":"TS"`))
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the tape differs from the document.\nthe tape:\n%s\nthe document:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	b, err := os.ReadFile(file) //nolint:gosec // a path in a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Join(goBlocks[1], "\n") + "\n"; string(b) != want {
		t.Errorf("file after the example:\n%s\nthe document says:\n%s", b, want)
	}
}
