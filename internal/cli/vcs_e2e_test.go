package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...) //nolint:gosec // git, in a temporary directory
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// headerVCS reads the vcs of the header of the only tape of the workspace.
func headerVCS(t *testing.T, root string) string {
	t.Helper()
	files := tapes(t, root)
	if len(files) != 1 {
		t.Fatalf("tapes = %v, want one", files)
	}
	b, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(string(b), "\n")
	var h struct{ VCS json.RawMessage }
	if err := json.Unmarshal([]byte(first), &h); err != nil || h.VCS == nil {
		t.Fatalf("header %q: %v", first, err)
	}
	return string(h.VCS)
}

// Stage condition: the header of a tape made by the real srwr mcp and srwr hook says the commit the work started from and whether
// it was clean, in a git work tree; and null outside git.
func TestHeaderSaysTheGitState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	newRoot := func() string {
		root := t.TempDir()
		if real, err := filepath.EvalSymlinks(root); err == nil {
			root = real
		}
		return root
	}

	// mcp, in a work tree with an uncommitted change.
	root := newRoot()
	runGit(t, root, "init", "-q")
	write(t, root, "a.go", "package a\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "first")
	head := runGit(t, root, "rev-parse", "HEAD")
	write(t, root, "a.go", "package a // changed\n")
	m := startClient(t, root)
	m.initialize()
	m.mustSelect("a.go", 1, 1)
	if got, want := headerVCS(t, root), `{"type":"git","head":"`+head+`","dirty":true}`; got != want {
		t.Errorf("mcp, dirty: vcs = %s, want %s", got, want)
	}

	// hook, in a clean work tree: the tape it makes has the same kind of header.
	root = newRoot()
	runGit(t, root, "init", "-q")
	write(t, root, "a.go", "package a\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "first")
	head = runGit(t, root, "rev-parse", "HEAD")
	runHookProcess(t, root, toolPayload("Read", map[string]any{"file_path": filepath.Join(root, "a.go")}, nil))
	if got, want := headerVCS(t, root), `{"type":"git","head":"`+head+`","dirty":false}`; got != want {
		t.Errorf("hook, clean: vcs = %s, want %s", got, want)
	}

	// Not under git (the temporary directory may be inside a work tree on some machines: then there is nothing to check).
	root = newRoot()
	probe := exec.Command("git", "-C", root, "rev-parse", "--git-dir") //nolint:gosec // git, in a temporary directory
	if probe.Run() == nil {
		t.Skip("the temporary directory is inside a git work tree")
	}
	write(t, root, "a.go", "package a\n")
	m2 := startClient(t, root)
	m2.initialize()
	m2.mustSelect("a.go", 1, 1)
	if got := headerVCS(t, root); got != "null" {
		t.Errorf("outside git: vcs = %s, want null", got)
	}
}
