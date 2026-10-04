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

// srwr tapes check lists what changed in the work tree and is not on the tape, and counts what the settings do not record apart.
func TestTapesCheck(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	runGit(t, root, "init", "-q")
	write(t, root, "seen.go", "package a\n")
	write(t, root, "other.go", "package b\n")
	write(t, root, ".srwrignore", "secret.txt\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "first")

	m := startClient(t, root)
	m.initialize()
	m.mustSelect("seen.go", 1, 1) // the tape knows seen.go and nothing else
	write(t, root, "seen.go", "package a // by the shell\n")
	write(t, root, "other.go", "package b // by the shell\n")
	write(t, root, "new.txt", "new\n")
	write(t, root, "secret.txt", "key\n")

	english(t)
	code, out, errOut := tapesOut(t, root, "check")
	if code != 1 || errOut != "" {
		t.Fatalf("code %d, stderr %q\n%s", code, errOut, out)
	}
	for _, want := range []string{
		"(current session)",
		"not on the tape (2):",
		"  M    other.go\n",
		"  ??   new.txt\n",
		"not recorded by the settings (1): secret.txt",
		"On the tape: 1 file.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "seen.go\n") || strings.Contains(out, "uncommitted changes when this tape started") {
		t.Errorf("a file on the tape is listed, or a clean start is called dirty:\n%s", out)
	}

	t.Setenv("SRWR_LANG", "ja")
	if _, out, _ := tapesOut(t, root, "check"); !strings.Contains(out, "テープにないファイル（2）") {
		t.Errorf("Japanese output:\n%s", out)
	}

	// Nothing is missing once the work tree is clean again.
	runGit(t, root, "checkout", "-q", "--", "other.go")
	if err := os.Remove(filepath.Join(root, "new.txt")); err != nil {
		t.Fatal(err)
	}
	english(t)
	if code, out, _ := tapesOut(t, root, "check"); code != 0 || !strings.Contains(out, "Every file that changed") {
		t.Errorf("code %d:\n%s", code, out)
	}
}

// A tape started in a dirty work tree says that some of the files may be older than the tape. Outside git it says why it cannot check.
func TestTapesCheckDirtyStartAndNoGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	runGit(t, root, "init", "-q")
	write(t, root, "a.go", "package a\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "first")
	write(t, root, "old.txt", "was there before\n") // not committed when the tape starts
	m := startClient(t, root)
	m.initialize()
	m.mustSelect("a.go", 1, 1)
	english(t)
	code, out, _ := tapesOut(t, root, "check")
	if code != 1 || !strings.Contains(out, "old.txt") || !strings.Contains(out, "uncommitted changes when this tape started") {
		t.Errorf("code %d:\n%s", code, out)
	}

	plain := t.TempDir()
	if probe := exec.Command("git", "-C", plain, "rev-parse", "--git-dir"); probe.Run() == nil { //nolint:gosec // git, in a temporary directory
		t.Skip("the temporary directory is inside a git work tree")
	}
	write(t, plain, "a.go", "package a\n")
	m2 := startClient(t, plain)
	m2.initialize()
	m2.mustSelect("a.go", 1, 1)
	if code, _, errOut := tapesOut(t, plain, "check"); code != 1 || !strings.Contains(errOut, "git") {
		t.Errorf("outside git: code %d, stderr %q", code, errOut)
	}
}
