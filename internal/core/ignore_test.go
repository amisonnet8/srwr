package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

const secret = "API_KEY=hunter2-very-secret\n" //nolint:gosec // a fake secret for the tests

// noSecretOnTape fails if any event of the current tape (or its header) holds the secret, or names the file.
func (e *env) noSecretOnTape(files ...string) {
	e.t.Helper()
	active, err := os.ReadFile(filepath.Join(e.root, ".srwr", "active"))
	if err != nil {
		return // no tape at all: nothing was recorded
	}
	b, err := os.ReadFile(filepath.Join(e.root, ".srwr", "tapes", tape.FileName(strings.TrimSpace(string(active))))) //nolint:gosec // a path in a temporary directory
	if err != nil {
		e.t.Fatal(err)
	}
	for _, bad := range append([]string{"hunter2"}, files...) {
		if strings.Contains(string(b), bad) {
			e.t.Errorf("the tape holds %q:\n%s", bad, b)
		}
	}
}

// Entrance 1: select.
func TestSelectTurnsIgnoredFilesAway(t *testing.T) {
	e := newEnv(t)
	e.write(".env", secret)
	e.write("sub/.env.local", secret)
	e.write(".srwr/notes", secret)
	e.write("k/server.PEM", secret)
	e.write(".srwrignore", "# mine\nprivate/\ntoken.txt\n")
	e.write("private/a.txt", secret)
	e.write("deep/token.txt", secret)
	e.write("fine.txt", "ok\n")
	for _, rel := range []string{".env", ".ENV", "sub/.env.local", ".srwr/notes", "k/server.PEM", "private/a.txt", "deep/token.txt"} {
		_, err := e.c.Select(SelectInput{File: rel, StartLine: 1, EndLine: 1, Why: "w"})
		wantCode(t, err, CodeIgnoredFile)
	}
	e.noSecretOnTape(".env", "private", "token.txt")
	if _, err := os.Stat(filepath.Join(e.root, ".srwr", "active")); err == nil {
		t.Error("a tape was made by calls that were all turned away")
	}
	e.sel(e.c, "fine.txt", 1, 1) // the others still work
	e.noSecretOnTape(".env", "private", "token.txt")
}

func TestSelectOfAMissingIgnoredFileIsIgnoredNotMissing(t *testing.T) {
	e := newEnv(t)
	_, err := e.c.Select(SelectInput{File: ".env", StartLine: 1, EndLine: 1, Why: "w"})
	wantCode(t, err, CodeIgnoredFile)
}

func TestSelectThroughALinkToAnIgnoredFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	e := newEnv(t)
	e.write(".env", secret)
	if err := os.Symlink(".env", filepath.Join(e.root, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	_, err := e.c.Select(SelectInput{File: "notes.txt", StartLine: 1, EndLine: 1, Why: "w"})
	wantCode(t, err, CodeIgnoredFile)
	e.noSecretOnTape("notes.txt")
}

// An unreadable .srwrignore: nothing can be called safe, so nothing is recorded.
func TestUnreadableSrwrignoreRecordsNothing(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "1\n")
	if err := os.Mkdir(filepath.Join(e.root, ".srwrignore"), 0o750); err != nil {
		t.Fatal(err)
	}
	_, err := e.c.Select(SelectInput{File: "a.txt", StartLine: 1, EndLine: 1, Why: "w"})
	wantCode(t, err, CodeInternalError)
	notes, herr := e.c.Hook(HookRequest{Selects: []HookSelect{{File: "a.txt", Range: HookRange{Mode: RangeAll}, Tool: "Read"}}})
	if herr != nil || len(notes) != 1 {
		t.Fatalf("hook: notes=%v err=%v, want one note and no error", notes, herr)
	}
	if _, err := os.Stat(filepath.Join(e.root, ".srwr", "active")); err == nil {
		t.Error("a tape was made")
	}
}

// Entrance 2: replace. A file that was fine when it was selected and is left out by the time of the replace.
func TestReplaceTurnsAwayAFileThatIsIgnoredNow(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "one\ntwo\n")
	s := e.sel(e.c, "a.txt", 1, 1)
	e.write(".srwrignore", "a.txt\n")
	_, err := e.c.Replace(ReplaceInput{Selection: s.Selection, NewText: "ONE", Why: "w"})
	wantCode(t, err, CodeIgnoredFile)
	if got := e.read("a.txt"); got != "one\ntwo\n" {
		t.Errorf("the file was written: %q", got)
	}
	if got := e.kinds(); len(got) != 2 || got[0] != tape.TypeSnapshot || got[1] != tape.TypeSelect {
		t.Errorf("kinds = %v, want only what was there before", got)
	}
}

// Entrance 3: hook (Read, Bash, Grep and Edit all come in as selects and an edit).
func TestHookLeavesIgnoredFilesOut(t *testing.T) {
	e := newEnv(t)
	e.write(".env", secret)
	e.write("keys/id_rsa", secret)
	e.write("a.txt", "x\n")
	all := HookRange{Mode: RangeAll}
	notes := e.hook(HookRequest{
		ObserveAll: true,
		Selects: []HookSelect{
			{File: ".env", Range: all, Tool: "Read"},
			{File: "keys/id_rsa", Range: HookRange{Mode: RangeLines, A: 1, B: 1}, Tool: "Bash"},
			{File: ".env", Range: HookRange{Mode: RangeLines, A: 1, B: 1}, Tool: "Grep"},
		},
		Edit: &HookEdit{File: ".env", OldString: "hunter2", NewString: "x", Original: ptr(secret)},
	})
	if len(notes) != 4 {
		t.Errorf("notes = %v, want one for each of the four", notes)
	}
	e.noSecretOnTape(".env", "id_rsa")
	e.hook(HookRequest{Selects: []HookSelect{{File: "a.txt", Range: all, Tool: "Read"}}})
	e.noSecretOnTape(".env", "id_rsa")
}

func ptr(s string) *string { return &s }

// Entrance 4: the look for changes made outside srwr. A file the tape knows is added to .srwrignore and then changed:
// the change is not recorded, by the hook after a Bash call or by the next select of another file.
func TestExternalChangeOfAnIgnoredFileIsNotRecorded(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "before\n")
	e.write("b.txt", "b\n")
	e.sel(e.c, "a.txt", 1, 1)
	e.write(".srwrignore", "a.txt\n")
	e.write("a.txt", "hunter2-after\n")

	notes := e.hook(HookRequest{ObserveAll: true})
	if len(notes) != 0 {
		t.Errorf("notes = %v, want none: a file that is left out is not even mentioned", notes)
	}
	e.sel(e.c, "b.txt", 1, 1)
	e.write("a.txt", "hunter2-again\n")
	e.sel(e.c, "b.txt", 1, 1)
	e.noSecretOnTape()
	for _, k := range e.kinds() {
		if k == tape.TypeExternal {
			t.Error("an external event was recorded for a file that is left out")
		}
	}
	// And a deletion of it is not recorded either.
	if err := os.Remove(filepath.Join(e.root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	e.hook(HookRequest{ObserveAll: true})
	for _, k := range e.kinds() {
		if k == tape.TypeExternal {
			t.Error("an external event for the deletion of a file that is left out")
		}
	}
}
