package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// longWhyTape is the tape qsoku ui-check makes on the spot: a select and a replace with a why too long for one row, made by
// the real `srwr mcp` (a hand-written tape would not have the shape of a real one). It is named by a fixed id, so that the
// screens are the same every time.
const longWhyTape = "20260101-0000-long-why"

const (
	longSelectWhy  = "これは折り返しの確認のための、とても長い理由です。幅を超えると何行かに分かれ、2行目からは字下げされて、全文が読めるはずです。さらに続けて、三行目にも届くようにします。"
	longReplaceWhy = "置き換えの理由も長くします。橙の理由の行が、幅を超えたときに何行かに分かれ、2行目からは字下げされ、全文が読めて、範囲がその直下に見えることを確かめるための文です。"
)

// makeLongWhyTape makes, in extra, the workspace pieces of the tape (a.go and .srwr/tapes/<longWhyTape>.tape.jsonl), which can be
// copied over the fixed workspace. bin is the srwr binary; the work happens in a temporary directory.
func makeLongWhyTape(bin, extra string) error {
	work, err := os.MkdirTemp("", "srwr-longwhy-")
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
	sel, err := c.tool("select", map[string]any{"file": "a.go", "startLine": 3, "endLine": 3, "why": longSelectWhy})
	if err != nil {
		_ = c.close()
		return err
	}
	token, _ := sel["selection"].(string)
	if _, err := c.tool("replace", map[string]any{"selection": token, "newText": "func main() {\n\tprintln(\"hi\")\n}", "why": longReplaceWhy}); err != nil {
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
	if err := os.WriteFile(filepath.Join(extra, ".srwr", "tapes", longWhyTape+".tape.jsonl"), tape, 0o600); err != nil { //nolint:gosec // the directory of this run
		return err
	}
	// The file as the replace left it, so that the tape has no frame for a change after the recording.
	after, err := os.ReadFile(filepath.Join(work, "a.go")) //nolint:gosec // the temporary directory
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(extra, "a.go"), after, 0o600) //nolint:gosec // see above
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
