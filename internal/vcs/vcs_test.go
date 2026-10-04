package vcs

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type info struct {
	Type  string
	Head  *string
	Dirty bool
}

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)...) //nolint:gosec // git, in a temporary directory
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, rel, text string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func detect(t *testing.T, dir string) *info {
	t.Helper()
	raw := Detect(dir)
	if string(raw) == "null" {
		return nil
	}
	var i info
	if err := json.Unmarshal(raw, &i); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	return &i
}

// repo makes a work tree with one commit.
func repo(t *testing.T) string {
	t.Helper()
	needGit(t)
	dir := t.TempDir()
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	runGit(t, dir, "init", "-q")
	write(t, dir, "a.txt", "one\n")
	write(t, dir, "sub/b.txt", "two\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "first")
	return dir
}

func TestNotUnderGit(t *testing.T) {
	needGit(t)
	// A directory that no git work tree contains. The temporary directory of the tests is outside this repository.
	if i := detect(t, t.TempDir()); i != nil {
		t.Skipf("the temporary directory is inside a git work tree: %+v", i)
	}
	if got := string(Detect(t.TempDir())); got != "null" {
		t.Errorf("got %s", got)
	}
}

func TestEmptyRepositoryHasNoHead(t *testing.T) {
	needGit(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	i := detect(t, dir)
	if i == nil || i.Type != "git" || i.Head != nil || i.Dirty {
		t.Errorf("got %+v", i)
	}
	write(t, dir, "new.txt", "x\n")
	if i := detect(t, dir); i == nil || i.Head != nil || !i.Dirty {
		t.Errorf("with a new file: %+v", i)
	}
}

func TestHeadAndDirty(t *testing.T) {
	dir := repo(t)
	head := runGit(t, dir, "rev-parse", "HEAD")
	if i := detect(t, dir); i == nil || i.Type != "git" || i.Head == nil || *i.Head != head || i.Dirty {
		t.Fatalf("clean: %+v want head %s", i, head)
	}
	steps := []struct {
		name string
		do   func()
		undo func()
		want bool
	}{
		{"a tracked file changed", func() { write(t, dir, "a.txt", "changed\n") }, func() { runGit(t, dir, "checkout", "--", "a.txt") }, true},
		{"only staged", func() { write(t, dir, "a.txt", "staged\n"); runGit(t, dir, "add", "a.txt") }, func() { runGit(t, dir, "reset", "-q", "--hard") }, true},
		{"a tracked file deleted", func() { _ = os.Remove(filepath.Join(dir, "a.txt")) }, func() { runGit(t, dir, "checkout", "--", "a.txt") }, true},
		{"a new file", func() { write(t, dir, "new.txt", "x\n") }, func() { _ = os.Remove(filepath.Join(dir, "new.txt")) }, true},
		{"a new file in a directory", func() { write(t, dir, "deep/er/new.txt", "x\n") }, func() { _ = os.RemoveAll(filepath.Join(dir, "deep")) }, true},
		{"only what .gitignore ignores", func() {
			write(t, dir, ".gitignore", "*.log\n")
			runGit(t, dir, "add", ".gitignore")
			runGit(t, dir, "commit", "-q", "-m", "ignore")
			write(t, dir, "x.log", "noise\n")
		}, func() {}, false},
		{"only inside .srwr/", func() {
			write(t, dir, ".srwr/tapes/t.tape.jsonl", "{}\n")
			write(t, dir, ".srwr/key", "k")
		}, func() {}, false},
	}
	for _, s := range steps {
		s.do()
		i := detect(t, dir)
		if i == nil || i.Dirty != s.want {
			t.Errorf("%s: %+v, want dirty %v", s.name, i, s.want)
		}
		s.undo()
		if i := detect(t, dir); i == nil || (s.name != "only what .gitignore ignores" && s.name != "only inside .srwr/" && i.Dirty) {
			t.Errorf("%s: not clean again: %+v", s.name, i)
		}
	}
}

func TestSrwrDirDoesNotHideOtherChanges(t *testing.T) {
	dir := repo(t)
	write(t, dir, ".srwr/key", "k")
	write(t, dir, "a.txt", "changed\n")
	if i := detect(t, dir); i == nil || !i.Dirty {
		t.Errorf("a change next to .srwr/ was lost: %+v", i)
	}
}

func TestWorkspaceInASubdirectory(t *testing.T) {
	dir := repo(t)
	head := runGit(t, dir, "rev-parse", "HEAD")
	sub := filepath.Join(dir, "sub")
	write(t, dir, "a.txt", "changed outside the workspace\n")
	i := detect(t, sub)
	if i == nil || i.Head == nil || *i.Head != head {
		t.Fatalf("got %+v", i)
	}
	if i.Dirty {
		t.Error("a change outside the workspace counts")
	}
	write(t, dir, "sub/b.txt", "changed inside\n")
	if i := detect(t, sub); i == nil || !i.Dirty {
		t.Errorf("a change inside the workspace is lost: %+v", i)
	}
}

func TestGitThatCannotBeRunIsNull(t *testing.T) {
	dir := repo(t)
	t.Setenv("PATH", t.TempDir())
	if got := string(Detect(dir)); got != "null" {
		t.Errorf("without git on PATH: %s", got)
	}
}

func TestBrokenGitDirectoryIsNull(t *testing.T) {
	needGit(t)
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if i := detect(t, dir); i != nil {
		// An empty .git directory is not a repository to git; unless a parent is one, the answer is null.
		t.Skipf("a parent of the temporary directory is a work tree: %+v", i)
	}
}

// A repository can name a program to run on every status (core.fsmonitor). Asking about a work tree must not start it.
func TestDoesNotRunProgramsOfTheRepository(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the program is a shell script")
	}
	dir := repo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	script := filepath.Join(t.TempDir(), "monitor.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o700); err != nil { //nolint:gosec // a script in a temporary directory
		t.Fatal(err)
	}
	runGit(t, dir, "config", "core.fsmonitor", script)
	if i := detect(t, dir); i == nil {
		t.Fatal("null")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the fsmonitor program of the repository was run")
	}
}

func TestParseStatus(t *testing.T) {
	out := " M a.go\x00A  dir/b.go\x00R  new.go\x00old.go\x00?? c.txt\x00 D gone.go\x00"
	got := parseStatus(out, "")
	want := []Change{{"M", "a.go"}, {"A", "dir/b.go"}, {"R", "new.go"}, {"??", "c.txt"}, {"D", "gone.go"}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("change %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// A workspace below the top of the repository: paths are relative to it, and the ones outside are dropped.
	got = parseStatus(" M sub/in.go\x00 M out.go\x00", "sub/")
	if len(got) != 1 || got[0] != (Change{"M", "in.go"}) {
		t.Errorf("below the top: %+v", got)
	}
}

func TestChanges(t *testing.T) {
	needGit(t)
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	runGit(t, root, "init", "-q")
	write(t, root, "a.go", "a\n")
	write(t, root, "sub/b.go", "b\n")
	write(t, root, "gone.go", "g\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "first")
	write(t, root, "a.go", "a2\n")
	write(t, root, "sub/new/c.go", "c\n")
	write(t, root, ".srwr/tapes/x.jsonl", "tape\n") // never counted
	if err := os.Remove(filepath.Join(root, "gone.go")); err != nil {
		t.Fatal(err)
	}
	got, err := Changes(root)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, c := range got {
		seen[c.Path] = c.Status
	}
	want := map[string]string{"a.go": "M", "gone.go": "D", "sub/new/c.go": "??"}
	if len(seen) != len(want) {
		t.Fatalf("changes = %v, want %v", seen, want)
	}
	for p, s := range want {
		if seen[p] != s {
			t.Errorf("%s = %q, want %q", p, seen[p], s)
		}
	}
	// A workspace in a directory of the repository sees only what is below it, with paths relative to it.
	sub, err := Changes(filepath.Join(root, "sub"))
	if err != nil || len(sub) != 1 || sub[0] != (Change{"??", "new/c.go"}) {
		t.Errorf("sub = %+v, %v", sub, err)
	}
}

func TestChangesOutsideGit(t *testing.T) {
	needGit(t)
	dir := t.TempDir()
	if exec.Command("git", "-C", dir, "rev-parse", "--git-dir").Run() == nil { //nolint:gosec // git, in a temporary directory
		t.Skip("the temporary directory is inside a git work tree")
	}
	if _, err := Changes(dir); err == nil {
		t.Error("Changes did not fail outside git")
	}
}

func TestUntracked(t *testing.T) {
	needGit(t)
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	runGit(t, root, "init", "-q")
	write(t, root, "tracked.go", "t\n")
	write(t, root, ".gitignore", "built.txt\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "first")
	write(t, root, "tracked.go", "changed\n") // changed, not new
	write(t, root, "new.go", "n\n")
	write(t, root, "sub/deep/new2.go", "n\n")
	write(t, root, "built.txt", "ignored\n")
	write(t, root, ".srwr/tapes/x.jsonl", "tape\n")
	got, err := Untracked(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := "new.go sub/deep/new2.go"; strings.Join(got, " ") != want {
		t.Errorf("untracked = %v, want %s", got, want)
	}
	// A workspace in a directory of the repository sees only what is below it, with paths relative to it.
	sub, err := Untracked(filepath.Join(root, "sub"))
	if err != nil || strings.Join(sub, " ") != "deep/new2.go" {
		t.Errorf("sub = %v, %v", sub, err)
	}
}
