package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Stage condition: with the real srwr mcp and srwr hook, a .env never reaches the tape.
func TestIgnoredFileNeverReachesTheTape(t *testing.T) {
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	secret := "PASSWORD=hunter2-very-secret\n"
	write(t, root, ".env", secret)
	write(t, root, "deploy/prod.key", secret)
	write(t, root, "notes.txt", "hello\n")
	write(t, root, ".srwrignore", "notes.txt\n")
	write(t, root, "main.go", "package main\n")

	m := startClient(t, root)
	m.initialize()
	for _, f := range []string{".env", "deploy/prod.key", "notes.txt"} {
		body, isErr := m.call("select", map[string]any{"file": f, "startLine": 1, "endLine": 1, "why": "見る"})
		if !isErr || body["error"].(map[string]any)["code"] != "ignored_file" {
			t.Errorf("select %s = %v (error %v), want ignored_file", f, body, isErr)
		}
	}
	m.mustSelect("main.go", 1, 1) // a file that is not left out still works

	for _, f := range []string{".env", "deploy/prod.key", "notes.txt"} {
		full := filepath.Join(root, f)
		runHookProcess(t, root, toolPayload("Read", map[string]any{"file_path": full}, nil))
		runHookProcess(t, root, toolPayload("Bash", map[string]any{"command": "cat " + f}, map[string]any{"stdout": secret}))
		runHookProcess(t, root, toolPayload("Edit",
			map[string]any{"file_path": full, "old_string": "hunter2", "new_string": "x"},
			map[string]any{"originalFile": secret}))
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("PASSWORD=hunter2-changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runHookProcess(t, root, toolPayload("Bash", map[string]any{"command": "ls"}, map[string]any{"stdout": ""}))

	files := tapes(t, root)
	if len(files) != 1 {
		t.Fatalf("tapes = %v, want one", files)
	}
	b, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"hunter2", ".env", "prod.key", "notes.txt", "hello"} {
		if strings.Contains(string(b), bad) {
			t.Errorf("the tape holds %q:\n%s", bad, b)
		}
	}
}
