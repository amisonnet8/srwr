// Command usage counts, in the conversation files of Claude Code (.jsonl), how the AI used the tools of srwr: which form of each
// argument it gave, which calls failed and why, and how often it used other tools instead (Read, Grep, Bash cat/grep...). It is how
// the features to keep or remove are judged (todo 2101fbfff0). `go run ./tools/usage [file.jsonl...]` (qsoku usage) reads the
// files given, or *.jsonl in the current directory, and writes ui-check-result/usage/index.html and result.json. It never reads the tapes.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "usage:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	files := args
	if len(files) == 0 {
		var err error
		if files, err = filepath.Glob("*.jsonl"); err != nil {
			return err
		}
	}
	if len(files) == 0 {
		return fmt.Errorf("no .jsonl files: give them as arguments, or put them in the current directory")
	}
	var reports []Report
	for _, f := range files {
		r, err := ReadFile(f)
		if err != nil {
			return err
		}
		reports = append(reports, r)
	}
	dir := filepath.Join("ui-check-result", "usage")
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // a result directory of the project
		return err
	}
	if err := WriteJSON(filepath.Join(dir, "result.json"), reports); err != nil {
		return err
	}
	if err := WritePage(filepath.Join(dir, "index.html"), reports); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "%d files read. Open %s\n", len(reports), filepath.Join(dir, "index.html"))
	return err
}
