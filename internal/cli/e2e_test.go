package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

// These tests run the real binary, built once, the way an AI client starts it: one process per
// client, speaking MCP over its standard input and output.

var (
	binOnce sync.Once
	binPath string
	binErr  error
	binDir  string
)

func TestMain(m *testing.M) {
	// The texts the tests expect are the Japanese ones (what srwr said before it spoke English by default).
	// lang_test.go sets SRWR_LANG back for the English ones. A child srwr inherits this.
	_ = os.Setenv("SRWR_LANG", "ja")
	code := m.Run()
	if binDir != "" {
		_ = os.RemoveAll(binDir)
	}
	os.Exit(code)
}

func binary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the binary")
	}
	binOnce.Do(func() {
		dir, err := os.MkdirTemp("", "srwr-e2e-")
		if err != nil {
			binErr = err
			return
		}
		binDir = dir
		name := "srwr"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		binPath = filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-o", binPath, "../../cmd/srwr") //nolint:gosec // fixed arguments
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			binErr = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return binPath
}

// client is a running srwr mcp.
type client struct {
	t      *testing.T
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Reader
	nextID int
}

func startClient(t *testing.T, root string) *client {
	t.Helper()
	return startClientCmd(t, exec.Command(binary(t), "mcp", "--root", root)) //nolint:gosec // the binary was built by this test
}

// startClientCmd runs cmd, which has to be srwr mcp, and talks to it.
func startClientCmd(t *testing.T, cmd *exec.Cmd) *client {
	t.Helper()
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &client{t: t, cmd: cmd, in: in, out: bufio.NewReader(out)}
	t.Cleanup(c.stop)
	return c
}

func (c *client) stop() {
	_ = c.in.Close()
	_ = c.cmd.Wait()
}

// request sends one request and returns the response line.
func (c *client) request(method, params string) string {
	c.t.Helper()
	c.nextID++
	if _, err := fmt.Fprintf(c.in, `{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`+"\n", c.nextID, method, params); err != nil {
		c.t.Fatal(err)
	}
	line, err := c.out.ReadString('\n')
	if err != nil {
		c.t.Fatalf("reading the response to %s: %v", method, err)
	}
	return line
}

func (c *client) initialize() {
	c.t.Helper()
	line := c.request("initialize", `{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"e2e","version":"1"}}`)
	if !strings.Contains(line, `"serverInfo":{"name":"srwr"`) {
		c.t.Fatalf("initialize: %s", line)
	}
	if _, err := io.WriteString(c.in, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"); err != nil {
		c.t.Fatal(err)
	}
}

// call calls a tool and returns the decoded body and isError.
func (c *client) call(name string, args map[string]any) (map[string]any, bool) {
	c.t.Helper()
	a, _ := json.Marshal(args)
	line := c.request("tools/call", fmt.Sprintf(`{"name":%q,"arguments":%s}`, name, a))
	var r struct {
		Result struct {
			Content []struct{ Text string }
			IsError bool
		}
		Error *struct{ Message string }
	}
	if err := json.Unmarshal([]byte(line), &r); err != nil || r.Error != nil || len(r.Result.Content) != 1 {
		c.t.Fatalf("response to %s: %s (%v)", name, line, err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(r.Result.Content[0].Text), &m); err != nil {
		c.t.Fatalf("body of %s: %v", line, err)
	}
	return m, r.Result.IsError
}

func (c *client) mustSelect(file string, start, end int) string {
	c.t.Helper()
	m, isErr := c.call("select", map[string]any{"file": file, "startLine": start, "endLine": end, "why": "見る"})
	if isErr {
		c.t.Fatalf("select %s %d..%d: %v", file, start, end, m)
	}
	return m["selection"].(string)
}

func (c *client) mustReplace(token, newText string) {
	c.t.Helper()
	m, isErr := c.call("replace", map[string]any{"selection": token, "newText": newText, "why": "変える"})
	if isErr {
		c.t.Fatalf("replace: %v", m)
	}
}

func write(t *testing.T, root, rel, text string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel)) //nolint:gosec // a path in a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// tapes returns the tape files of a workspace.
func tapes(t *testing.T, root string) []string {
	t.Helper()
	var m []string
	for _, id := range tape.IDs(filepath.Join(root, ".srwr", "tapes")) {
		if path, ok := tape.Find(filepath.Join(root, ".srwr", "tapes"), id); ok {
			m = append(m, path)
		}
	}
	return m
}

func readTape(t *testing.T, path string) []tape.Event {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // a path in a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	res := tape.Parse(b)
	if res.Skipped != 0 {
		t.Fatalf("%d lines of the tape cannot be read", res.Skipped)
	}
	return res.Events
}

// checkSeqs checks that the events after the header are numbered 1, 2, 3, … with nothing missing.
func checkSeqs(t *testing.T, events []tape.Event) {
	t.Helper()
	if len(events) == 0 || events[0].Type != tape.TypeHeader {
		t.Fatalf("the tape does not start with a header: %+v", events)
	}
	for i, e := range events[1:] {
		if e.Seq != i+1 {
			t.Fatalf("event %d has seq %d, want %d", i, e.Seq, i+1)
		}
		if e.Type == tape.TypeHeader {
			t.Fatalf("a second header at event %d", i)
		}
	}
}

// Stage condition 1: a token from one srwr mcp works in another.
func TestTokenWorksInAnotherProcess(t *testing.T) {
	root := t.TempDir()
	write(t, root, "main.go", "package main\n\nfunc main() {\n\trun()\n}\n")
	a, b := startClient(t, root), startClient(t, root)
	a.initialize()
	b.initialize()

	tok := a.mustSelect("main.go", 3, 5)
	b.mustReplace(tok, "func main() {\n\tsetup()\n\trun()\n}")
	tok2 := b.mustSelect("main.go", 1, 1)
	a.mustReplace(tok2, "package app")
	if got, want := read(t, root, "main.go"), "package app\n\nfunc main() {\n\tsetup()\n\trun()\n}\n"; got != want {
		t.Errorf("main.go = %q, want %q", got, want)
	}

	files := tapes(t, root)
	if len(files) != 1 {
		t.Fatalf("tapes = %v, want one", files)
	}
	events := readTape(t, files[0])
	checkSeqs(t, events)
	var kinds []string
	for _, e := range events[1:] {
		kinds = append(kinds, e.Type)
	}
	if want := []string{"snapshot", "select", "replace", "select", "replace"}; !slices.Equal(kinds, want) {
		t.Errorf("tape = %v, want %v", kinds, want)
	}
	if h := events[0]; h.Author == nil || h.Author.Name != "e2e" || h.Tool == nil || h.Tool.Name != "srwr" {
		t.Errorf("header = %+v", h)
	}
	if st := tape.Build(events); st.Files["main.go"].Text != read(t, root, "main.go") {
		t.Error("the tape does not replay to the file")
	}
}

// Stage condition 4: starting several together does not fail on making the key.
func TestStartedTogether(t *testing.T) {
	root := t.TempDir()
	const n = 8
	for i := range n {
		write(t, root, fmt.Sprintf("f%d.txt", i), "0\n")
	}
	clients := make([]*client, n)
	for i := range clients {
		clients[i] = startClient(t, root)
	}

	// Everybody initializes, then everybody makes the first call at once.
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, c := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			c.initialize()
			for r := 1; r <= 3; r++ {
				tok := c.mustSelect(fmt.Sprintf("f%d.txt", i), 1, 1)
				c.mustReplace(tok, fmt.Sprint(r))
			}
		}()
	}
	close(start)
	wg.Wait()
	if t.Failed() {
		return
	}

	for i := range n {
		if got := read(t, root, fmt.Sprintf("f%d.txt", i)); got != "3\n" {
			t.Errorf("f%d.txt = %q", i, got)
		}
	}
	key, err := os.ReadFile(filepath.Join(root, ".srwr", "key")) //nolint:gosec // a path in a temporary directory
	if err != nil || len(key) != 32 {
		t.Errorf("key: %d bytes, %v", len(key), err)
	}
	files := tapes(t, root)
	if len(files) != 1 {
		t.Fatalf("tapes = %v, want one", files)
	}
	events := readTape(t, files[0])
	checkSeqs(t, events)
	if want := 1 + n*(1+6); len(events) != want {
		t.Errorf("tape has %d events, want %d", len(events), want)
	}
	entries, _ := os.ReadDir(filepath.Join(root, ".srwr"))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("a temporary file is left: %s", e.Name())
		}
	}
}

var tsRe = regexp.MustCompile(`"(ts|startedAt)":"[^"]*"`)

// Stage conditions 2 and 3: after a long pause the session is a new one, and the old tokens are refused.
// The pause is made by dating the tape back, because the length of a session is fixed at 30 minutes.
// The new dates are shorter, so that the running server notices the tape was replaced.
func TestNewSessionAfterAPause(t *testing.T) {
	root := t.TempDir()
	write(t, root, "f.txt", "1\n2\n")
	a := startClient(t, root)
	a.initialize()
	tok := a.mustSelect("f.txt", 1, 1)

	files := tapes(t, root)
	if len(files) != 1 {
		t.Fatalf("tapes = %v", files)
	}
	b, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	old := tsRe.ReplaceAllString(string(b), `"$1":"2020-01-01T00:00:00Z"`)
	if err := os.WriteFile(files[0], []byte(old), 0o600); err != nil { //nolint:gosec // a path in a temporary directory
		t.Fatal(err)
	}

	m, isErr := a.call("replace", map[string]any{"selection": tok, "newText": "x", "why": "w"})
	if code := m["error"].(map[string]any)["code"]; !isErr || code != "invalid_selection" {
		t.Fatalf("replace with the token of the old session: %v", m)
	}
	if got := read(t, root, "f.txt"); got != "1\n2\n" {
		t.Errorf("f.txt = %q", got)
	}

	tok2 := a.mustSelect("f.txt", 1, 1)
	a.mustReplace(tok2, "one")
	files = tapes(t, root)
	if len(files) != 2 {
		t.Fatalf("tapes = %v, want the old one and a new one", files)
	}
	if closed := slices.IndexFunc(files, func(f string) bool { return strings.HasSuffix(f, tape.GzSuffix) }); closed < 0 || files[closed] == files[1-closed] {
		t.Errorf("the tape of the session that ended is not compressed: %v", files)
	}
	active, _ := os.ReadFile(filepath.Join(root, ".srwr", "active")) //nolint:gosec // a path in a temporary directory
	newTape := filepath.Join(root, ".srwr", "tapes", tape.FileName(strings.TrimSpace(string(active))))
	if newTape == files[0] && newTape == files[1] {
		t.Fatal("active does not point at a tape")
	}
	events := readTape(t, newTape)
	checkSeqs(t, events)
	var n int
	for _, e := range events {
		if e.Type != tape.TypeFailure {
			n++
		}
	}
	if n != 4 { // header, snapshot, select, replace (and the failure of the token of the old session)
		t.Errorf("the new tape has %d events besides failures, want 4", n)
	}
}

func TestRootDefaultsToTheCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	write(t, root, "f.txt", "1\n")
	cmd := exec.Command(binary(t), "mcp") //nolint:gosec // the binary was built by this test
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"select","arguments":{"file":"f.txt","startLine":1,"endLine":1,"why":"w"}}}` + "\n")
	out, err := cmd.Output()
	if err != nil || !strings.Contains(string(out), `\"ok\":true`) {
		t.Errorf("output = %s, err = %v", out, err)
	}
	if len(tapes(t, root)) != 1 {
		t.Error("the tape is not in the current directory")
	}
}

// viewer is a running srwr view-server.
func startViewer(t *testing.T, root string) *client {
	t.Helper()
	cmd := exec.Command(binary(t), "view-server", "--root", root) //nolint:gosec // the binary was built by this test
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &client{t: t, cmd: cmd, in: in, out: bufio.NewReader(out)}
	t.Cleanup(c.stop)
	return c
}

// The display server reads the tape that srwr mcp wrote.
func TestViewServerShowsWhatMCPWrote(t *testing.T) {
	root := t.TempDir()
	write(t, root, "main.go", "package main\n\nfunc main() {\n\trun()\n}\n")
	m := startClient(t, root)
	m.initialize()
	tok := m.mustSelect("main.go", 3, 5)
	m.mustReplace(tok, "func main() {\n\tsetup()\n\trun()\n}")

	v := startViewer(t, root)
	var init struct{ Result struct{ ProtocolVersion int } }
	if err := json.Unmarshal([]byte(v.request("initialize", `{"client":"vim","protocolVersion":1}`)), &init); err != nil || init.Result.ProtocolVersion != 1 {
		t.Fatalf("initialize: %+v %v", init, err)
	}

	var list struct {
		Result struct {
			Tapes []struct {
				TapeID string
				Ops    int
				Files  []string
			}
		}
	}
	if err := json.Unmarshal([]byte(v.request("tapes/list", `{}`)), &list); err != nil || len(list.Result.Tapes) != 1 {
		t.Fatalf("tapes/list: %+v %v", list, err)
	}
	ti := list.Result.Tapes[0]
	if ti.Ops != 2 || !slices.Equal(ti.Files, []string{"main.go"}) {
		t.Errorf("tape = %+v", ti)
	}

	var opened struct {
		Result struct {
			Frames []struct {
				Kind          string
				Before, After string
				Range         struct{ Start, End int }
				Why           string
			}
		}
	}
	line := v.request("tape/open", fmt.Sprintf(`{"tapeId":%q,"withText":true}`, ti.TapeID))
	if err := json.Unmarshal([]byte(line), &opened); err != nil || len(opened.Result.Frames) != 2 {
		t.Fatalf("tape/open: %s %v", line, err)
	}
	f := opened.Result.Frames[1]
	if f.Kind != "replace" || f.Why != "変える" || f.Range.Start != 3 || f.Range.End != 6 ||
		f.Before != "package main\n\nfunc main() {\n\trun()\n}\n" || f.After != "package main\n\nfunc main() {\n\tsetup()\n\trun()\n}\n" {
		t.Errorf("frame = %+v", f)
	}

	if got := v.request("shutdown", `{}`); !strings.Contains(got, `"result":{}`) {
		t.Errorf("shutdown: %s", got)
	}
	// It ends by itself after the response.
	if err := v.cmd.Wait(); err != nil {
		t.Errorf("view-server exited with %v", err)
	}
}

// An external change is on the tape as the lines that changed, with no snapshot after it, and the tape still replays to the file.
func TestExternalChangeIsHunksAndReplays(t *testing.T) {
	root := t.TempDir()
	write(t, root, "f.txt", "1\n2\n3\n4\n5\n6\n7\n8\n")
	c := startClient(t, root)
	c.initialize()
	c.mustSelect("f.txt", 1, 1)
	write(t, root, "f.txt", "1\nTWO\n3\n4\n5\n6\n7\n8\n9\n") // two places, not by srwr
	tok := c.mustSelect("f.txt", 1, 1)
	c.mustReplace(tok, "one")

	events := readTape(t, tapes(t, root)[0])
	checkSeqs(t, events)
	var kinds []string
	for _, e := range events[1:] {
		kinds = append(kinds, e.Type)
	}
	if want := []string{"snapshot", "select", "external", "select", "replace"}; !slices.Equal(kinds, want) {
		t.Fatalf("tape = %v, want %v", kinds, want)
	}
	if x := events[3]; x.Text != nil || len(x.Hunks) != 2 {
		t.Errorf("external = %+v, want two hunks and no text", x)
	}
	if st := tape.Build(events); st.Files["f.txt"].Text != read(t, root, "f.txt") {
		t.Error("the tape does not replay to the file")
	}
}

// A call that failed is on the tape of the real srwr mcp as a failure with no real path, and it is not a step of the replay.
func TestFailedCallIsOnTheTapeAndNotAFrame(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.go", "package a\n")
	c := startClient(t, root)
	c.initialize()
	if m, isErr := c.call("select", map[string]any{"file": filepath.Join(root, "a.go"), "startLine": 1, "endLine": 1, "why": "見る"}); !isErr || m["error"].(map[string]any)["code"] != "invalid_range" {
		t.Fatalf("select with an absolute path: %v (isError %v)", m, isErr)
	}
	c.mustSelect("a.go", 1, 1)

	events := readTape(t, tapes(t, root)[0])
	checkSeqs(t, events)
	var failures []tape.Event
	for _, e := range events {
		if e.Type == tape.TypeFailure {
			failures = append(failures, e)
		}
	}
	if len(failures) != 1 || failures[0].Failure.File != nil || failures[0].Failure.Code != "invalid_range" || strings.Contains(failures[0].Failure.Message, root) {
		t.Fatalf("failures = %+v", failures)
	}
	if st := tape.Build(events); st.Files["a.go"].Text != "package a\n" {
		t.Error("the tape does not replay to the file")
	}
	english(t)
	if code, out, errOut := tapesOut(t, root); code != 0 || errOut != "" || !strings.Contains(out, "1 tapes") && !strings.Contains(out, "1 tape") {
		t.Errorf("srwr tapes: code %d, stderr %q\n%s", code, errOut, out)
	}
}
