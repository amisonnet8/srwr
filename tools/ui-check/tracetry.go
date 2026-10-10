package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// traceSource is the file the trace-try session changes: two functions, so that the commit has a change in the middle and one
// at the end.
const traceSource = "package main\n\nimport \"fmt\"\n\nfunc one() {\n\tfmt.Println(\"one\")\n}\n\nfunc two() {\n\tfmt.Println(\"two\")\n}\n"

var viewLine = regexp.MustCompile(`srwr view (\S+)\s*$`)

// runTraceTry is qsoku trace-try: it makes a git work tree where the real srwr mcp changes a file and makes another and a person
// adds a line to a third, commits that, runs srwr trace --as-tape on `git show`, and opens the tape it cut in Vim.
func runTraceTry(root string, out io.Writer) error {
	bin := filepath.Join(root, "bin", "srwr")
	base := os.Getenv("SRWR_UI_OPEN_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), "srwr-ui-open")
	}
	ws := filepath.Join(base, "trace-try")
	if err := os.RemoveAll(ws); err != nil { //nolint:gosec // the directory of this run, under SRWR_UI_OPEN_DIR or the temporary directory
		return err
	}
	if err := os.MkdirAll(ws, 0o750); err != nil { //nolint:gosec // see above
		return err
	}
	git := func(args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-c", "user.email=try@example.com", "-c", "user.name=try", "-c", "commit.gpgsign=false"}, args...)...) //nolint:gosec // git, in a temporary directory
		cmd.Dir = ws
		b, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %v: %w\n%s", args, err, b)
		}
		return string(b), nil
	}
	write := func(name, text string) error {
		return os.WriteFile(filepath.Join(ws, name), []byte(text), 0o600) //nolint:gosec // see above
	}
	for _, f := range []struct{ name, text string }{{"a.go", traceSource}, {"c.go", "package main\n"}, {".gitignore", ".srwr/\n"}} {
		if err := write(f.name, f.text); err != nil {
			return err
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"commit", "-q", "-m", "first"}} {
		if _, err := git(args...); err != nil {
			return err
		}
	}

	c, err := startMCP(bin, ws)
	if err != nil {
		return err
	}
	if _, err := c.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "trace-try", "version": "0"}}); err != nil {
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
		{"look", map[string]any{"file": "a.go", "why": "one と two のどこに出力を足すかを決めるため"}},
		{"edit", map[string]any{"file": "a.go", "old": `fmt.Println("one")`, "new": "fmt.Println(\"one\")\n\tfmt.Println(\"one, again\")", "why": "one が呼ばれたことを2回の出力で確かめられるようにするため"}},
		{"new", map[string]any{"file": "b.go", "content": "package main\n\nimport \"fmt\"\n\nfunc three() {\n\tfmt.Println(\"three\")\n}\n", "why": "three を別のファイルに作って、a.go を増やさないため"}},
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
	// A person adds a line to c.go: no tape holds it.
	if err := write("c.go", "package main\n\n// a note written by a person, outside srwr\n"); err != nil {
		return err
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-q", "-m", "Say one twice, make three, add a note"}} {
		if _, err := git(args...); err != nil {
			return err
		}
	}
	show, err := git("show", "HEAD")
	if err != nil {
		return err
	}

	cmd := exec.Command(bin, "trace", "--as-tape") //nolint:gosec // the binary this repository built
	cmd.Dir = ws
	cmd.Stdin = strings.NewReader(show)
	var traced bytes.Buffer
	cmd.Stdout, cmd.Stderr = &traced, &traced
	cmd.Env = append(os.Environ(), "SRWR_LANG="+os.Getenv("SRWR_LANG"))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("srwr trace --as-tape: %w\n%s", err, traced.String())
	}
	m := viewLine.FindStringSubmatch(strings.TrimRight(traced.String(), "\n"))
	if m == nil {
		return fmt.Errorf("srwr trace --as-tape named no tape:\n%s", traced.String())
	}
	id := m[1]

	_, _ = fmt.Fprintf(out, "作業場：%s\n\n$ git show HEAD | srwr trace --as-tape\n%s\n", ws, traced.String())
	_, _ = fmt.Fprint(out, "【手順】\n"+
		"1. 上の出力を読む：a.go と b.go の行に、操作（edit・new）と理由があり、c.go の行は「テープに無い」になっている。\n"+
		"2. Enter で Vim が開く：]] で進め、橙の理由の行（why）と変わった行が、コミットの行と同じか見る（a.go と b.go のコマだけで、c.go のコマは無い）。q で閉じる。\n")
	_, _ = fmt.Fprint(out, "\n読んだら Enter を押してください（Vim が開きます）: ")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')

	view := exec.Command(bin, "view", id) //nolint:gosec // the binary this repository built
	view.Dir = ws
	view.Stdin, view.Stdout, view.Stderr = os.Stdin, os.Stdout, os.Stderr
	view.Env = append(os.Environ(), vimEnv(os.Getenv("SRWR_LANG"), false)...)
	return view.Run()
}
