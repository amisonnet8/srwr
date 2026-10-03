package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

// srwrOnPath puts the built binary on PATH under the name srwr, as an installed srwr is, and returns the directory.
func srwrOnPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	name := "srwr"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	b, err := os.ReadFile(binary(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o700); err != nil { //nolint:gosec // an executable in a temporary directory
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// Stage condition: after srwr init, the commands it wrote into .mcp.json and .claude/settings.json are the ones that work,
// and they write one tape together.
func TestInitThenTheRegisteredCommandsWork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook command is run by sh")
	}
	srwrOnPath(t)
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	write(t, root, "main.go", "package main\n\nfunc main() {}\n")
	cmd := exec.Command("srwr", "init")
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("srwr init: %v\n%s", err, out.String())
	}

	var mcp struct {
		McpServers map[string]struct {
			Command string
			Args    []string
		}
	}
	if err := json.Unmarshal([]byte(read(t, root, ".mcp.json")), &mcp); err != nil {
		t.Fatal(err)
	}
	srv, ok := mcp.McpServers["srwr"]
	if !ok {
		t.Fatalf(".mcp.json = %s", read(t, root, ".mcp.json"))
	}
	mc := exec.Command(srv.Command, srv.Args...) //nolint:gosec // the command srwr init wrote, which is srwr on PATH
	mc.Dir = root
	m := startClientCmd(t, mc)
	m.initialize()
	m.mustSelect("main.go", 1, 3)

	var settings struct {
		Hooks struct {
			PostToolUse []struct {
				Matcher string
				Hooks   []struct{ Type, Command string }
			}
		}
		Permissions struct{ Allow, Deny []string }
	}
	if err := json.Unmarshal([]byte(read(t, root, ".claude/settings.json")), &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Hooks.PostToolUse) != 1 || len(settings.Hooks.PostToolUse[0].Hooks) != 1 {
		t.Fatalf("settings.json = %s", read(t, root, ".claude/settings.json"))
	}
	entry := settings.Hooks.PostToolUse[0]
	if !slices.Contains(settings.Permissions.Allow, "mcp__srwr__select") || !slices.Contains(settings.Permissions.Deny, "Edit") {
		t.Errorf("permissions = %+v", settings.Permissions)
	}
	payload, _ := json.Marshal(map[string]any{
		"hook_event_name": "PostToolUse", "tool_name": "Read", "cwd": root,
		"tool_input": map[string]any{"file_path": filepath.Join(root, "main.go")}, "tool_response": map[string]any{"type": "text"},
	})
	hc := exec.Command("sh", "-c", entry.Hooks[0].Command) //nolint:gosec // the command srwr init wrote
	hc.Dir = root
	hc.Stdin = bytes.NewReader(payload)
	var hookErr bytes.Buffer
	hc.Stderr = &hookErr
	if err := hc.Run(); err != nil {
		t.Fatalf("hook command %q: %v %s", entry.Hooks[0].Command, err, hookErr.String())
	}

	files := tapes(t, root)
	if len(files) != 1 {
		t.Fatalf("tapes = %v, want one", files)
	}
	var got []string
	for _, e := range readTape(t, files[0])[1:] {
		got = append(got, e.Type+"/"+e.Source)
	}
	if want := []string{tape.TypeSnapshot + "/", tape.TypeSelect + "/mcp", tape.TypeSelect + "/hook"}; !slices.Equal(got, want) {
		t.Errorf("tape = %v, want %v", got, want)
	}
}
