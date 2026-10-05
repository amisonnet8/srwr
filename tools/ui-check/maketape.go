package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// longWhyTape is the tape qsoku ui-check makes on the spot: a select and a replace with a why too long for one row, made by
// the real `srwr mcp` (a hand-written tape would not have the shape of a real one). It is named by a fixed id, so that the
// screens are the same every time.
const longWhyTape = "20260101-0000-long-why"

const (
	longSelectWhy  = "これは折り返しの確認のための、とても長い理由です。幅を超えると何行かに分かれ、2行目からは字下げされて、全文が読めるはずです。さらに続けて、三行目にも届くようにします。"
	longReplaceWhy = "置き換えの理由も長くします。橙の理由の行が、幅を超えたときに何行かに分かれ、2行目からは字下げされ、全文が読めて、範囲がその直下に見えることを確かめるための文です。"
)

// longSubWhy is the why of the replace that ends the long-why tape: long enough to be more than one row, since it is shown above
// each of the three places the replace changed.
const longSubWhy = "エラー変数の名前を、公開する変数の命名規則に合わせて ErrEmpty に改める。3か所が離れているので、理由はそれぞれの変更の直前に出る。"

// storeSource is the file the replace of the long-why tape works on: three places of errEmpty, far enough apart that the first
// of them is below the top of a window.
const storeSource = `package store

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	items map[string]string
}

func (s *Store) Get(id string) (string, error) {
	if id == "" {
		return "", errEmpty
	}
	v, ok := s.items[id]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *Store) Load(id string) error {
	if _, err := s.Get(id); err != nil {
		return fmt.Errorf("load: %w", errEmpty)
	}
	return nil
}

func (s *Store) Put(id, v string) error {
	if id == "" {
		return errEmpty
	}
	s.items[id] = v
	return nil
}

func (s *Store) Len() int { return len(s.items) }
`

// makeLongWhyTape makes, in extra, the workspace pieces of the tape (a.go and .srwr/tapes/<longWhyTape>.tape.jsonl), which can be
// copied over the fixed workspace. bin is the srwr binary; the work happens in a temporary directory.
func makeLongWhyTape(bin, extra string) error {
	return makeTape(bin, extra, longWhyTape, func(c *mcpClient) error {
		sel, err := c.tool("look", map[string]any{"file": "a.go", "startLine": 3, "endLine": 3, "why": longSelectWhy})
		if err != nil {
			return err
		}
		token, _ := sel["selection"].(string)
		if _, err = c.tool("edit", map[string]any{"selection": token, "newText": mainReplacement, "why": longReplaceWhy}); err != nil {
			return err
		}
		// A file with three places to rename, for a replace frame: the why is shown above each of the places.
		if _, err = c.tool("new", map[string]any{"file": "store.go", "content": storeSource, "why": "名前を変える対象のファイルを作る"}); err != nil {
			return err
		}
		_, err = c.tool("replace", map[string]any{"files": []string{"store.go"}, "old": "errEmpty", "new": "ErrEmpty", "count": 3, "why": longSubWhy})
		return err
	})
}

// failureTape is the tape of a look and an edit that went well and two calls that failed (an absolute path, and a token that
// went stale), made by the real `srwr mcp`. Shown with failure off it has 2 frames; with failure on, 4.
const failureTape = "20260101-0001-with-failure"

const mainReplacement = "func main() {\n\tprintln(\"hi\")\n}"

// makeFailureTape makes, in extra, the workspace pieces of failureTape. a.go is the same as the long-why tape leaves it, so that
// the two can share one workspace.
func makeFailureTape(bin, extra string) error {
	return makeTape(bin, extra, failureTape, func(c *mcpClient) error {
		sel, err := c.tool("look", map[string]any{"file": "a.go", "startLine": 3, "endLine": 3, "why": "main を確かめる"})
		if err != nil {
			return err
		}
		// A path that is absolute: the AI is told to give a relative one, and the tape keeps a failure.
		if _, err := c.tool("look", map[string]any{"file": "/work/a.go", "startLine": 3, "endLine": 3, "why": "もう一度、main を確かめる"}); err == nil {
			return fmt.Errorf("a look with an absolute path did not fail")
		}
		token, _ := sel["selection"].(string)
		if _, err := c.tool("edit", map[string]any{"selection": token, "newText": mainReplacement, "why": "メッセージを出す"}); err != nil {
			return err
		}
		// The same token again: an edit overlapped its range, so it is stale.
		if _, err := c.tool("edit", map[string]any{"selection": token, "newText": mainReplacement, "why": "もう一度、メッセージを出す"}); err == nil {
			return fmt.Errorf("a replace with a stale token did not fail")
		}
		return nil
	})
}

// makeTape runs a session of the real `srwr mcp` on a.go in a temporary directory and puts the one tape it made (with fixed times)
// and the file as it was left into extra under the id.
func makeTape(bin, extra, id string, session func(c *mcpClient) error) error {
	work, err := os.MkdirTemp("", "srwr-maketape-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(work) }()
	source := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(work, "a.go"), []byte(source), 0o600); err != nil {
		return err
	}
	c, err := startMCP(bin, work)
	if err != nil {
		return err
	}
	if _, err := c.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "ui-check", "version": "0"}}); err != nil {
		_ = c.close()
		return err
	}
	if err := c.notify("notifications/initialized"); err != nil {
		_ = c.close()
		return err
	}
	if err := session(c); err != nil {
		_ = c.close()
		return err
	}
	if err := c.close(); err != nil {
		return err
	}
	tapes, err := filepath.Glob(filepath.Join(work, ".srwr", "tapes", "*.tape.jsonl"))
	if err != nil || len(tapes) != 1 {
		return fmt.Errorf("srwr mcp made %d tapes, want 1 (%v)", len(tapes), err)
	}
	if err := os.MkdirAll(filepath.Join(extra, ".srwr", "tapes"), 0o750); err != nil {
		return err
	}
	tape, err := os.ReadFile(tapes[0])
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(extra, ".srwr", "tapes", id+".tape.jsonl"), fixTimes(tape), 0o600); err != nil { //nolint:gosec // the directory of this run
		return err
	}
	// The files as the last call left them, so that the tape has no frame for a change after the recording.
	entries, err := os.ReadDir(work)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() { // .srwr
			continue
		}
		after, err := os.ReadFile(filepath.Join(work, e.Name())) //nolint:gosec // the temporary directory
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(extra, e.Name()), after, 0o600); err != nil { //nolint:gosec // see above
			return err
		}
	}
	return nil
}

// mcpClient talks to `srwr mcp` over its standard input and output.
type mcpClient struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bufio.Scanner
	id  int
}

func startMCP(bin, dir string) (*mcpClient, error) {
	cmd := exec.Command(bin, "mcp")
	cmd.Dir = dir
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	return &mcpClient{cmd: cmd, in: in, out: sc}, nil
}

func (c *mcpClient) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = c.in.Write(append(b, '\n'))
	return err
}

func (c *mcpClient) notify(method string) error {
	return c.send(map[string]any{"jsonrpc": "2.0", "method": method})
}

func (c *mcpClient) call(method string, params any) (json.RawMessage, error) {
	c.id++
	if err := c.send(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	if !c.out.Scan() {
		return nil, fmt.Errorf("srwr mcp ended before answering %s: %v", method, c.out.Err())
	}
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  *struct{ Message string }
	}
	if err := json.Unmarshal(c.out.Bytes(), &r); err != nil {
		return nil, err
	}
	if r.Error != nil {
		return nil, fmt.Errorf("%s: %s", method, r.Error.Message)
	}
	return r.Result, nil
}

// tool calls a tool and returns the JSON the tool answered.
func (c *mcpClient) tool(name string, args map[string]any) (map[string]any, error) {
	res, err := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return nil, err
	}
	var r struct {
		Content []struct{ Text string }
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(res, &r); err != nil || len(r.Content) == 0 {
		return nil, fmt.Errorf("%s: no answer (%v)", name, err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(r.Content[0].Text), &body); err != nil {
		return nil, err
	}
	if r.IsError {
		return nil, fmt.Errorf("%s failed: %s", name, r.Content[0].Text)
	}
	return body, nil
}

func (c *mcpClient) close() error {
	_ = c.in.Close()
	return c.cmd.Wait()
}

var timeField = regexp.MustCompile(`"(startedAt|ts)":"[^"]*"`)

// fixTimes gives the events fixed times, one second apart from the newest day of the fixed tapes on: the screens of the extension
// show the time of the tape (the list of tapes), which must not change from one run to the next. It is the only thing changed.
func fixTimes(tape []byte) []byte {
	n := 0
	return timeField.ReplaceAllFunc(tape, func(m []byte) []byte {
		key := strings.Split(string(m), `"`)[1]
		t := time.Date(2026, 10, 3, 0, 0, n, 0, time.FixedZone("JST", 9*3600))
		if key == "ts" {
			n++
			t = t.Add(time.Second)
		}
		return []byte(fmt.Sprintf(`"%s":"%s"`, key, t.Format("2006-01-02T15:04:05.000-07:00")))
	})
}
