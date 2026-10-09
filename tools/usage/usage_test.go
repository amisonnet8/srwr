package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileCountsTheFormsOfTheArguments(t *testing.T) {
	r, err := ReadFile("testdata/sample.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{
		"srwr: look":                             2,
		"look: search of a directory":            1,
		"look: search with include":              1,
		"look: whole file (no range)":            1,
		"srwr: edit":                             1,
		"edit: edits (1 to 5 items)":             1,
		"edit: edits item: file with old/new":    1,
		"edit: edits item: content (a new file)": 1,
		"edit: edits item with a why":            1,
		"failed: edit content_not_found":         1,
		"failed: with retry":                     1,
		"failed: with nearMatches":               1,
		"other: Bash":                            1,
		"bash: grep":                             1,
		"srwr: replace":                          1,
		"replace: count=3":                       1,
	}
	for k, v := range want {
		if r.Counts[k] != v {
			t.Errorf("%q = %d, want %d", k, r.Counts[k], v)
		}
	}
	if r.Counts["failed: look ?"] != 0 {
		t.Errorf("a look that worked is counted as failed")
	}
}

func TestRunWritesThePageAndTheJSON(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	src, _ := filepath.Abs("testdata/sample.jsonl")
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()
	var out bytes.Buffer
	if err := run([]string{src}, &out); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"index.html", "result.json"} {
		if _, err := os.Stat(filepath.Join(dir, "ui-check-result", "usage", f)); err != nil {
			t.Error(err)
		}
	}
	if err := run(nil, &out); err == nil {
		t.Error("no files: want an error")
	}
}
