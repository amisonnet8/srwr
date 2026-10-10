package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// runSessionTry is qsoku session-try: the real srwr mcp works in three sessions (the first by the 30-minute rule, the other two
// begun with the session tool and a title), then srwr tapes is shown and the list of tapes is opened in Vim.
func runSessionTry(root string, out io.Writer) error {
	bin := filepath.Join(root, "bin", "srwr")
	base := os.Getenv("SRWR_UI_OPEN_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), "srwr-ui-open")
	}
	ws := filepath.Join(base, "session-try")
	if err := os.RemoveAll(ws); err != nil { //nolint:gosec // the directory of this run, under SRWR_UI_OPEN_DIR or the temporary directory
		return err
	}
	if err := os.MkdirAll(ws, 0o750); err != nil { //nolint:gosec // see above
		return err
	}
	if err := os.WriteFile(filepath.Join(ws, "a.go"), []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil { //nolint:gosec // see above
		return err
	}
	c, err := startMCP(bin, ws)
	if err != nil {
		return err
	}
	if _, err := c.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "session-try", "version": "0"}}); err != nil {
		_ = c.close()
		return err
	}
	if err := c.notify("notifications/initialized"); err != nil {
		_ = c.close()
		return err
	}
	calls := []struct {
		tool string
		args map[string]any
	}{
		{"look", map[string]any{"file": "a.go", "why": "main の中身を確かめるため"}},
		{"session", map[string]any{"title": "ドキュメントを先に直す", "why": "仕様の言い回しを決めてから、コードを変えるため"}},
		{"new", map[string]any{"file": "NOTES.md", "content": "# Notes\n\nmain prints a greeting.\n", "why": "先に仕様を書くため"}},
		{"session", map[string]any{"title": "main に挨拶を足す", "why": "NOTES.md に書いた仕様をコードにするため"}},
		{"edit", map[string]any{"file": "a.go", "old": "func main() {}", "new": "func main() { println(\"hello\") }", "why": "NOTES.md のとおりに挨拶を出すため"}},
	}
	for _, call := range calls {
		if _, err := c.tool(call.tool, call.args); err != nil {
			_ = c.close()
			return err
		}
	}
	if err := c.close(); err != nil {
		return err
	}

	list := exec.Command(bin, "tapes") //nolint:gosec // the binary this repository built
	list.Dir = ws
	var listed bytes.Buffer
	list.Stdout, list.Stderr = &listed, &listed
	list.Env = append(os.Environ(), "SRWR_LANG="+os.Getenv("SRWR_LANG"))
	if err := list.Run(); err != nil {
		return fmt.Errorf("srwr tapes: %w\n%s", err, listed.String())
	}
	_, _ = fmt.Fprintf(out, "作業場：%s\n\n$ srwr tapes\n%s\n", ws, listed.String())
	_, _ = fmt.Fprint(out, "【手順】\n"+
		"1. 上の出力を読む：題の付いた2本のテープの行の下に、題が1行ずつ出ている（最初の1本は題なし）。\n"+
		"2. Enter で Vim の一覧が開く：行の最後に題が出ているか見る。<CR> で開いて ]] で進める。q で閉じる。\n")
	_, _ = fmt.Fprint(out, "\n読んだら Enter を押してください（Vim が開きます）: ")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')

	view := exec.Command(bin, "view") //nolint:gosec // the binary this repository built
	view.Dir = ws
	view.Stdin, view.Stdout, view.Stderr = os.Stdin, os.Stdout, os.Stderr
	view.Env = append(os.Environ(), vimEnv(os.Getenv("SRWR_LANG"), false)...)
	return view.Run()
}
